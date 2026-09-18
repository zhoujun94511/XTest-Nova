param(
    [Parameter(Mandatory)][string]$Serial,
    [ValidateRange(1, 1440)][int]$DurationMinutes = 120,
    [ValidateRange(1, 60)][int]$SampleIntervalSeconds = 5,
    [ValidateRange(0, 120)][int]$WarmupMinutes = 5,
    [ValidateRange(1024, 65535)][int]$LocalPort = 47912,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Package,
    [string]$OutputPath = ''
)

$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
$agentPackage = 'com.openatx.xtest.nova.uiautomator'
$testPackage = 'com.openatx.xtest.nova.uiautomator.test'
$ownedDeployment = $false
$forwardOwned = $false
$startedAt = [DateTime]::UtcNow
$runId = 'provider-longrun-{0}-{1}' -f $Serial, $startedAt.ToString('yyyyMMddTHHmmssZ')
if (-not $OutputPath) {
    $OutputPath = Join-Path $root ('tests\reports\{0}.json' -f $runId)
}

function Invoke-Adb([string[]]$Arguments) {
    $output = & adb -s $Serial @Arguments
    if ($LASTEXITCODE -ne 0) { throw "adb failed for ${Serial}: $Arguments" }
    return $output
}

function Test-Installed([string]$PackageName) {
    $value = (& adb -s $Serial shell pm path $PackageName 2>$null | Out-String).Trim()
    return $value -match '^package:'
}

function Get-ForegroundPackage {
    $text = (Invoke-Adb @('shell', 'dumpsys', 'window') | Out-String)
    $match = [regex]::Match($text, 'mCurrentFocus=.*?\s([A-Za-z][A-Za-z0-9_.]+)/(?:[A-Za-z0-9_.$]+)')
    if ($match.Success) { return $match.Groups[1].Value }
    $match = [regex]::Match($text, 'mFocusedApp=.*?\s([A-Za-z][A-Za-z0-9_.]+)/(?:[A-Za-z0-9_.$]+)')
    if ($match.Success) { return $match.Groups[1].Value }
    return ''
}

function Get-ProviderPssKb {
    $pidText = (& adb -s $Serial shell pidof $agentPackage 2>$null | Out-String).Trim()
    if ($pidText -notmatch '^\d+') { return $null }
    $memory = (Invoke-Adb @('shell', 'dumpsys', 'meminfo', $matches[0]) | Out-String)
    $match = [regex]::Match($memory, 'TOTAL PSS:\s+(\d+)')
    if (-not $match.Success) { $match = [regex]::Match($memory, '(?m)^\s*TOTAL\s+(\d+)') }
    if ($match.Success) { return [int64]$match.Groups[1].Value }
    return $null
}

function Get-Percentile([System.Collections.Generic.List[double]]$Values, [double]$Percentile) {
    if ($Values.Count -eq 0) { return $null }
    $sorted = @($Values | Sort-Object)
    $index = [Math]::Ceiling($Percentile * $sorted.Count) - 1
    $index = [Math]::Max(0, [Math]::Min($sorted.Count - 1, $index))
    return [Math]::Round([double]$sorted[$index], 2)
}

function Get-PssSlopeKbPerMinute([System.Collections.Generic.List[object]]$Samples, [int]$SkipSamples) {
    $usable = @($Samples | Select-Object -Skip $SkipSamples)
    if ($usable.Count -lt 2) { return $null }
    $count = [double]$usable.Count
    $sumX = 0.0; $sumY = 0.0; $sumXY = 0.0; $sumXX = 0.0
    for ($index = 0; $index -lt $usable.Count; $index++) {
        $x = $index * $SampleIntervalSeconds / 60.0
        $y = [double]$usable[$index].pssKb
        $sumX += $x; $sumY += $y; $sumXY += $x * $y; $sumXX += $x * $x
    }
    $denominator = $count * $sumXX - $sumX * $sumX
    if ($denominator -eq 0) { return $null }
    return [Math]::Round(($count * $sumXY - $sumX * $sumY) / $denominator, 2)
}

function Start-Exploration([string]$BaseUrl, [int]$Sequence) {
    Invoke-Adb @('shell', 'monkey', '-p', $Package, '-c', 'android.intent.category.LAUNCHER', '1') | Out-Null
    $requestId = '{0}-{1}' -f $runId, $Sequence
    $config = [ordered]@{
        requestId = $requestId; package = $Package; maxSteps = 10000; intervalMillis = 750
        seed = 20260909 + $Sequence; execute = $true; enableScroll = $true
        enableBacktrack = $true; recoverPopups = $true
        specialHandling = [ordered]@{
            mode = 'safe'; consentPolicy = 'accept'; permissionPolicy = 'allow'
            paywallPolicy = 'explore'; adPolicy = 'dismiss'; reviewPolicy = 'dismiss'
            onboardingPolicy = 'advance'; maxAttempts = 3
        }
    }
    return Invoke-RestMethod -Uri "$BaseUrl/v1/exploration/sessions" -Method Post -ContentType 'application/json' -Body ($config | ConvertTo-Json -Depth 5) -TimeoutSec 20
}

$latencies = [System.Collections.Generic.List[double]]::new()
$nodeCounts = [System.Collections.Generic.List[int]]::new()
$pssSamples = [System.Collections.Generic.List[object]]::new()
$stopReasons = [System.Collections.Generic.List[string]]::new()
$failures = [System.Collections.Generic.List[string]]::new()
$sessionCount = 0
$totalSteps = 0
$originalForeground = ''
$report = $null

try {
    if ($Serial -notmatch '^[A-Za-z0-9._:-]+$') { throw 'invalid device serial' }
    if ($Package -notmatch '^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$') { throw 'invalid target package' }
    if ((& adb -s $Serial get-state 2>$null | Out-String).Trim() -ne 'device') { throw 'device is not online' }
    if (-not (Test-Installed $Package)) { throw "target package is not installed: $Package" }
    if (Test-Installed $agentPackage -or Test-Installed $testPackage) { throw 'UiAutomator test packages already exist; refusing to replace unowned packages' }
    $existingAgent = (& adb -s $Serial shell pidof xtest-nova-agent 2>$null | Out-String).Trim()
    if ($existingAgent) { throw 'Agent is already running; refusing to replace an unowned process' }
    Assert-ValidationRemotePathsAbsent $Serial @('/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.pid','/data/local/tmp/xtest-nova-agent.log','/data/local/tmp/xtest-nova-agent.staged')
    Assert-ValidationRuntimeAbsent $Serial

    $originalForeground = Get-ForegroundPackage
    & (Join-Path $root 'deploy.ps1') -Serial $Serial -StartServer -HierarchyProvider nova -LocalAgentPort $LocalPort
    if ($LASTEXITCODE -ne 0) { throw 'deployment failed' }
    $ownedDeployment = $true
    $forwardOwned = $true
    $baseUrl = "http://127.0.0.1:$LocalPort"
    $health = Invoke-RestMethod -Uri "$baseUrl/v1/health" -TimeoutSec 10
    if ($health.status -ne 'ok') { throw 'Agent health check failed' }

    $sessionCount++
    Start-Exploration $baseUrl $sessionCount | Out-Null
    $deadline = $startedAt.AddMinutes($DurationMinutes)
    $nextProgress = [DateTime]::UtcNow.AddMinutes(1)
    while ([DateTime]::UtcNow -lt $deadline) {
        try {
            $state = Invoke-RestMethod -Uri "$baseUrl/v1/exploration/sessions/current" -TimeoutSec 10
            if (-not $state.running) {
                $totalSteps += [int]$state.steps
                if ($state.stopReason) { $stopReasons.Add([string]$state.stopReason) }
                $sessionCount++
                Start-Exploration $baseUrl $sessionCount | Out-Null
            }
            $watch = [System.Diagnostics.Stopwatch]::StartNew()
            $hierarchy = Invoke-RestMethod -Uri "$baseUrl/dump/hierarchy" -TimeoutSec 15
            $watch.Stop()
            if (-not $hierarchy.result) { throw 'hierarchy response did not contain result XML' }
            $latencies.Add($watch.Elapsed.TotalMilliseconds)
            $nodeCounts.Add([regex]::Matches([string]$hierarchy.result, '<node(?:\s|>)').Count)
            $pss = Get-ProviderPssKb
            if ($null -ne $pss) {
                $pssSamples.Add([pscustomobject]@{ at = [DateTime]::UtcNow.ToString('o'); pssKb = $pss })
            }
        } catch {
            $failures.Add(('{0}: {1}' -f [DateTime]::UtcNow.ToString('o'), $_.Exception.Message))
        }
        if ([DateTime]::UtcNow -ge $nextProgress) {
            Write-Host ("{0}: samples={1}, failures={2}, sessions={3}" -f $runId, $latencies.Count, $failures.Count, $sessionCount)
            $nextProgress = [DateTime]::UtcNow.AddMinutes(1)
        }
        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    $finalState = Invoke-RestMethod -Uri "$baseUrl/v1/exploration/sessions/current" -TimeoutSec 10
    $runtimeDiagnostics = Invoke-RestMethod -Uri "$baseUrl/v1/diagnostics/runtime" -TimeoutSec 10
    $totalSteps += [int]$finalState.steps
    $pssValues = @($pssSamples | ForEach-Object { [int64]$_.pssKb })
    $pssFirst = if ($pssValues.Count) { $pssValues[0] } else { $null }
    $pssLast = if ($pssValues.Count) { $pssValues[-1] } else { $null }
    $warmupSamples = [Math]::Ceiling($WarmupMinutes * 60.0 / $SampleIntervalSeconds)
    $report = [ordered]@{
        schemaVersion = 'xtest-provider-longrun/v1'
        runId = $runId; serial = $Serial; package = $Package
        startedAt = $startedAt.ToString('o'); endedAt = [DateTime]::UtcNow.ToString('o')
        requestedDurationMinutes = $DurationMinutes; sampleIntervalSeconds = $SampleIntervalSeconds; warmupMinutes = $WarmupMinutes
        sessions = $sessionCount; totalSteps = $totalSteps; samples = $latencies.Count; failures = @($failures)
        latencyScope = 'Agent hierarchy requests sampled concurrently with exploration; not standalone hot-path qualification latency'
        latencyMs = [ordered]@{ p50 = Get-Percentile $latencies 0.50; p95 = Get-Percentile $latencies 0.95; p99 = Get-Percentile $latencies 0.99; max = if ($latencies.Count) { [Math]::Round(($latencies | Measure-Object -Maximum).Maximum, 2) } else { $null } }
        nodes = [ordered]@{ min = if ($nodeCounts.Count) { ($nodeCounts | Measure-Object -Minimum).Minimum } else { $null }; max = if ($nodeCounts.Count) { ($nodeCounts | Measure-Object -Maximum).Maximum } else { $null } }
        pssKb = [ordered]@{ first = $pssFirst; last = $pssLast; delta = if ($null -ne $pssFirst -and $null -ne $pssLast) { $pssLast - $pssFirst } else { $null }; slopeAllKbPerMinute = Get-PssSlopeKbPerMinute $pssSamples 0; slopeAfterWarmupKbPerMinute = Get-PssSlopeKbPerMinute $pssSamples $warmupSamples; warmupSamplesExcluded = [Math]::Min($warmupSamples, $pssSamples.Count); min = if ($pssValues.Count) { ($pssValues | Measure-Object -Minimum).Minimum } else { $null }; max = if ($pssValues.Count) { ($pssValues | Measure-Object -Maximum).Maximum } else { $null }; samples = @($pssSamples) }
        stopReasons = @($stopReasons)
        runtimeDiagnostics = $runtimeDiagnostics
        completed = $true
    }
} catch {
    $failures.Add($_.Exception.Message)
    $report = [ordered]@{
        schemaVersion = 'xtest-provider-longrun/v1'; runId = $runId; serial = $Serial; package = $Package
        startedAt = $startedAt.ToString('o'); endedAt = [DateTime]::UtcNow.ToString('o')
        requestedDurationMinutes = $DurationMinutes; samples = $latencies.Count; failures = @($failures); completed = $false
    }
} finally {
    if ($ownedDeployment) {
        try { Stop-XTestOwnedExecution "http://127.0.0.1:$LocalPort" '/v1/exploration/sessions/current' $null 10 | Out-Null } catch {}
        try { Invoke-RestMethod -Uri "http://127.0.0.1:$LocalPort/uiautomator" -Method Delete -TimeoutSec 10 | Out-Null } catch {}
        & adb -s $Serial shell /data/local/tmp/xtest-nova-agent server -d --stop 2>$null | Out-Null
        Remove-OwnedValidationRuntime $Serial
        & adb -s $Serial shell rm -f /data/local/tmp/xtest-nova-agent /data/local/tmp/xtest-nova-agent.pid /data/local/tmp/xtest-nova-agent.log /data/local/tmp/xtest-nova-agent.staged 2>$null | Out-Null
    }
    if ($forwardOwned) { & adb -s $Serial forward --remove "tcp:$LocalPort" 2>$null | Out-Null }
    if ($originalForeground -and $originalForeground -match '^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$') {
        & adb -s $Serial shell monkey -p $originalForeground -c android.intent.category.LAUNCHER 1 2>$null | Out-Null
    }
    $agentPidAfter = (& adb -s $Serial shell pidof xtest-nova-agent 2>$null | Out-String).Trim()
    $forwardAfter = @(& adb forward --list 2>$null | Where-Object { $_ -match "^$([regex]::Escape($Serial))\s+tcp:$LocalPort\s" })
    $report['cleanup'] = [ordered]@{
        agentStopped = -not [bool]$agentPidAfter
        hostPackageRemoved = -not (Test-Installed $agentPackage)
        testPackageRemoved = -not (Test-Installed $testPackage)
        forwardRemoved = $forwardAfter.Count -eq 0
        targetPackagePreserved = Test-Installed $Package
    }
    $directory = Split-Path -Parent $OutputPath
    if ($directory) { New-Item -ItemType Directory -Force -Path $directory | Out-Null }
    $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding utf8
}

$report | ConvertTo-Json -Depth 8
if (-not $report.completed -or $failures.Count -gt 0) { exit 1 }
