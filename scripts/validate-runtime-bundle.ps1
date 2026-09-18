param(
    [string]$BundleDir=(Join-Path $PSScriptRoot '..\agent\internal\runtimebundle\assets'),
    [string]$AndroidHome=$env:ANDROID_HOME
)
$ErrorActionPreference='Stop'
$toolchain=Import-PowerShellDataFile (Join-Path $PSScriptRoot '..\tools\build-toolchain.psd1')
$buildTools=Get-Item (Join-Path $AndroidHome "build-tools\$($toolchain.AndroidBuildTools)") -ErrorAction Stop
$aapt2=Join-Path $buildTools.FullName 'aapt2.exe'
$signer=Join-Path $buildTools.FullName 'apksigner.bat'
$manifestPath=Join-Path $BundleDir 'manifest.json'
if(-not(Test-Path -LiteralPath $manifestPath)){throw 'Embedded runtime bundle manifest is missing'}
$manifest=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
if($manifest.schemaVersion-ne'xtest-nova-runtime-bundle/v1'){throw 'Unsupported embedded runtime bundle manifest'}
$expectedNames=@('runner','companion','uiautomatorHost','uiautomatorTest')
if(@($manifest.components).Count-ne 4-or@(Compare-Object $expectedNames @($manifest.components.name)).Count){throw 'Embedded runtime component set mismatch'}
$expectedFiles=@('manifest.json')+@($manifest.components.file)
$actualFiles=@(Get-ChildItem -LiteralPath $BundleDir -File|ForEach-Object{$_.Name})
if(@(Compare-Object $expectedFiles $actualFiles).Count){throw 'Runtime bundle directory contains undeclared or missing files'}
$certificates=@()
foreach($component in $manifest.components){
    if($component.name-eq'fixture'-or$component.package-eq'com.xtest.nova.fixture'){throw 'Validation fixture must not enter the runtime bundle'}
    $path=Join-Path $BundleDir $component.file
    if(-not(Test-Path -LiteralPath $path)){throw "Missing embedded component: $($component.file)"}
    $item=Get-Item -LiteralPath $path
    if($item.Length-ne$component.size-or(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$component.sha256){throw "Embedded component hash mismatch: $($component.file)"}
    if($component.kind-eq'apk'){
        $badging=(& $aapt2 dump badging $path|Out-String)
        $package=[regex]::Match($badging,"package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'")
        if($LASTEXITCODE-ne 0-or-not$package.Success-or$package.Groups[1].Value-ne$component.package-or[int]$package.Groups[2].Value-ne$component.versionCode-or$package.Groups[3].Value-ne$component.versionName){throw "Embedded APK metadata mismatch: $($component.file)"}
        $signature=(& $signer verify --verbose --print-certs $path|Out-String)
        $certificate=[regex]::Match($signature,'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]{64})')
        if($LASTEXITCODE-ne 0-or-not$certificate.Success-or$certificate.Groups[1].Value.ToUpperInvariant()-ne$component.certificateSha256.ToUpperInvariant()){throw "Embedded APK signature mismatch: $($component.file)"}
        $certificates+=$component.certificateSha256.ToUpperInvariant()
    }
}
if(@($certificates|Select-Object -Unique).Count-ne 1){throw 'Embedded APK signing certificates do not match'}
Write-Host "Embedded runtime bundle validated: $(@($manifest.components).Count) components"
