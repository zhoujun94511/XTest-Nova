param(
    [string]$AndroidHome=$env:ANDROID_HOME,
    [string]$OutputDir="$PSScriptRoot\..\dist",
    [string]$KeyStore,
    [string]$KeyAlias='xtest-recovery',
    [securestring]$StorePassword,
    [securestring]$KeyPassword
)
$ErrorActionPreference='Stop'
. "$PSScriptRoot\..\scripts\apk-signing.ps1"
if(-not $AndroidHome){throw 'ANDROID_HOME is required'}
$env:ANDROID_HOME=$AndroidHome
$policyClasses=Join-Path $PSScriptRoot 'build\policy-classes'
Remove-Item -LiteralPath $policyClasses -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $policyClasses|Out-Null
$policySource=Join-Path $PSScriptRoot 'test\src\main\java\com\openatx\xtest\nova\uiautomator\test\AccessibilityRecyclingPolicy.java'
javac --release 11 -d $policyClasses $policySource
if($LASTEXITCODE-ne 0){throw "Accessibility recycling policy javac failed: $LASTEXITCODE"}
java -cp $policyClasses com.openatx.xtest.nova.uiautomator.test.AccessibilityRecyclingPolicy
if($LASTEXITCODE-ne 0){throw "Accessibility recycling policy self-test failed: $LASTEXITCODE"}
& "$PSScriptRoot\gradlew.bat" --project-dir $PSScriptRoot --no-daemon :host:assembleRelease :test:assembleRelease
if($LASTEXITCODE-ne 0){throw "UiAutomator build failed: $LASTEXITCODE"}
New-Item -ItemType Directory -Force $OutputDir|Out-Null
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\..\tools\build-toolchain.psd1"
$signer=Join-Path $AndroidHome "build-tools\$($toolchain.AndroidBuildTools)\apksigner.bat"
foreach($component in @('host','test')){
    $source="$PSScriptRoot\$component\build\outputs\apk\release\$component-release-unsigned.apk"
    $unsigned="$OutputDir\xtest-nova-uiautomator-$component-unsigned.apk"
    $signed="$OutputDir\xtest-nova-uiautomator-$component.apk"
    Copy-Item -LiteralPath $source -Destination $unsigned -Force
    if($KeyStore){
        if(-not $StorePassword-or-not $KeyPassword){throw 'StorePassword and KeyPassword are required when signing'}
        try{Invoke-ApkSigner -Signer $signer -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword -OutputPath $signed -InputPath $unsigned}catch{throw "UiAutomator $component signing failed: $($_.Exception.Message)"}
    }else{
        Remove-Item -LiteralPath $signed -Force -ErrorAction SilentlyContinue
    }
}
@('xtest-nova-uiautomator.apk','xtest-nova-uiautomator.apk.idsig','xtest-nova-uiautomator-unsigned.apk')|ForEach-Object{Remove-Item -LiteralPath (Join-Path $OutputDir $_) -Force -ErrorAction SilentlyContinue}
Write-Host "UiAutomator: $OutputDir\xtest-nova-uiautomator-{host,test}[-unsigned].apk"
