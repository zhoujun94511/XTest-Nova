param(
    [Parameter(Mandatory)][string]$Android16Serial,
    [Parameter(Mandatory)][string]$Android15Serial,
    [int]$Android16Port = 7912,
    [int]$Android15Port = 7913,
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\reports\android15-16-pingpong-phase2-validation-20260915'),
    [switch]$SkipShortswave
)
$ErrorActionPreference = 'Stop'
$runner = Join-Path $PSScriptRoot 'validate-phase2-pingpong-20m.ps1'

function Invoke-Phase2Run([string]$Serial, [string]$Label, [int]$Port, [string]$Scenario, [switch]$Perf) {
    & $runner -Serial $Serial -DeviceLabel $Label -Scenario $Scenario -AgentPort $Port -OutputRoot $OutputRoot -EnablePerformance:$Perf
    if ($LASTEXITCODE -ne 0) { throw "Phase 2 run failed: $Label $Scenario" }
}

Write-Host '=== Phase 2 matrix: deploy Phase 2 Agent/Runner to both devices and forward ports before running ==='
Write-Host "Android 16 serial=$Android16Serial port=$Android16Port"
Write-Host "Android 15 serial=$Android15Serial port=$Android15Port"

Invoke-Phase2Run -Serial $Android16Serial -Label 'android16' -Port $Android16Port -Scenario 'game' -Perf
Invoke-Phase2Run -Serial $Android15Serial -Label 'android15' -Port $Android15Port -Scenario 'game' -Perf

if (-not $SkipShortswave) {
    Invoke-Phase2Run -Serial $Android16Serial -Label 'android16' -Port $Android16Port -Scenario 'shortswave'
    Invoke-Phase2Run -Serial $Android15Serial -Label 'android15' -Port $Android15Port -Scenario 'shortswave'
}

& (Join-Path $PSScriptRoot 'aggregate-phase2-report.ps1') -OutputRoot $OutputRoot
