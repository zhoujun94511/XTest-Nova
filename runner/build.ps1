param([string]$AndroidHome=$env:ANDROID_HOME,[string]$OutputDir="$PSScriptRoot\\..\\dist")
$ErrorActionPreference='Stop'
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\\..\\tools\\build-toolchain.psd1"
if(-not $AndroidHome){throw 'ANDROID_HOME is required'}
$buildTools=Get-Item "$AndroidHome\\build-tools\\$($toolchain.AndroidBuildTools)" -ErrorAction Stop
$d8=Join-Path $buildTools.FullName 'd8.bat'
$classes=Join-Path $PSScriptRoot 'build\\classes'
$dex=Join-Path $PSScriptRoot 'build\\dex'
Remove-Item -LiteralPath $classes,$dex -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $classes,$dex,$OutputDir|Out-Null
$sources=Get-ChildItem "$PSScriptRoot\\src\\main\\java" -Recurse -Filter *.java|ForEach-Object FullName
javac --release 8 -d $classes $sources
if($LASTEXITCODE -ne 0){throw "javac failed: $LASTEXITCODE"}
java -cp $classes com.openatx.xtest.nova.runner.Main --self-test
if($LASTEXITCODE -ne 0){throw "Runner self-test failed: $LASTEXITCODE"}
$classJar=Join-Path $PSScriptRoot 'build\\runner-classes.jar'
jar --create --date $toolchain.ArchiveTimestamp --file $classJar -C $classes .
if($LASTEXITCODE -ne 0){throw "class jar failed: $LASTEXITCODE"}
& $d8 --min-api 28 --output $dex $classJar
if($LASTEXITCODE -ne 0){throw "d8 failed: $LASTEXITCODE"}
$output=Join-Path $OutputDir 'xtest-nova-runner.jar'
if(Test-Path $output){Remove-Item $output}
jar --create --date $toolchain.ArchiveTimestamp --file $output -C $dex classes.dex
if($LASTEXITCODE -ne 0){throw "jar failed: $LASTEXITCODE"}
Write-Host "Runner: $output"
