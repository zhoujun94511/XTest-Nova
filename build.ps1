param(
    [string]$KeyStore,
    [string]$KeyAlias='xtest-recovery',
    [securestring]$StorePassword,
    [securestring]$KeyPassword,
    [switch]$BuildValidationFixture,
    [switch]$Development
)
$ErrorActionPreference='Stop'
if($Development-and$KeyStore){throw 'Development cannot be combined with signing parameters'}
if($Development-and$BuildValidationFixture){throw 'Development does not build the signed validation fixture'}
if(-not$Development){
    if(-not$KeyStore){throw 'Release builds require -KeyStore. Use -Development to build development-only Runner and Agent artifacts with the existing embedded runtime bundle.'}
    if(-not$StorePassword-or-not$KeyPassword){throw 'Release builds require StorePassword and KeyPassword'}
}
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\tools\build-toolchain.psd1"
$goVersion=(go version|Out-String).Trim();if($goVersion-notmatch"\s$([regex]::Escape($toolchain.GoVersion))\s"){throw "Expected Go $($toolchain.GoVersion), got: $goVersion"}
$javaVersion=(javac -version 2>&1|Out-String).Trim();if($javaVersion-notmatch"^javac $([regex]::Escape($toolchain.JavaMajor))\."){throw "Expected javac $($toolchain.JavaMajor).x, got: $javaVersion"}
if(-not$Development-and-not$BuildValidationFixture){
    @('xtest-nova-validation.apk','xtest-nova-validation.apk.idsig','xtest-nova-validation-unsigned.apk')|ForEach-Object{Remove-Item -LiteralPath (Join-Path $PSScriptRoot "dist\$_") -Force -ErrorAction SilentlyContinue}
}
& "$PSScriptRoot\runner\build.ps1"
if($Development){
    & "$PSScriptRoot\scripts\validate-runtime-bundle.ps1" -AndroidHome $env:ANDROID_HOME
    Write-Warning 'Development build reuses the validated, previously embedded release runtime bundle. The newly built dist\xtest-nova-runner.jar is development-only and is not embedded.'
}else{
    & "$PSScriptRoot\companion\build.ps1" -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword
    & "$PSScriptRoot\uiautomator\build.ps1" -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword
    if($BuildValidationFixture){& "$PSScriptRoot\fixtures\validation-app\build.ps1" -KeyStore $KeyStore -KeyAlias $KeyAlias -StorePassword $StorePassword -KeyPassword $KeyPassword}
    & "$PSScriptRoot\scripts\sync-runtime-bundle.ps1" -AndroidHome $env:ANDROID_HOME
}
Push-Location "$PSScriptRoot\agent"
try{
    $previousGoCache=$env:GOCACHE;$previousGoProxy=$env:GOPROXY;$previousGoFlags=$env:GOFLAGS
    $previousCgo=$env:CGO_ENABLED;$previousGoos=$env:GOOS;$previousGoarch=$env:GOARCH;$previousGoarm=$env:GOARM
    $env:GOCACHE=Join-Path $PSScriptRoot '.gocache';New-Item -ItemType Directory -Force $env:GOCACHE|Out-Null
    $env:GOPROXY='off';$env:GOFLAGS='-mod=vendor'
    Remove-Item Env:GOOS,Env:GOARCH,Env:GOARM -ErrorAction SilentlyContinue
    go test ./...;if($LASTEXITCODE-ne 0){throw 'Go tests failed'}
    New-Item -ItemType Directory -Force "$PSScriptRoot\dist"|Out-Null
    $env:CGO_ENABLED='0';$env:GOOS='linux';$env:GOARCH='arm64'
    $agentPrefix=if($Development){'xtest-nova-agent-development'}else{'xtest-nova-agent'}
    $arm64Path="$PSScriptRoot\dist\$agentPrefix-arm64";$armv7Path="$PSScriptRoot\dist\$agentPrefix-armv7"
    go build -trimpath -buildvcs=false -o $arm64Path ./cmd/xtest-nova-agent;if($LASTEXITCODE-ne 0){throw 'ARM64 build failed'}
    $env:GOARCH='arm';$env:GOARM='7'
    go build -trimpath -buildvcs=false -o $armv7Path ./cmd/xtest-nova-agent;if($LASTEXITCODE-ne 0){throw 'ARMv7 build failed'}
}finally{
    Pop-Location
    foreach($entry in @(
        @('GOCACHE',$previousGoCache),@('GOPROXY',$previousGoProxy),@('GOFLAGS',$previousGoFlags),@('CGO_ENABLED',$previousCgo),
        @('GOOS',$previousGoos),@('GOARCH',$previousGoarch),@('GOARM',$previousGoarm)
    )){if($null-eq$entry[1]){Remove-Item "Env:$($entry[0])" -ErrorAction SilentlyContinue}else{Set-Item "Env:$($entry[0])" $entry[1]}}
}
& "$PSScriptRoot\scripts\validate-artifact-size.ps1" -Development:$Development
if($Development){
    $bundle=Get-Content -LiteralPath "$PSScriptRoot\agent\internal\runtimebundle\assets\manifest.json" -Raw|ConvertFrom-Json
    $developmentManifest=[ordered]@{
        schemaVersion='xtest-nova-development-build/v1'
        warning='Development-only Agents reuse the existing validated embedded release runtime bundle; dist/xtest-nova-runner.jar is not embedded.'
        artifacts=@($arm64Path,$armv7Path|ForEach-Object{$item=Get-Item -LiteralPath $_;[ordered]@{name=$item.Name;size=$item.Length;sha256=(Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash}})
        developmentRunner=(&{$item=Get-Item -LiteralPath "$PSScriptRoot\dist\xtest-nova-runner.jar";[ordered]@{name=$item.Name;size=$item.Length;sha256=(Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash}})
        embeddedRuntimeBundle=$bundle
    }
    $json=($developmentManifest|ConvertTo-Json -Depth 8).Replace("`r`n","`n").TrimEnd()+"`n"
    [IO.File]::WriteAllText("$PSScriptRoot\dist\development-build-manifest.json",$json,(New-Object Text.UTF8Encoding($false)))
    Write-Host "Development-only Agent artifacts: $PSScriptRoot\dist\xtest-nova-agent-development-{arm64,armv7}"
}else{
    Write-Host "Release Agent artifacts: $PSScriptRoot\dist\xtest-nova-agent-{arm64,armv7}"
}
