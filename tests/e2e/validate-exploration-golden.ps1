param(
    [Parameter(Mandatory=$true)][string]$ScenarioFile,
    [Parameter(Mandatory=$true)][string]$BaselineReport,
    [Parameter(Mandatory=$true)][string]$CandidateReport,
    [string[]]$DeterminismReports=@(),
    [string]$OutputDir=(Join-Path $PSScriptRoot '..\reports\exploration-golden')
)

$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$command=Join-Path $repoRoot 'agent\cmd\exploration-compare'
New-Item -ItemType Directory -Force $OutputDir | Out-Null

function Invoke-Comparison([string[]]$Arguments) {
    & go run $command @Arguments
    if($LASTEXITCODE -ne 0){throw "exploration comparison failed: $LASTEXITCODE"}
}

Invoke-Comparison @('-scenario-file',$ScenarioFile,'-out',(Join-Path $OutputDir 'scenario.json'))
Invoke-Comparison @('-normalize',$BaselineReport,'-out',(Join-Path $OutputDir 'baseline.golden.json'))
Invoke-Comparison @('-normalize',$CandidateReport,'-out',(Join-Path $OutputDir 'candidate.golden.json'))
Invoke-Comparison @('-baseline',$BaselineReport,'-candidate',$CandidateReport,'-out',(Join-Path $OutputDir 'comparison.json'))

if($DeterminismReports.Count -gt 0){
    if($DeterminismReports.Count -lt 2){throw 'DeterminismReports requires at least two reports'}
    Invoke-Comparison @('-determinism',($DeterminismReports -join ','),'-out',(Join-Path $OutputDir 'determinism.json'))
}

Write-Host "Exploration golden reports: $OutputDir"
