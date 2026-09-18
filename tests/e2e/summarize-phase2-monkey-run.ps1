param(
    [Parameter(Mandatory)][string]$ArtifactDir,
    [string]$BaselinePath = (Join-Path $PSScriptRoot '..\reports\android15-16-pingpong-phase2-validation-20260915\BASELINE.json'),
    [string]$DeviceLabel = '',
    [string]$ScenarioLabel = 'game',
    [string]$PerformanceCsvPath = ''
)
$ErrorActionPreference = 'Stop'

function Read-JsonFile([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    return (Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Count-EventStates([string]$EventsPath) {
    $counts = @{}
    if (-not (Test-Path -LiteralPath $EventsPath)) { return $counts }
    Get-Content -LiteralPath $EventsPath -Encoding UTF8 | ForEach-Object {
        if ([string]::IsNullOrWhiteSpace($_)) { return }
        try { $event = $_ | ConvertFrom-Json } catch { return }
        $state = [string]$event.state
        if (-not $counts.ContainsKey($state)) { $counts[$state] = 0 }
        $counts[$state]++
    }
    return $counts
}

$runPath = Join-Path $ArtifactDir 'run.json'
$eventsPath = Join-Path $ArtifactDir 'events.jsonl'
$exitInfoPath = Join-Path $ArtifactDir 'exit_info.json'
$run = Read-JsonFile $runPath
$exitInfo = Read-JsonFile $exitInfoPath
$eventCounts = Count-EventStates $eventsPath

$exploration = $run.exploration
$hardRestarts = [int]($eventCounts['exploration_cycle_restarted'])
if ($hardRestarts -eq 0 -and $null -ne $exploration) {
    $hardRestarts = [int]$eventCounts['exploration_cycle_restarted']
}
$softRefreshes = [int]($eventCounts['exploration_cycle_refreshed'])
if ($softRefreshes -eq 0 -and $null -ne $exploration.generationSoftRefreshes) {
    $softRefreshes = [int]$exploration.generationSoftRefreshes
}

$coordinateActions = if ($null -ne $exploration.coordinateActions) { [int]$exploration.coordinateActions } else { 0 }
$coordinateExternal = if ($null -ne $exploration.coordinateExternal) { [int]$exploration.coordinateExternal } else { 0 }
$sessionActions = if ($null -ne $run.events) { [int]$run.events } else { 0 }
$denominator = [Math]::Max(1, $sessionActions)
$externalRate = [double]$coordinateExternal / [double]$denominator

$relaunchRequested = [int]($eventCounts['relaunch_requested'])
$relaunchCompleted = [int]($eventCounts['relaunch_completed'])
$quarantineEvents = [int]($eventCounts['render_context_quarantine'])
$pingPongBlocks = if ($null -ne $exploration.generationPingPongBlocks) { [int]$exploration.generationPingPongBlocks } else { [int]($eventCounts['generation_pingpong_blocked']) }

$expectedToolExits = 0
$abnormalExits = 0
$unexplainedAbnormal = @()
if ($null -ne $exitInfo -and $null -ne $exitInfo.records) {
    foreach ($record in $exitInfo.records) {
        if ($record.expectedToolExit) { $expectedToolExits++ }
        elseif ($record.abnormal) {
            $abnormalExits++
            $unexplainedAbnormal += [pscustomobject]@{
                reasonCode = $record.reasonCode
                reason     = $record.reason
                subreason  = $record.subreason
                status     = $record.status
                timestamp  = $record.timestamp
            }
        }
    }
}
if ($null -ne $run.abnormalExitCount) { $abnormalExits = [int]$run.abnormalExitCount }

$fpsQualified = $null
$fpsSamples = $null
if ($PerformanceCsvPath -and (Test-Path -LiteralPath $PerformanceCsvPath)) {
    $rows = @(Import-Csv -LiteralPath $PerformanceCsvPath)
    $fpsValues = @($rows | ForEach-Object { if ($_.fps -ne '') { [double]$_.fps } })
    $fpsSamples = $fpsValues.Count
    $fpsQualified = @($fpsValues | Where-Object { $_ -ge 20.0 }).Count
}

$baseline = Read-JsonFile $BaselinePath
if ($null -eq $baseline) { $baseline = [pscustomobject]@{ acceptance = [pscustomobject]@{}; devices = [pscustomobject]@{} } }
$baselineHard = $null
$baselineExternal = $null
$baselineShortswaveEpm = $null
if ($null -ne $baseline -and $DeviceLabel) {
    $device = $baseline.devices.$DeviceLabel
    if ($ScenarioLabel -eq 'game' -and $null -ne $device.game) {
        $baselineHard = $device.game.explorationCycleRestarted
        $baselineExternal = $device.game.coordinateExternalRate
    }
    if ($ScenarioLabel -eq 'shortswave' -and $null -ne $device.shortswave) {
        $baselineShortswaveEpm = $device.shortswave.eventsPerMinute
    }
}

$restartReduction = $null
if ($null -ne $baselineHard -and [int]$baselineHard -gt 0) {
    $restartReduction = 1.0 - ([double]$hardRestarts / [double]$baselineHard)
}

$eventsPerMinute = $null
if ($null -ne $run.startedAt -and $null -ne $run.endedAt) {
    $started = [DateTime]::Parse($run.startedAt)
    $ended = [DateTime]::Parse($run.endedAt)
    $minutes = ($ended - $started).TotalMinutes
    if ($minutes -gt 0) { $eventsPerMinute = [Math]::Round([double]$sessionActions / $minutes, 2) }
}

$maxExternal = if ($null -ne $baseline.acceptance.coordinateExternalRateMax) { [double]$baseline.acceptance.coordinateExternalRateMax } else { 0.02 }
$minReduction = if ($null -ne $baseline.acceptance.hardGenerationRestartReductionMin) { [double]$baseline.acceptance.hardGenerationRestartReductionMin } else { 0.9 }
$minFpsQualified = if ($null -ne $baseline.acceptance.android15FpsQualifiedSamplesMin) { [int]$baseline.acceptance.android15FpsQualifiedSamplesMin } else { 3 }
$acceptance = @{
    hardRestartReductionPass = ($ScenarioLabel -ne 'game') -or ($null -eq $restartReduction) -or ($restartReduction -ge $minReduction)
    externalRatePass         = ($ScenarioLabel -ne 'game') -or ($externalRate -le $maxExternal)
    crashFreePass            = ([int]$run.crashCount -eq 0 -and [int]$run.anrCount -eq 0 -and [int]$run.nativeCrashCount -eq 0)
    fpsPass                  = ($ScenarioLabel -ne 'game') -or ($DeviceLabel -ne 'android15') -or ($null -eq $fpsQualified) -or ($fpsQualified -ge $minFpsQualified)
}

$summary = [ordered]@{
    schemaVersion            = 'xtest-phase2-summary/v1'
    generatedAt              = [DateTime]::UtcNow.ToString('o')
    artifactDir              = (Resolve-Path -LiteralPath $ArtifactDir).Path
    deviceLabel              = $DeviceLabel
    scenarioLabel            = $ScenarioLabel
    package                  = $run.package
    stopReason               = $run.stopReason
    events                   = $sessionActions
    eventsPerMinute          = $eventsPerMinute
    hardGenerationRestarts   = $hardRestarts
    softGenerationRefreshes  = $softRefreshes
    generationPingPongBlocks = $pingPongBlocks
    renderContextQuarantines = $quarantineEvents
    coordinateActions        = $coordinateActions
    coordinateExternal       = $coordinateExternal
    coordinateExternalRate   = [Math]::Round($externalRate, 4)
    relaunchRequested        = $relaunchRequested
    relaunchCompleted        = $relaunchCompleted
    expectedToolExits        = $expectedToolExits
    abnormalExitCount        = $abnormalExits
    unexplainedAbnormal      = @($unexplainedAbnormal)
    fpsSampleCount           = $fpsSamples
    fpsQualifiedSamples      = $fpsQualified
    baselineHardRestarts     = $baselineHard
    hardRestartReduction     = if ($null -ne $restartReduction) { [Math]::Round($restartReduction, 4) } else { $null }
    baselineExternalRate     = $baselineExternal
    baselineShortswaveEpm    = $baselineShortswaveEpm
    acceptance               = $acceptance
    eventStateCounts         = $eventCounts
}

$outPath = Join-Path $ArtifactDir 'phase2-summary.json'
$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $outPath -Encoding utf8
$summary | ConvertTo-Json -Depth 8
