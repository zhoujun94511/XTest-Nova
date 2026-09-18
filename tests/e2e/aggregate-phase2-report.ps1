param(
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\reports\android15-16-pingpong-phase2-validation-20260915')
)
$ErrorActionPreference = 'Stop'
$baselinePath = Join-Path $OutputRoot 'BASELINE.json'
if (-not (Test-Path -LiteralPath $baselinePath)) { throw "Missing baseline: $baselinePath" }
$baseline = Get-Content -LiteralPath $baselinePath -Raw -Encoding UTF8 | ConvertFrom-Json

$summaries = @()
Get-ChildItem -LiteralPath $OutputRoot -Recurse -Filter 'phase2-summary.json' -ErrorAction SilentlyContinue | ForEach-Object {
    $summaries += (Get-Content -LiteralPath $_.FullName -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Format-Rate([object]$Value) {
    if ($null -eq $Value) { return 'n/a' }
    return ('{0:P2}' -f [double]$Value)
}

$sb = New-Object System.Text.StringBuilder
[void]$sb.AppendLine('# Phase 2 真机复测对比（自动生成）')
[void]$sb.AppendLine('')
[void]$sb.AppendLine("生成时间（UTC）：$([DateTime]::UtcNow.ToString('o'))")
[void]$sb.AppendLine('')
[void]$sb.AppendLine('## Phase 2 实测')
[void]$sb.AppendLine('')

if ($summaries.Count -eq 0) {
    [void]$sb.AppendLine('_尚无 phase2-summary.json。_')
} else {
    $header = '| 设备 | 场景 | events | 硬重启 | 软刷新 | 外跳率 | 降幅 | FPS>=20 | abnormal | 判定 |'
    $sep = '| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |'
    [void]$sb.AppendLine($header)
    [void]$sb.AppendLine($sep)
    foreach ($item in ($summaries | Sort-Object deviceLabel, scenarioLabel)) {
        $pass = ($item.acceptance.hardRestartReductionPass -and $item.acceptance.externalRatePass -and $item.acceptance.crashFreePass -and $item.acceptance.fpsPass)
        $verdict = if ($pass) { '通过' } else { '待查' }
        $row = "| $($item.deviceLabel) | $($item.scenarioLabel) | $($item.events) | $($item.hardGenerationRestarts) | $($item.softGenerationRefreshes) | $(Format-Rate $item.coordinateExternalRate) | $(Format-Rate $item.hardRestartReduction) | $($item.fpsQualifiedSamples) | $($item.abnormalExitCount) | $verdict |"
        [void]$sb.AppendLine($row)
    }
}

[void]$sb.AppendLine('')
[void]$sb.AppendLine('## 基线引用')
[void]$sb.AppendLine('')
[void]$sb.AppendLine("- Android 16 游戏硬重启基线: $($baseline.devices.android16.game.explorationCycleRestarted)")
[void]$sb.AppendLine("- Android 16 外跳基线: $(Format-Rate $baseline.devices.android16.game.coordinateExternalRate)")
[void]$sb.AppendLine("- Android 15 外跳基线: $(Format-Rate $baseline.devices.android15.game.coordinateExternalRate)")

$autoPath = Join-Path $OutputRoot 'REPORT-AUTO.md'
[System.IO.File]::WriteAllText($autoPath, $sb.ToString(), [System.Text.UTF8Encoding]::new($false))
Write-Output "Wrote $autoPath ($($summaries.Count) summaries)"
