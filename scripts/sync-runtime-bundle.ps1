param(
    [string]$SourceDir=(Join-Path $PSScriptRoot '..\dist'),
    [string]$TargetDir=(Join-Path $PSScriptRoot '..\agent\internal\runtimebundle\assets'),
    [string]$AndroidHome=$env:ANDROID_HOME
)
$ErrorActionPreference='Stop'
$toolchain=Import-PowerShellDataFile (Join-Path $PSScriptRoot '..\tools\build-toolchain.psd1')
$buildTools=Get-Item (Join-Path $AndroidHome "build-tools\$($toolchain.AndroidBuildTools)") -ErrorAction Stop
$aapt2=Join-Path $buildTools.FullName 'aapt2.exe';$signer=Join-Path $buildTools.FullName 'apksigner.bat'
$definitions=@(
    [ordered]@{name='runner';file='xtest-nova-runner.jar';kind='jar';package='';target='/data/local/tmp/xtest-nova-runner.jar'},
    [ordered]@{name='companion';file='xtest-nova-companion.apk';kind='apk';package='com.openatx.xtest.popup';target='/data/local/tmp/xtest-nova-companion.apk'},
    [ordered]@{name='uiautomatorHost';file='xtest-nova-uiautomator-host.apk';kind='apk';package='com.openatx.xtest.nova.uiautomator';target='/data/local/tmp/xtest-nova-uiautomator-host.apk'},
    [ordered]@{name='uiautomatorTest';file='xtest-nova-uiautomator-test.apk';kind='apk';package='com.openatx.xtest.nova.uiautomator.test';target='/data/local/tmp/xtest-nova-uiautomator-test.apk'}
)
New-Item -ItemType Directory -Force -Path $TargetDir|Out-Null
$allowedFiles=@('manifest.json')+@($definitions.file)
$unexpected=@(Get-ChildItem -LiteralPath $TargetDir -File|Where-Object{$_.Name-notin$allowedFiles})
if($unexpected.Count){throw "Unexpected file in dedicated runtime bundle directory: $($unexpected.Name-join', ')"}
$components=@();$certificates=@()
foreach($definition in $definitions){
    $source=Join-Path $SourceDir $definition.file;if(-not(Test-Path -LiteralPath $source)){throw "Missing signed runtime component: $source"}
    $item=Get-Item -LiteralPath $source;$entry=[ordered]@{name=$definition.name;file=$definition.file;kind=$definition.kind;package=$definition.package;target=$definition.target;size=$item.Length;sha256=(Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash}
    if($definition.kind-eq'apk'){
        $badging=(& $aapt2 dump badging $source|Out-String);if($LASTEXITCODE-ne 0){throw "Unable to inspect $source"};$package=[regex]::Match($badging,"package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'");if(-not$package.Success-or$package.Groups[1].Value-ne$definition.package){throw "Unexpected APK metadata: $source"}
        $signature=(& $signer verify --verbose --print-certs $source|Out-String);if($LASTEXITCODE-ne 0){throw "APK signature verification failed: $source"};$certificate=[regex]::Match($signature,'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]{64})');if(-not$certificate.Success){throw "Unable to read APK certificate: $source"}
        $entry.versionCode=[int]$package.Groups[2].Value;$entry.versionName=$package.Groups[3].Value;$entry.certificateSha256=$certificate.Groups[1].Value.ToUpperInvariant();$certificates+=$entry.certificateSha256
    }
    Copy-Item -LiteralPath $source -Destination (Join-Path $TargetDir $definition.file) -Force
    $components+=[pscustomobject]$entry
}
if(@($certificates|Select-Object -Unique).Count-ne 1){throw 'Embedded APK signing certificates do not match'}
$manifest=[ordered]@{schemaVersion='xtest-nova-runtime-bundle/v1';components=$components}
$manifestPath=Join-Path $TargetDir 'manifest.json'
$manifestJson=$manifest|ConvertTo-Json -Depth 6
[System.IO.File]::WriteAllText($manifestPath,$manifestJson,(New-Object System.Text.UTF8Encoding($false)))
Write-Host "Runtime bundle synchronized: $($components.Count) components; validation fixture excluded"
