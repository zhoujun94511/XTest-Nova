param(
    [string]$Serial = 'R5CN30EQKNM',
    [int]$AgentPort = 17912,
	[ValidateRange(10, 240)][int]$TotalMinutes = 60,
	[ValidateRange(5, 120)][int]$GoMinutes = 30,
	[string]$TargetName = 'gallery',
	[string]$PackageName = '',
	[int]$SeedBase = 2026091000,
	[string]$OutputRoot = '',
    [ValidateRange(1, 10000)][int]$MinimumGoStepsPerRun = 1,
    [ValidateRange(1, 1000000)][int]$MinimumJavaEventsPerRun = 1
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
if ($GoMinutes -le 0 -or $GoMinutes -ge $TotalMinutes) { throw 'GoMinutes must be greater than zero and less than TotalMinutes so both engines execute' }
$galleryPackages = @(
    'com.miui.gallery',
    'com.sec.android.gallery3d',
    'com.android.gallery3d',
    'com.google.android.apps.photos'
)
if (-not $PackageName) {
    foreach ($candidate in $galleryPackages) {
        $path = (& adb -s $Serial shell pm path $candidate 2>$null | Out-String).Trim()
        if ($LASTEXITCODE -eq 0 -and $path -match '^package:') {
            $PackageName = $candidate
            break
        }
    }
}
if (-not $PackageName) { throw "No supported Gallery application is installed on device $Serial" }
$targetSlug = ($TargetName.Trim().ToLowerInvariant() -replace '[^a-z0-9_-]', '-')
if (-not $targetSlug -or $PackageName -notmatch '^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$') { throw 'Invalid target name or Android package' }
$packageName = $PackageName
$base = "http://127.0.0.1:$AgentPort"
$startedAt = [DateTime]::UtcNow
if (-not $OutputRoot) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
	$OutputRoot = Join-Path $PSScriptRoot "..\reports\$targetSlug-long-audit-$stamp"
}
$OutputRoot = [IO.Path]::GetFullPath($OutputRoot)
$goRoot = Join-Path $OutputRoot 'go'
$javaRoot = Join-Path $OutputRoot 'java'
New-Item -ItemType Directory -Force $OutputRoot,$goRoot,$javaRoot | Out-Null

function Save-Json([string]$Path, $Value, [int]$Depth = 20) {
    $Value | ConvertTo-Json -Depth $Depth | Set-Content -LiteralPath $Path -Encoding utf8
}
function Test-NonEmptyFile([string]$Path) {
    return (Test-Path -LiteralPath $Path -PathType Leaf) -and (Get-Item -LiteralPath $Path).Length -gt 0
}
function Get-API([string]$Path) { Invoke-RestMethod -Uri "$base$Path" -TimeoutSec 15 }
function Wait-RunnerFinalized([int]$TimeoutSeconds = 60) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        $state = Get-API '/v1/monkey/runs/current'
        if (-not $state.running -and -not $state.finalizing) { return $state }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Runner finalization did not complete within $TimeoutSeconds seconds"
}
function Wait-ExplorerFinalized([int]$TimeoutSeconds = 60) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        $state = Get-API '/v1/exploration/sessions/current'
        if (-not $state.running -and -not $state.stopping -and -not $state.finalizing) { return $state }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Explorer finalization did not complete within $TimeoutSeconds seconds"
}
function Get-TerminalStateIssues($State) {
    $issues = [Collections.Generic.List[string]]::new()
    if ($State.running -or $State.stopping -or $State.finalizing) { $issues.Add('execution did not reach a stable terminal state') }
    if (-not $State.startedAt -or -not $State.endedAt) { $issues.Add('terminal timestamps are incomplete') }
    if ([string]::IsNullOrWhiteSpace([string]$State.stopReason)) { $issues.Add('stopReason is missing') }
    if (-not [string]::IsNullOrWhiteSpace([string]$State.error)) { $issues.Add("execution error: $($State.error)") }
    return @($issues)
}
function Pull-ExplorationArtifacts([string]$ArtifactDir, [string]$RunDir) {
    if ([string]::IsNullOrWhiteSpace($ArtifactDir)) { throw 'Explorer artifactDir is missing' }
    $artifactRoot = Join-Path $RunDir 'device-artifacts'
    $sessionName = Split-Path ($ArtifactDir.TrimEnd('/')) -Leaf
    if (-not $sessionName) { throw "Invalid Explorer artifact directory: $ArtifactDir" }
    New-Item -ItemType Directory -Force $artifactRoot | Out-Null
    & adb -s $Serial pull $ArtifactDir $artifactRoot | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Failed to pull Explorer artifacts from $ArtifactDir" }
    $sessionOutput = Join-Path $artifactRoot $sessionName
    $required = @('graph.json','steps.json','receipts.json','evidence.json')
    $missing = @($required | Where-Object { -not (Test-NonEmptyFile (Join-Path $sessionOutput $_)) })
    if ($missing.Count) { throw "Explorer artifact set is incomplete: $($missing -join ', ')" }
    $evidence = Get-Content -LiteralPath (Join-Path $sessionOutput 'evidence.json') -Raw | ConvertFrom-Json
    if ($evidence.schemaVersion -ne 'xtest-nova-evidence/v1' -or @($evidence.errors).Count) { throw 'Explorer evidence index is invalid' }
    foreach ($item in @($evidence.items)) {
        $path = Join-Path $sessionOutput (Split-Path $item.path -Leaf)
        if (-not (Test-NonEmptyFile $path) -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $item.sha256) {
            throw "Explorer artifact hash mismatch: $($item.path)"
        }
    }
    return $sessionOutput
}
function Pull-RunnerArtifacts([string]$ArtifactDir, [string]$RunDir) {
    $artifactRoot = Join-Path $RunDir 'device-artifacts'
    $sessionName = Split-Path ($ArtifactDir.TrimEnd('/')) -Leaf
    if (-not $sessionName) { throw "Invalid Runner artifact directory: $ArtifactDir" }
    $sessionOutput = Join-Path $artifactRoot $sessionName
    if (Test-Path -LiteralPath $sessionOutput) { throw "Runner artifact output already exists: $sessionOutput" }
    New-Item -ItemType Directory -Force $artifactRoot | Out-Null
    & adb -s $Serial pull $ArtifactDir $artifactRoot | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Failed to pull Runner artifacts from $ArtifactDir" }

    $required = @('events.jsonl','run.json','activity_coverage.json','activity_coverage.txt','exploration_graph.json','start.png','logcat.txt','crash.json','anr.json','native_crash.txt','diagnostics.txt','exit_info.json')
    $missing = @($required | Where-Object { -not (Test-NonEmptyFile (Join-Path $sessionOutput $_)) })
    if (-not (Test-NonEmptyFile (Join-Path $sessionOutput 'finish.png')) -and -not (Test-NonEmptyFile (Join-Path $sessionOutput 'failure.png'))) {
        $missing += 'finish.png|failure.png'
    }
    if ($missing.Count -gt 0) { throw "Runner artifact set is incomplete: $($missing -join ', ')" }
    $manifest = Get-Content -LiteralPath (Join-Path $sessionOutput 'run.json') -Raw | ConvertFrom-Json
    if (@($manifest.sha256.PSObject.Properties).Count -ne 12) { throw 'Runner manifest must contain 12 artifact hashes' }
    foreach ($property in $manifest.sha256.PSObject.Properties) {
        $path = Join-Path $sessionOutput $property.Name
        if (-not (Test-NonEmptyFile $path) -or (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $property.Value) {
            throw "Runner artifact hash mismatch: $($property.Name)"
        }
    }
    return $sessionOutput
}
function Stop-Executions {
    try { Stop-XTestOwnedExecution $base '/v1/exploration/sessions/current' $null 15 | Out-Null } catch {}
    try { Stop-XTestOwnedExecution $base '/v1/monkey/runs/current' $null 15 | Out-Null } catch {}
}
function Start-Target {
    & adb -s $Serial shell am force-stop $packageName | Out-Null
    for ($attempt = 1; $attempt -le 4; $attempt++) {
        & adb -s $Serial shell monkey -p $packageName -c android.intent.category.LAUNCHER 1 | Out-Null
        Start-Sleep -Seconds 2
        try {
            if ((Get-API '/foregroundPkg').package -eq $packageName) {
                Start-Sleep -Seconds 4
                return
            }
        } catch {}
        & adb -s $Serial shell input keyevent BACK | Out-Null
        Start-Sleep -Seconds 1
    }
    Start-Sleep -Seconds 3
}
function Save-Screenshot([string]$Name) {
    try { Invoke-WebRequest -Uri "$base/screenshot/0" -OutFile (Join-Path $OutputRoot $Name) -TimeoutSec 20 | Out-Null } catch {}
}

try {
    $health = Get-API '/v1/health'
    $device = Get-API '/v1/device'
    $capabilities = Get-API '/v1/capabilities'
    $packageDump = (& adb -s $Serial shell dumpsys package $packageName | Out-String)
    $baseline = [ordered]@{
		schemaVersion = 'xtest-target-long-audit/v1'
		targetName = $TargetName
        serial = $Serial
        package = $packageName
        startedAt = $startedAt.ToString('o')
        requestedMinutes = $TotalMinutes
        goMinutes = $GoMinutes
        javaMinutes = $TotalMinutes - $GoMinutes
        health = $health
        device = $device
        packageVersion = @($packageDump -split "`r?`n" | Where-Object { $_ -match 'version(Name|Code)=' } | ForEach-Object { $_.Trim() })
        capabilities = $capabilities
    }
    Save-Json (Join-Path $OutputRoot 'baseline.json') $baseline
    try { Save-Json (Join-Path $OutputRoot 'runtime-start.json') (Get-API '/v1/diagnostics/runtime') } catch {}
    Save-Screenshot 'start.png'
    Stop-Executions

    $goDeadline = $startedAt.AddMinutes($GoMinutes)
    $goRuns = [Collections.Generic.List[object]]::new()
    $goResults = [Collections.Generic.List[object]]::new()
    $goIndex = 0
    while ([DateTime]::UtcNow -lt $goDeadline) {
        $goIndex++
		Start-Target
		$requestId = "$targetSlug-long-go-$($startedAt.ToString('yyyyMMddHHmmss'))-$goIndex"
        $config = @{
            requestId = $requestId; package = $packageName; maxSteps = 10000; intervalMillis = 500
			seed = $SeedBase + $goIndex; execute = $true; enableScroll = $true; enableBacktrack = $true
            recoverPopups = $true; maxBacktracks = 100
            inputStrategy = @{ casesPerField = 18; maxLength = 64 }
			rules = @{ denyText = @('删除','永久删除','移至回收站','编辑','分享','发送','设为','sign out','purchase','buy','pay','subscribe','start trial','delete','remove','trash','edit','share','send','set as','uninstall') }
            specialHandling = @{ mode='safe';consentPolicy='accept';permissionPolicy='allow';paywallPolicy='explore';adPolicy='dismiss';reviewPolicy='dismiss';onboardingPolicy='advance';maxAttempts=3 }
        }
        $state = Invoke-RestMethod -Uri "$base/v1/exploration/sessions" -Method Post -ContentType 'application/json' -Body ($config | ConvertTo-Json -Depth 8) -TimeoutSec 20
        $nextProgress = [DateTime]::UtcNow
        do {
            Start-Sleep -Seconds 5
            $state = Get-API '/v1/exploration/sessions/current'
            if ([DateTime]::UtcNow -ge $nextProgress) {
                Write-Host ("PROGRESS phase=go run={0} elapsedMin={1:N1} steps={2} states={3} inputs={4} scrolls={5} cycles={6}" -f $goIndex,([DateTime]::UtcNow-$startedAt).TotalMinutes,$state.steps,$state.discoveredStates,$state.inputs,$state.scrolls,$state.cycleDetections)
                $nextProgress = [DateTime]::UtcNow.AddSeconds(45)
            }
            if ([DateTime]::UtcNow -ge $goDeadline -and $state.running) {
                $state = Stop-XTestOwnedExecution $base '/v1/exploration/sessions/current' $state 15
            }
        } while ($state.running)
        $state = Wait-ExplorerFinalized
        $runDir = Join-Path $goRoot ("run-{0:D2}" -f $goIndex)
        New-Item -ItemType Directory -Force $runDir | Out-Null
        Save-Json (Join-Path $runDir 'state.json') $state
        $steps = @(Get-API '/v1/exploration/steps')
        $graph = Get-API '/v1/exploration/graph'
        $report = Get-API "/v1/exploration/report?scenario=$targetSlug-long-go-$goIndex"
        Save-Json (Join-Path $runDir 'steps.json') $steps
        Save-Json (Join-Path $runDir 'graph.json') $graph
		Save-Json (Join-Path $runDir 'report.json') $report
        $issues = [Collections.Generic.List[string]]::new()
        Get-TerminalStateIssues $state | ForEach-Object { $issues.Add($_) }
        if ($report.schemaVersion -ne 'xtest-evaluation/v1' -or $report.engine -ne 'nova' -or -not $report.completed -or -not [string]::IsNullOrWhiteSpace([string]$report.error)) {
            $issues.Add('Explorer diagnostic report is incomplete or inconsistent with a completed Nova run')
        }
        if ([int]$state.steps -lt $MinimumGoStepsPerRun -or $steps.Count -lt $MinimumGoStepsPerRun) {
            $issues.Add("workload below minimum: state=$($state.steps) persisted=$($steps.Count) minimum=$MinimumGoStepsPerRun")
        }
        if ([string]::IsNullOrWhiteSpace([string]$state.artifactDir) -or [string]::IsNullOrWhiteSpace([string]$state.evidenceIndexPath)) {
            $issues.Add('Explorer artifact metadata is incomplete')
        } else {
            try {
                $artifactOutput = Pull-ExplorationArtifacts $state.artifactDir $runDir
                Write-Host "ARTIFACTS phase=go path=$artifactOutput"
            } catch { $issues.Add($_.Exception.Message) }
        }
        $goResults.Add([ordered]@{ run = $goIndex; passed = ($issues.Count -eq 0); workload = [int]$state.steps; issues = @($issues) })
        $goRuns.Add($state)
    }

    $totalDeadline = $startedAt.AddMinutes($TotalMinutes)
    $javaRuns = [Collections.Generic.List[object]]::new()
    $javaResults = [Collections.Generic.List[object]]::new()
    $javaIndex = 0
    while ([DateTime]::UtcNow -lt $totalDeadline) {
        $javaIndex++
		Start-Target
        $javaSeconds = [Math]::Max(60, [int](($totalDeadline - [DateTime]::UtcNow).TotalSeconds))
        $javaConfig = @{
			requestId = "$targetSlug-long-java-$($startedAt.ToString('yyyyMMddHHmmss'))-$javaIndex"
			package = $packageName; durationSeconds = $javaSeconds; throttleMillis = 500; seed = $SeedBase + 99 + $javaIndex
            inputCasesPerField = 18; inputMaxLength = 64
            lowBatteryExit = $true; minBatteryPercent = 15
        }
        $javaState = Invoke-RestMethod -Uri "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body ($javaConfig | ConvertTo-Json -Depth 6) -TimeoutSec 20
        $nextProgress = [DateTime]::UtcNow
        do {
            Start-Sleep -Seconds 5
            $javaState = Get-API '/v1/monkey/runs/current'
            if ([DateTime]::UtcNow -ge $nextProgress) {
                Write-Host ("PROGRESS phase=java run={0} elapsedMin={1:N1} events={2} states={3} inputs={4} scrolls={5} cycles={6} fallback={7}" -f $javaIndex,([DateTime]::UtcNow-$startedAt).TotalMinutes,$javaState.events,$javaState.exploration.states,$javaState.exploration.inputs,$javaState.exploration.scrolls,$javaState.exploration.cycleDetections,$javaState.exploration.fallbackActions)
                $nextProgress = [DateTime]::UtcNow.AddSeconds(45)
            }
            if ([DateTime]::UtcNow -ge $totalDeadline -and $javaState.running) {
                $javaState = Stop-XTestOwnedExecution $base '/v1/monkey/runs/current' $javaState 15
            }
        } while ($javaState.running)
        $javaState = Wait-RunnerFinalized
        $runDir = Join-Path $javaRoot ("run-{0:D2}" -f $javaIndex)
        New-Item -ItemType Directory -Force $runDir | Out-Null
        Save-Json (Join-Path $runDir 'state.json') $javaState
        $issues = [Collections.Generic.List[string]]::new()
        Get-TerminalStateIssues $javaState | ForEach-Object { $issues.Add($_) }
        if ([int64]$javaState.events -lt $MinimumJavaEventsPerRun) { $issues.Add("workload below minimum: events=$($javaState.events) minimum=$MinimumJavaEventsPerRun") }
        if ([int]$javaState.exitCode -ne 0) { $issues.Add("Runner exit code was $($javaState.exitCode)") }
        if (-not [string]::IsNullOrWhiteSpace([string]$javaState.failureType)) { $issues.Add("Runner failure type: $($javaState.failureType)") }
        if (-not $javaState.diagnosticsAvailable -or -not $javaState.diagnosticsComplete -or @($javaState.diagnosticErrors).Count -gt 0) {
            $issues.Add("Runner diagnostics were incomplete: $($javaState.diagnosticErrors -join ', ')")
        }
        if (@($javaState.artifactErrors).Count -gt 0) { $issues.Add("Runner artifact errors: $($javaState.artifactErrors -join ', ')") }
        if ([string]::IsNullOrWhiteSpace([string]$javaState.artifactDir)) {
            $issues.Add('Runner artifactDir is missing')
        } else {
            try {
			    $artifactOutput = Pull-RunnerArtifacts $javaState.artifactDir $runDir
			    Write-Host "ARTIFACTS phase=java path=$artifactOutput"
            } catch { $issues.Add($_.Exception.Message) }
        }
        $javaResults.Add([ordered]@{ run = $javaIndex; passed = ($issues.Count -eq 0); workload = [int64]$javaState.events; issues = @($issues) })
        $javaRuns.Add($javaState)
    }

    try { Save-Json (Join-Path $OutputRoot 'runtime-end.json') (Get-API '/v1/diagnostics/runtime') } catch {}
    (& adb -s $Serial logcat -b crash -d | Out-String) | Set-Content -LiteralPath (Join-Path $OutputRoot 'logcat-crash.txt') -Encoding utf8
    (& adb -s $Serial shell dumpsys activity exit-info $packageName | Out-String) | Set-Content -LiteralPath (Join-Path $OutputRoot 'exit-info.txt') -Encoding utf8
    Save-Screenshot 'finish.png'
    $allGoPassed = ($goResults.Count -gt 0 -and @($goResults | Where-Object { -not $_.passed }).Count -eq 0)
    $allJavaPassed = ($javaResults.Count -gt 0 -and @($javaResults | Where-Object { -not $_.passed }).Count -eq 0)
    $summary = [ordered]@{
		schemaVersion = 'xtest-target-long-audit/v1'
		passed = ($allGoPassed -and $allJavaPassed)
		targetName = $TargetName
        startedAt = $startedAt.ToString('o')
        endedAt = [DateTime]::UtcNow.ToString('o')
        elapsedMinutes = ([DateTime]::UtcNow-$startedAt).TotalMinutes
        goRuns = $goRuns
        goRunResults = $goResults
        javaRuns = $javaRuns
        javaRunResults = $javaResults
        thresholds = [ordered]@{ minimumGoStepsPerRun = $MinimumGoStepsPerRun; minimumJavaEventsPerRun = $MinimumJavaEventsPerRun }
        finalForeground = (Get-API '/foregroundPkg').package
        outputRoot = $OutputRoot
    }
    Save-Json (Join-Path $OutputRoot 'summary.json') $summary
    if (-not $summary.passed) { throw 'Long audit failed one or more workload, terminal-state, diagnostic, or artifact checks; see summary.json' }
    Write-Host "COMPLETE output=$OutputRoot"
} finally {
    Stop-Executions
}
