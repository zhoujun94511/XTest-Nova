param(
    [string]$ManifestPath=(Join-Path $PSScriptRoot '..\dist\release-manifest.json'),
    [string]$ReferenceContractPath=(Join-Path $PSScriptRoot '..\docs\compliance\reference-http-contract.md'),
    [string]$AndroidHome=$env:ANDROID_HOME,
    [switch]$RequireCleanGit
)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$gitRepoRoot=$repoRoot.Replace('\','/')
. (Join-Path $PSScriptRoot 'release-reproducibility.ps1')
$toolchain=Import-PowerShellDataFile (Join-Path $repoRoot 'tools/build-toolchain.psd1')
$buildTools=Get-Item (Join-Path $AndroidHome "build-tools\$($toolchain.AndroidBuildTools)") -ErrorAction Stop
$aapt2=Join-Path $buildTools.FullName 'aapt2.exe'
$signer=Join-Path $buildTools.FullName 'apksigner.bat'
function Get-ActualApkMetadata([string]$Path){
    $badging=(& $aapt2 dump badging $Path|Out-String);if($LASTEXITCODE-ne 0){throw "Unable to read APK metadata: $Path"}
    $package=[regex]::Match($badging,"package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'");if(-not$package.Success){throw "Unable to parse APK metadata: $Path"}
    $signature=(& $signer verify --verbose --print-certs $Path|Out-String);if($LASTEXITCODE-ne 0){throw "APK signature verification failed: $Path"}
    $certificate=[regex]::Match($signature,'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]{64})');if(-not$certificate.Success){throw "Unable to parse APK signing certificate: $Path"}
    return [pscustomobject]@{package=$package.Groups[1].Value;versionCode=[int]$package.Groups[2].Value;versionName=$package.Groups[3].Value;certificateSha256=$certificate.Groups[1].Value.ToUpperInvariant()}
}
if(-not(Test-Path -LiteralPath $ManifestPath)){throw 'Release manifest not found'}
& (Join-Path $PSScriptRoot 'validate-artifact-size.ps1')
$manifest=Get-Content -LiteralPath $ManifestPath -Raw|ConvertFrom-Json
if($manifest.schemaVersion-ne'xtest-nova-release/v2'){throw 'Unsupported release manifest schema'}
$expectedArtifacts=@('xtest-nova-agent-arm64','xtest-nova-agent-armv7')
$actualArtifacts=@($manifest.artifacts|ForEach-Object{$_.name})
if($actualArtifacts.Count-ne$expectedArtifacts.Count){throw "Release manifest must contain exactly $($expectedArtifacts.Count) artifacts"}
if(@($actualArtifacts|Group-Object|Where-Object{$_.Count-ne 1}).Count){throw 'Release manifest contains duplicate artifacts'}
$artifactDifference=@(Compare-Object -ReferenceObject $expectedArtifacts -DifferenceObject $actualArtifacts)
if($artifactDifference.Count){throw "Release manifest artifact set mismatch: $($artifactDifference.InputObject-join', ')"}
$versionSource=Get-Content -LiteralPath (Join-Path $repoRoot 'agent/internal/buildinfo/version.go') -Raw
$versionMatch=[regex]::Match($versionSource,'const Version = "([^"]+)"')
if(-not$versionMatch.Success-or$manifest.agentVersion-ne$versionMatch.Groups[1].Value){throw 'Release manifest Agent version does not match source'}
foreach($artifact in $manifest.artifacts){if($artifact.sha256-notmatch'^[A-Fa-f0-9]{64}$'-or$artifact.size-le 0){throw "Invalid artifact metadata: $($artifact.name)"};$path=Join-Path "$repoRoot\dist" $artifact.name;if(-not(Test-Path -LiteralPath $path)){throw "Missing artifact: $($artifact.name)"};$item=Get-Item -LiteralPath $path;if($item.Length-ne$artifact.size){throw "Size mismatch: $($artifact.name)"};if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$artifact.sha256){throw "Hash mismatch: $($artifact.name)"}}
$expectedDependencyDocuments=@('THIRD_PARTY_NOTICES.txt','third-party-sbom.json');$actualDependencyDocuments=@($manifest.dependencyDocuments|ForEach-Object{$_.name})
if(@(Compare-Object $expectedDependencyDocuments $actualDependencyDocuments).Count){throw 'Release dependency document set mismatch'}
foreach($document in $manifest.dependencyDocuments){$path=Join-Path "$repoRoot\dist" $document.name;if(-not(Test-Path -LiteralPath $path)){throw "Missing release dependency document: $($document.name)"};$item=Get-Item -LiteralPath $path;if($item.Length-ne$document.size-or(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$document.sha256){throw "Release dependency document mismatch: $($document.name)"}}
$sbomSourcePath=Join-Path $repoRoot 'docs/compliance/third-party-sbom.json';$sbomReleasePath=Join-Path $repoRoot 'dist/third-party-sbom.json'
if(-not(Test-Path -LiteralPath $sbomSourcePath)-or-not(Test-Path -LiteralPath $sbomReleasePath)){throw 'Third-party SBOM is missing'}
$expectedSbom=ConvertTo-DeterministicText (Get-Content -LiteralPath $sbomSourcePath -Raw)
$releasedSbom=[IO.File]::ReadAllText($sbomReleasePath)
if($releasedSbom-ne$expectedSbom){throw 'Released SBOM does not match the normalized source'}
$sbom=Get-Content -LiteralPath $sbomSourcePath -Raw|ConvertFrom-Json
if($sbom.schemaVersion-ne'xtest-nova-third-party-sbom/v1'){throw 'Unsupported third-party SBOM schema'}
$expectedThirdParty=@('github.com/coder/websocket','github.com/creack/pty','scrcpy-server','Gradle','Android Gradle Plugin')
if(@($sbom.components).Count-ne$expectedThirdParty.Count-or@(Compare-Object $expectedThirdParty @($sbom.components.name)).Count){throw 'Third-party SBOM component set mismatch'}
$notice=Get-Content -LiteralPath (Join-Path $repoRoot 'dist/THIRD_PARTY_NOTICES.txt') -Raw
foreach($component in $expectedThirdParty){if($notice-notmatch[regex]::Escape($component)){throw "Third-party notice omits $component"}}
$goMod=Get-Content -LiteralPath (Join-Path $repoRoot 'agent/go.mod') -Raw
foreach($module in @($sbom.components|Where-Object{$_.purl-like'pkg:golang/*'})){if($goMod-notmatch("{0}\s+v{1}"-f[regex]::Escape($module.name),[regex]::Escape($module.version))){throw "Go dependency version does not match SBOM: $($module.name)"}}
$scrcpyComponent=$sbom.components|Where-Object{$_.name-eq'scrcpy-server'};$scrcpyPath=Join-Path $repoRoot $scrcpyComponent.artifact
if(-not(Test-Path -LiteralPath $scrcpyPath)-or(Get-FileHash -LiteralPath $scrcpyPath -Algorithm SHA256).Hash-ne$scrcpyComponent.artifactSha256){throw 'scrcpy artifact does not match SBOM'}
$wrapperProperties=Get-Content -LiteralPath (Join-Path $repoRoot 'uiautomator/gradle/wrapper/gradle-wrapper.properties') -Raw
if($wrapperProperties-notmatch'gradle-8\.14\.5-bin\.zip'-or$wrapperProperties-notmatch'6f74b601422d6d6fc4e1f9a1ab6522f642c2fdcbc15ae33ebd30ba3d7198e854'){throw 'Gradle wrapper does not match SBOM'}
$uiSource=@((Join-Path $repoRoot 'uiautomator/host/src'),(Join-Path $repoRoot 'uiautomator/test/src'),(Join-Path $repoRoot 'uiautomator/host/build.gradle'),(Join-Path $repoRoot 'uiautomator/test/build.gradle'),(Join-Path $repoRoot 'uiautomator/build.gradle'))|ForEach-Object{if(Test-Path -LiteralPath $_){Get-ChildItem -LiteralPath $_ -File -Recurse -ErrorAction SilentlyContinue|Where-Object{$_.Extension-in'.gradle','.java','.xml'}|Get-Content -Raw}}
if(($uiSource-join"`n")-match'androidx\.test\.uiautomator'){throw 'AndroidX UIAutomator runtime dependency returned'}
$bundle=$manifest.runtimeBundle;if(-not$bundle-or$bundle.schemaVersion-ne'xtest-nova-runtime-bundle/v1'-or@($bundle.components).Count-ne 4){throw 'Runtime bundle manifest is incomplete'}
$expectedComponents=@('runner','companion','uiautomatorHost','uiautomatorTest');if(@(Compare-Object $expectedComponents @($bundle.components.name)).Count){throw 'Runtime bundle component set mismatch'}
$bundleDir=Join-Path $repoRoot 'agent/internal/runtimebundle/assets';$certificates=@()
$expectedBundleFiles=@('manifest.json','xtest-nova-runner.jar','xtest-nova-companion.apk','xtest-nova-uiautomator-host.apk','xtest-nova-uiautomator-test.apk');$actualBundleFiles=@(Get-ChildItem -LiteralPath $bundleDir -File|ForEach-Object{$_.Name});if(@(Compare-Object $expectedBundleFiles $actualBundleFiles).Count){throw 'Runtime bundle directory contains undeclared files'}
foreach($component in $bundle.components){
    if($component.name-eq'fixture'-or$component.package-eq'com.xtest.nova.fixture'){throw 'Validation fixture must not be embedded'}
    $path=Join-Path $bundleDir $component.file;if(-not(Test-Path -LiteralPath $path)){throw "Missing embedded component: $($component.file)"};$item=Get-Item -LiteralPath $path
    if($item.Length-ne$component.size-or(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$component.sha256){throw "Embedded component mismatch: $($component.file)"}
    if($component.kind-eq'apk'){$actual=Get-ActualApkMetadata $path;if($actual.package-ne$component.package-or$actual.versionCode-ne$component.versionCode-or$actual.versionName-ne$component.versionName-or$actual.certificateSha256-ne$component.certificateSha256.ToUpperInvariant()){throw "Embedded APK metadata mismatch: $($component.file)"};$certificates+=$actual.certificateSha256}
}
if(@($certificates|Select-Object -Unique).Count-ne 1){throw 'Embedded APK signing certificates do not match'}
Push-Location $repoRoot
try {
    & (Join-Path $PSScriptRoot 'test-first-commit.ps1');if($LASTEXITCODE-ne 0){throw 'First-commit source gate failed'}
    & (Join-Path $PSScriptRoot 'test-build-release.ps1');if($LASTEXITCODE-ne 0){throw 'Build and release script tests failed'}
    if(-not(Test-Path -LiteralPath (Join-Path $repoRoot 'vendor/modules.txt'))){throw 'Offline Go vendor manifest is missing'}
    $previousGoProxy=$env:GOPROXY;$previousGoFlags=$env:GOFLAGS;$previousGoCache=$env:GOCACHE
    try{$env:GOCACHE=Join-Path $repoRoot '.gocache';New-Item -ItemType Directory -Force $env:GOCACHE|Out-Null;$env:GOPROXY='off';$env:GOFLAGS='-mod=vendor';go vet ./agent/...;if($LASTEXITCODE-ne 0){throw 'offline go vet failed'};go test ./agent/...;if($LASTEXITCODE-ne 0){throw 'offline go test failed'}}finally{$env:GOPROXY=$previousGoProxy;$env:GOFLAGS=$previousGoFlags;if($null-eq$previousGoCache){Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue}else{$env:GOCACHE=$previousGoCache}}
    $moduleFiles=@((Join-Path $repoRoot 'agent/go.mod'),(Join-Path $repoRoot 'agent/go.sum'),(Join-Path $repoRoot 'vendor/modules.txt'))
    $moduleMetadata=($moduleFiles|ForEach-Object{Get-Content -LiteralPath $_ -Raw})-join"`n"
    if($moduleMetadata-match'github\.com/shogo82148/androidbinary'){throw 'Retired androidbinary dependency returned to module metadata'}
    if(Test-Path -LiteralPath (Join-Path $repoRoot 'vendor/github.com/shogo82148/androidbinary')){throw 'Retired androidbinary vendor source returned'}
    if([string]::IsNullOrWhiteSpace($ReferenceContractPath)){throw 'ReferenceContractPath cannot be empty; contract checks are mandatory'}
    $resolvedReferenceContractPath=[IO.Path]::GetFullPath($ReferenceContractPath)
    if(-not(Test-Path -LiteralPath $resolvedReferenceContractPath -PathType Leaf)){throw "Reference contract not found: $resolvedReferenceContractPath"}
    go run -mod=vendor ./agent/cmd/contract-check -reference $resolvedReferenceContractPath -document (Join-Path $repoRoot 'docs/compliance/http-contract.md')
    if($LASTEXITCODE-ne 0){throw 'Contract or documentation check failed'}
    $scriptPaths=@(Get-ChildItem -LiteralPath $repoRoot -Filter '*.ps1' -File -Recurse|ForEach-Object{$_.FullName})
    foreach($scriptPath in $scriptPaths){
        $tokens=$null;$parseDiagnostics=$null;$absoluteScriptPath=if([IO.Path]::IsPathRooted($scriptPath)){$scriptPath}else{Join-Path $repoRoot $scriptPath}
        [System.Management.Automation.Language.Parser]::ParseInput([IO.File]::ReadAllText($absoluteScriptPath),$absoluteScriptPath,[ref]$tokens,[ref]$parseDiagnostics)|Out-Null
        if($parseDiagnostics.Count){throw "PowerShell parse failed: $scriptPath - $($parseDiagnostics[0].Message)"}
    }
    $matrix=Get-Content -LiteralPath (Join-Path $repoRoot 'docs/compliance/compatibility.md') -Raw;if($matrix-match'\|\s*partial\s*\|'){throw 'Compatibility matrix contains unexplained partial status'}
    if(-not(Test-Path -LiteralPath (Join-Path $repoRoot 'docs/compliance/compatibility-waivers.md'))){throw 'Compatibility waiver register is missing'}
    if($RequireCleanGit){$status=git -c "safe.directory=$gitRepoRoot" status --porcelain;if($status){throw 'Git working tree is not clean'}}
} finally {Pop-Location}
Write-Host "Release gate passed: $($manifest.agentVersion)"
