param(
    [string]$BudgetPath=(Join-Path $PSScriptRoot 'artifact-size-budget.json'),
    [string]$RepoRoot=(Join-Path $PSScriptRoot '..'),
    [switch]$Development
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath($RepoRoot)
if(-not(Test-Path -LiteralPath $BudgetPath)){throw "Artifact size budget not found: $BudgetPath"}
$budget=Get-Content -LiteralPath $BudgetPath -Raw|ConvertFrom-Json
if($budget.schemaVersion-ne'xtest-nova-artifact-size-budget/v1'){throw 'Unsupported artifact size budget schema'}
$results=@()
foreach($definition in @($budget.artifacts)){
    $artifactPath=$definition.path
    if($Development-and$artifactPath-eq'dist/xtest-nova-agent-arm64'){$artifactPath='dist/xtest-nova-agent-development-arm64'}
    if($Development-and$artifactPath-eq'dist/xtest-nova-agent-armv7'){$artifactPath='dist/xtest-nova-agent-development-armv7'}
    $path=[IO.Path]::GetFullPath((Join-Path $root $artifactPath))
    if(-not$path.StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){throw "Artifact path escapes repository: $($definition.path)"}
    if(-not(Test-Path -LiteralPath $path -PathType Leaf)){throw "Required artifact is missing: $($definition.path)"}
    $item=Get-Item -LiteralPath $path
    $maximum=if($null-ne$definition.maximumBytes){[int64]$definition.maximumBytes}else{[int64]$definition.baselineBytes+[int64]$definition.maximumDeltaBytes}
    $delta=[int64]$item.Length-[int64]$definition.baselineBytes
    $result=[pscustomobject][ordered]@{
        Name=$definition.name
        Bytes=[int64]$item.Length
        BaselineBytes=[int64]$definition.baselineBytes
        DeltaBytes=$delta
        MaximumBytes=$maximum
        Sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
    }
    $results+=$result
    if($item.Length-gt$maximum){throw "Artifact size budget exceeded: $($definition.name) is $($item.Length) B; maximum is $maximum B"}
}
$bundlePath=[IO.Path]::GetFullPath((Join-Path $root $budget.runtimeBundle.path))
if(-not$bundlePath.StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){throw 'Runtime bundle path escapes repository'}
$actual=@(Get-ChildItem -LiteralPath $bundlePath -File|ForEach-Object{$_.Name})
$allowed=@($budget.runtimeBundle.allowedFiles)
$difference=@(Compare-Object -ReferenceObject $allowed -DifferenceObject $actual)
if($difference.Count){throw "Runtime bundle content differs from the four-component budget: $($difference.InputObject-join', ')"}
$results|Format-Table -AutoSize
Write-Host "Artifact size gate passed: $($results.Count) artifacts; runtime bundle contains only the declared four components"
