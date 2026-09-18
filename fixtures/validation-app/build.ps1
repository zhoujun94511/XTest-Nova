param([string]$AndroidHome=$env:ANDROID_HOME,[string]$OutputDir="$PSScriptRoot\..\..\dist",[string]$KeyStore,[string]$KeyAlias='xtest-recovery',[securestring]$StorePassword,[securestring]$KeyPassword)
$ErrorActionPreference='Stop'
. "$PSScriptRoot\..\..\scripts\apk-signing.ps1"
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\..\..\tools\build-toolchain.psd1"
$platform=Get-Item "$AndroidHome\platforms\$($toolchain.AndroidPlatform)" -ErrorAction Stop
$tools=Get-Item "$AndroidHome\build-tools\$($toolchain.AndroidBuildTools)" -ErrorAction Stop
$androidJar=Join-Path $platform.FullName 'android.jar'
$aapt2=Join-Path $tools.FullName 'aapt2.exe'
$d8=Join-Path $tools.FullName 'd8.bat'
$zipalign=Join-Path $tools.FullName 'zipalign.exe'
$signer=Join-Path $tools.FullName 'apksigner.bat'
$build="$PSScriptRoot\build"
$classes="$build\classes"
$dex="$build\dex"
Remove-Item -LiteralPath $classes,$dex -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $classes,$dex,$OutputDir|Out-Null
$source=Get-ChildItem "$PSScriptRoot\src" -Recurse -Filter *.java|ForEach-Object FullName
javac --release 8 -classpath $androidJar -d $classes $source
if($LASTEXITCODE-ne 0){throw 'Fixture javac failed'}
$jar="$build\classes.jar"
jar --create --date $toolchain.ArchiveTimestamp --file $jar -C $classes .
if($LASTEXITCODE-ne 0){throw 'Fixture class jar failed'}
& $d8 --lib $androidJar --min-api 28 --output $dex $jar
if($LASTEXITCODE-ne 0){throw 'Fixture d8 failed'}
$base="$build\base.apk"
& $aapt2 link -I $androidJar --manifest "$PSScriptRoot\AndroidManifest.xml" -o $base
if($LASTEXITCODE-ne 0){throw 'Fixture aapt2 failed'}
jar --update --date $toolchain.ArchiveTimestamp --file $base -C $dex classes.dex
if($LASTEXITCODE-ne 0){throw 'Fixture APK assembly failed'}
$aligned=Join-Path $OutputDir 'xtest-nova-validation-unsigned.apk'
$signed=Join-Path $OutputDir 'xtest-nova-validation.apk'
& $zipalign -f 4 $base $aligned
if($LASTEXITCODE-ne 0){throw 'Fixture zipalign failed'}
if($KeyStore){
    if(-not $StorePassword-or-not $KeyPassword){throw 'StorePassword and KeyPassword are required when signing'}
    Remove-Item -LiteralPath $signed -Force -ErrorAction SilentlyContinue
    try{Invoke-ApkSigner -Signer $signer -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword -OutputPath $signed -InputPath $aligned}catch{throw "Fixture signing failed: $($_.Exception.Message)"}
    Write-Host "Validation fixture: $signed"
}else{
    Remove-Item -LiteralPath $signed -Force -ErrorAction SilentlyContinue
    Write-Host "Unsigned validation fixture: $aligned"
}
