param([string]$AndroidHome=$env:ANDROID_HOME,[string]$OutputPath="$PSScriptRoot\dist\release-manifest.json")
$ErrorActionPreference='Stop'
. "$PSScriptRoot\scripts\release-reproducibility.ps1"
$toolchain=Import-PowerShellDataFile "$PSScriptRoot\tools\build-toolchain.psd1"
$buildTools=Get-Item "$AndroidHome\build-tools\$($toolchain.AndroidBuildTools)" -ErrorAction Stop
$signer=Join-Path $buildTools.FullName 'apksigner.bat';$aapt2=Join-Path $buildTools.FullName 'aapt2.exe'
$agentPaths=@("$PSScriptRoot\dist\xtest-nova-agent-arm64","$PSScriptRoot\dist\xtest-nova-agent-armv7")
foreach($path in $agentPaths){if(-not(Test-Path -LiteralPath $path)){throw "Missing release Agent: $path"}}
& "$PSScriptRoot\scripts\validate-artifact-size.ps1"
& "$PSScriptRoot\scripts\validate-runtime-bundle.ps1" -AndroidHome $AndroidHome
$bundleDir="$PSScriptRoot\agent\internal\runtimebundle\assets";$bundleManifestPath=Join-Path $bundleDir 'manifest.json'
if(-not(Test-Path -LiteralPath $bundleManifestPath)){throw 'Embedded runtime bundle manifest is missing'}
$bundle=Get-Content -LiteralPath $bundleManifestPath -Raw|ConvertFrom-Json
if($bundle.schemaVersion-ne'xtest-nova-runtime-bundle/v1'-or@($bundle.components).Count-ne 4){throw 'Embedded runtime bundle manifest is incomplete'}
$expectedNames=@('runner','companion','uiautomatorHost','uiautomatorTest');if(@(Compare-Object $expectedNames @($bundle.components.name)).Count){throw 'Embedded runtime component set mismatch'}
$expectedFiles=@('manifest.json','xtest-nova-runner.jar','xtest-nova-companion.apk','xtest-nova-uiautomator-host.apk','xtest-nova-uiautomator-test.apk')
$actualFiles=@(Get-ChildItem -LiteralPath $bundleDir -File|ForEach-Object{$_.Name});if(@(Compare-Object $expectedFiles $actualFiles).Count){throw 'Runtime bundle directory must contain only the four declared components and manifest'}
$certificates=@()
foreach($component in $bundle.components){
    if($component.name-eq'fixture'-or$component.package-eq'com.xtest.nova.fixture'){throw 'Validation fixture must not enter the runtime bundle'}
    $path=Join-Path $bundleDir $component.file;if(-not(Test-Path -LiteralPath $path)){throw "Missing embedded component: $($component.file)"}
    $item=Get-Item -LiteralPath $path;if($item.Length-ne$component.size-or(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$component.sha256){throw "Embedded component hash mismatch: $($component.file)"}
    if($component.kind-eq'apk'){
        $badging=(& $aapt2 dump badging $path|Out-String);$package=[regex]::Match($badging,"package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'");if($LASTEXITCODE-ne 0-or-not$package.Success-or$package.Groups[1].Value-ne$component.package-or[int]$package.Groups[2].Value-ne$component.versionCode-or$package.Groups[3].Value-ne$component.versionName){throw "Embedded APK metadata mismatch: $($component.file)"}
        $signature=(& $signer verify --verbose --print-certs $path|Out-String);$certificate=[regex]::Match($signature,'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]{64})');if($LASTEXITCODE-ne 0-or-not$certificate.Success-or$certificate.Groups[1].Value.ToUpperInvariant()-ne$component.certificateSha256.ToUpperInvariant()){throw "Embedded APK signature mismatch: $($component.file)"};$certificates+=$component.certificateSha256.ToUpperInvariant()
    }
}
if(@($certificates|Select-Object -Unique).Count-ne 1){throw 'Embedded APK signing certificates do not match'}
$versionSource=Get-Content -LiteralPath "$PSScriptRoot\agent\internal\buildinfo\version.go" -Raw;$versionMatch=[regex]::Match($versionSource,'const Version = "([^"]+)"');if(-not$versionMatch.Success){throw 'Unable to read Agent version'}
$artifacts=@($agentPaths|ForEach-Object{$item=Get-Item -LiteralPath $_;[ordered]@{name=$item.Name;size=$item.Length;sha256=(Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash}})
$directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null}
$noticeSource=Join-Path $PSScriptRoot 'THIRD_PARTY_NOTICES.md';$sbomSource=Join-Path $PSScriptRoot 'docs\compliance\third-party-sbom.json'
$licenseSources=@(
    [ordered]@{name='github.com/coder/websocket ISC license';path=(Join-Path $PSScriptRoot 'vendor\github.com\coder\websocket\LICENSE.txt')},
    [ordered]@{name='github.com/creack/pty MIT license';path=(Join-Path $PSScriptRoot 'vendor\github.com\creack\pty\LICENSE')},
    [ordered]@{name='Genymobile scrcpy Apache-2.0 license';path=(Join-Path $PSScriptRoot 'agent\internal\scrcpy\LICENSE.scrcpy')}
)
if(-not(Test-Path -LiteralPath $noticeSource)-or-not(Test-Path -LiteralPath $sbomSource)){throw 'Third-party notice or SBOM source is missing'}
$noticeParts=@((Get-Content -LiteralPath $noticeSource -Raw).Trim())
foreach($license in $licenseSources){if(-not(Test-Path -LiteralPath $license.path)){throw "Missing third-party license: $($license.path)"};$noticeParts+="`n`n===== $($license.name) =====`n`n$((Get-Content -LiteralPath $license.path -Raw).Trim())"}
$noticePath=Join-Path $directory 'THIRD_PARTY_NOTICES.txt';$sbomPath=Join-Path $directory 'third-party-sbom.json'
Write-Utf8NoBomText -Path $noticePath -Text ($noticeParts-join'')
Write-Utf8NoBomText -Path $sbomPath -Text (Get-Content -LiteralPath $sbomSource -Raw)
$dependencyDocuments=@($noticePath,$sbomPath|ForEach-Object{$item=Get-Item -LiteralPath $_;[ordered]@{name=$item.Name;size=$item.Length;sha256=(Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash}})
$manifest=[ordered]@{schemaVersion='xtest-nova-release/v2';generatedAt=(Get-ReleaseTimestamp -RepoRoot $PSScriptRoot);agentVersion=$versionMatch.Groups[1].Value;toolchain=$toolchain;runtimeBundle=$bundle;artifacts=$artifacts;dependencyDocuments=$dependencyDocuments}
$manifestJson=$manifest|ConvertTo-Json -Depth 8
Write-Utf8NoBomText -Path $OutputPath -Text $manifestJson
$manifestJson
