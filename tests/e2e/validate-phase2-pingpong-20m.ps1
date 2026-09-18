param(
    [Parameter(Mandatory)][string]$Serial,
    [Parameter(Mandatory)][ValidateSet('android15','android16')][string]$DeviceLabel,
    [Parameter(Mandatory)][ValidateSet('game','shortswave')][string]$Scenario,
    [ValidateRange(1,65535)][int]$AgentPort = 7912,
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\reports\android15-16-pingpong-phase2-validation-20260915'),
    [switch]$EnablePerformance,
    [int]$PerformanceIntervalSeconds = 5
)
$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))

function Invoke-Adb([string[]]$Arguments) {
    $output = & adb -s $Serial @Arguments
    if ($LASTEXITCODE -ne 0) { throw "adb failed: $($Arguments -join ' ')" }
    return $output
}

function Wait-MonkeyFinished([string]$BaseUrl) {
    $deadline = [DateTime]::UtcNow.AddSeconds(1500)
    do {
        $state = Invoke-RestMethod "$BaseUrl/v1/monkey/runs/current" -TimeoutSec 20
        if (-not $state.running -and -not $state.finalizing) { return $state }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Monkey run did not finish within 25 minutes'
}

if ($Serial -notmatch '^[A-Za-z0-9._:-]+$') { throw 'Invalid serial' }

$scenarioPath = switch ($Scenario) {
    'game' { Join-Path $repoRoot 'tests\scenarios\watermelon-maker-2048-monkey-20m.json' }
    'shortswave' { Join-Path $repoRoot 'tests\scenarios\shortswave-monkey-20m-bounded.json' }
}
if (-not (Test-Path -LiteralPath $scenarioPath)) { throw "Missing scenario: $scenarioPath" }

$config = Get-Content -LiteralPath $scenarioPath -Raw -Encoding UTF8 | ConvertFrom-Json
$config.requestId = "$($config.requestId)-phase2-$DeviceLabel-$(Get-Date -Format 'yyyyMMddHHmmss')"

$runDir = Join-Path $OutputRoot "$DeviceLabel-$Scenario"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

$base = "http://127.0.0.1:$AgentPort"
$health = Invoke-RestMethod "$base/v1/health" -TimeoutSec 5
if ($health.status -ne 'ok') { throw 'Agent unhealthy' }

$current = Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10
if ($current.running -or $current.finalizing) { throw 'Another Monkey run is active; stop it first' }

$performanceStarted = $false
$performanceCsvLocal = ''
if ($EnablePerformance -and $Scenario -eq 'game') {
    try {
        $perfBody = @{
            package          = [string]$config.package
            intervalSeconds  = $PerformanceIntervalSeconds
            durationSeconds  = [int]$config.durationSeconds
        } | ConvertTo-Json -Compress
        $performanceState = Invoke-RestMethod "$base/v1/performance/sessions" -Method Post -ContentType 'application/json' -Body $perfBody -TimeoutSec 20
        $performanceStarted = $true
    } catch {
        Write-Warning "Performance session skipped: $($_.Exception.Message)"
    }
}

$body = $config | ConvertTo-Json -Depth 6 -Compress
try {
    $response = Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $body -TimeoutSec 30
} catch {
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        throw "Monkey start failed: HTTP $([int]$_.Exception.Response.StatusCode) $($reader.ReadToEnd())"
    }
    throw
}
if ($response.StatusCode -ne 202) { throw "Monkey start failed: HTTP $($response.StatusCode) $($response.Content)" }
$started = $response.Content | ConvertFrom-Json
if (-not $started.running) { throw 'Monkey did not enter running state' }

$final = Wait-MonkeyFinished $base

if ($performanceStarted) {
    try {
        $performanceHeaders=@{'X-XTest-Session-Id'=[string]$performanceState.identity.sessionId;'X-XTest-Owner-Token'=[string]$performanceState.identity.ownerToken}
        $perfState = Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers $performanceHeaders -TimeoutSec 60
        if ($perfState.path) {
            $performanceCsvLocal = Join-Path $runDir 'performance.csv'
            Invoke-Adb @('pull', $perfState.path, $performanceCsvLocal) | Out-Null
        }
    } catch {
        Write-Warning "Performance session cleanup failed: $($_.Exception.Message)"
    }
}

if (-not $final.artifactDir) { throw 'artifactDir missing from final Monkey state' }
$remoteDir = [string]$final.artifactDir
Invoke-Adb @('pull', $remoteDir, $runDir) | Out-Null
$pulledDir = Get-ChildItem -LiteralPath $runDir -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if ($pulledDir) {
    Get-ChildItem -LiteralPath $pulledDir.FullName | Move-Item -Destination $runDir -Force
    Remove-Item -LiteralPath $pulledDir.FullName -Recurse -Force
}

$manifest = [ordered]@{
    schemaVersion = 'xtest-phase2-run/v1'
    generatedAt   = [DateTime]::UtcNow.ToString('o')
    serial        = $Serial
    deviceLabel   = $DeviceLabel
    scenario      = $Scenario
    agentVersion  = $health.version
    requestId     = $config.requestId
    remoteDir     = $remoteDir
    localDir      = (Resolve-Path -LiteralPath $runDir).Path
    stopReason    = $final.stopReason
    events        = $final.events
}
$manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $runDir 'phase2-run-manifest.json') -Encoding utf8

$summaryScript = Join-Path $PSScriptRoot 'summarize-phase2-monkey-run.ps1'
& $summaryScript -ArtifactDir $runDir -DeviceLabel $DeviceLabel -ScenarioLabel $Scenario -PerformanceCsvPath $performanceCsvLocal

Write-Host "Phase 2 run complete: $runDir"
