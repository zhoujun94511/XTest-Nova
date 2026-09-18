param(
    [string]$AndroidHome=$env:ANDROID_HOME,
    [string]$OutputDir="$PSScriptRoot\\..\\dist",
    [string]$KeyStore,
    [string]$KeyAlias='xtest-recovery',
    [securestring]$StorePassword,
    [securestring]$KeyPassword,
    [ValidatePattern('^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$')][string]$PackageName='com.openatx.xtest.popup',
    [ValidatePattern('^[A-Za-z0-9._-]+$')][string]$ArtifactName='xtest-nova-companion',
    [string]$ApplicationLabel='XTest Nova'
)
$ErrorActionPreference='Stop'
. "$PSScriptRoot\..\scripts\apk-signing.ps1"
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\\..\\tools\\build-toolchain.psd1"
$platform=Get-Item "$AndroidHome\\platforms\\$($toolchain.AndroidPlatform)" -ErrorAction Stop
$tools=Get-Item "$AndroidHome\\build-tools\\$($toolchain.AndroidBuildTools)" -ErrorAction Stop
$androidJar=Join-Path $platform.FullName 'android.jar';$aapt2=Join-Path $tools.FullName 'aapt2.exe';$d8=Join-Path $tools.FullName 'd8.bat';$zipalign=Join-Path $tools.FullName 'zipalign.exe';$signer=Join-Path $tools.FullName 'apksigner.bat'
$build="$PSScriptRoot\\build";$classes="$build\\classes";$dex="$build\\dex";Remove-Item -LiteralPath $classes,$dex -Recurse -Force -ErrorAction SilentlyContinue;New-Item -ItemType Directory -Force $classes,$dex,$OutputDir|Out-Null
$src=Get-ChildItem "$PSScriptRoot\\app\\src\\main\\java" -Recurse -Filter *.java|ForEach-Object FullName
javac --release 8 -classpath $androidJar -d $classes $src
if($LASTEXITCODE -ne 0){throw "javac failed: $LASTEXITCODE"}
$jar="$build\\classes.jar";jar --create --date $toolchain.ArchiveTimestamp --file $jar -C $classes .
if($LASTEXITCODE -ne 0){throw "class jar failed: $LASTEXITCODE"}
& $d8 --lib $androidJar --min-api 28 --output $dex $jar
if($LASTEXITCODE -ne 0){throw "d8 failed: $LASTEXITCODE"}
$manifest="$build\\AndroidManifest.generated.xml"
$manifestText=Get-Content -LiteralPath "$PSScriptRoot\\app\\src\\main\\AndroidManifest.xml" -Raw
$manifestText=$manifestText.Replace('package="com.openatx.xtest.popup"',"package=`"$PackageName`"")
$escapedLabel=[System.Security.SecurityElement]::Escape($ApplicationLabel)
$manifestText=$manifestText.Replace('android:label="XTest Nova"',"android:label=`"$escapedLabel`"")
Set-Content -LiteralPath $manifest -Value $manifestText -Encoding UTF8
$base="$build\\base.apk";& $aapt2 link -I $androidJar --manifest $manifest -o $base
if($LASTEXITCODE -ne 0){throw "aapt2 failed: $LASTEXITCODE"}
jar --update --date $toolchain.ArchiveTimestamp --file $base -C $dex classes.dex
if($LASTEXITCODE -ne 0){throw "APK assembly failed: $LASTEXITCODE"}
if($LASTEXITCODE -ne 0){throw "jar failed: $LASTEXITCODE"}
$aligned=Join-Path $OutputDir "$ArtifactName-unsigned.apk";$signed=Join-Path $OutputDir "$ArtifactName.apk";& $zipalign -f 4 $base $aligned
if($LASTEXITCODE -ne 0){throw "zipalign failed: $LASTEXITCODE"}
if($KeyStore){
    if(-not $StorePassword-or-not $KeyPassword){throw 'StorePassword and KeyPassword are required when signing'}
    Remove-Item -LiteralPath $signed -Force -ErrorAction SilentlyContinue
    try{Invoke-ApkSigner -Signer $signer -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword -OutputPath $signed -InputPath $aligned}catch{throw "Companion signing failed: $($_.Exception.Message)"}
    Write-Host "Companion: $signed"
}else{
    Remove-Item -LiteralPath $signed -Force -ErrorAction SilentlyContinue
    Write-Host "Unsigned companion: $aligned"
}
