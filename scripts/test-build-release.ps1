$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
. (Join-Path $PSScriptRoot 'release-reproducibility.ps1')

foreach($script in Get-ChildItem -LiteralPath $repoRoot -Filter '*.ps1' -File -Recurse){
    $tokens=$null;$diagnostics=$null
    [Management.Automation.Language.Parser]::ParseInput([IO.File]::ReadAllText($script.FullName),$script.FullName,[ref]$tokens,[ref]$diagnostics)|Out-Null
    if($diagnostics.Count){throw "PowerShell parse failed: $($script.FullName) - $($diagnostics[0].Message)"}
}

$previousEpoch=$env:SOURCE_DATE_EPOCH
$temporary=Join-Path ([IO.Path]::GetTempPath()) ("xtest-release-text-"+[Guid]::NewGuid().ToString('N')+'.txt')
try{
    $env:SOURCE_DATE_EPOCH='0'
    if((Get-ReleaseTimestamp -RepoRoot $repoRoot)-ne'1970-01-01T00:00:00Z'){throw 'SOURCE_DATE_EPOCH was not applied exactly'}
    Remove-Item Env:SOURCE_DATE_EPOCH
    $previousErrorAction=$ErrorActionPreference
    try{$ErrorActionPreference='Continue';$commitEpoch=(& git -C $repoRoot log -1 --format=%ct 2>$null|Out-String).Trim();$gitExitCode=$LASTEXITCODE}finally{$ErrorActionPreference=$previousErrorAction}
    if($gitExitCode-eq 0-and-not[string]::IsNullOrWhiteSpace($commitEpoch)){
        $expectedCommitTime=[DateTimeOffset]::FromUnixTimeSeconds([int64]$commitEpoch).UtcDateTime.ToString('yyyy-MM-ddTHH:mm:ssZ',[Globalization.CultureInfo]::InvariantCulture)
        if((Get-ReleaseTimestamp -RepoRoot $repoRoot)-ne$expectedCommitTime){throw 'Git commit timestamp fallback was not applied'}
    }else{
        $before=[DateTime]::UtcNow.AddSeconds(-2)
        $fallback=[DateTimeOffset]::Parse((Get-ReleaseTimestamp -RepoRoot $repoRoot),[Globalization.CultureInfo]::InvariantCulture,[Globalization.DateTimeStyles]::AssumeUniversal).UtcDateTime
        if($fallback-lt$before-or$fallback-gt[DateTime]::UtcNow.AddSeconds(2)){throw 'Current UTC fallback was not applied for a repository without commits'}
    }
    Write-Utf8NoBomText -Path $temporary -Text "one`r`ntwo`rthree"
    $bytes=[IO.File]::ReadAllBytes($temporary)
    if($bytes.Length-ge 3-and$bytes[0]-eq 0xEF-and$bytes[1]-eq 0xBB-and$bytes[2]-eq 0xBF){throw 'Deterministic text writer emitted a UTF-8 BOM'}
    $text=[Text.Encoding]::UTF8.GetString($bytes)
    if($text.Contains("`r")-or$text-ne"one`ntwo`nthree`n"){throw 'Deterministic text writer did not normalize line endings'}
}finally{
    if($null-eq$previousEpoch){Remove-Item Env:SOURCE_DATE_EPOCH -ErrorAction SilentlyContinue}else{$env:SOURCE_DATE_EPOCH=$previousEpoch}
    Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
}

$signingSource=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'apk-signing.ps1') -Raw
if($signingSource-match'--ks-pass\s+["'']?pass:'-or$signingSource-match'--key-pass\s+["'']?pass:'){throw 'apksigner plaintext password syntax returned'}
if($signingSource-notmatch'--ks-pass\s+"env:\$storeVariable"'-or$signingSource-notmatch'--key-pass\s+"env:\$keyVariable"'){throw 'apksigner environment password syntax is missing'}
$buildSource=Get-Content -LiteralPath (Join-Path $repoRoot 'build.ps1') -Raw
if($buildSource-notmatch'\[switch\]\$Development'-or$buildSource-notmatch'Release builds require -KeyStore'){throw 'Root build does not enforce explicit release or development mode'}

$deployPath=Join-Path $repoRoot 'deploy.ps1'
$deployTokens=$null;$deployDiagnostics=$null
$deployAst=[Management.Automation.Language.Parser]::ParseInput([IO.File]::ReadAllText($deployPath),$deployPath,[ref]$deployTokens,[ref]$deployDiagnostics)
if($deployDiagnostics.Count){throw "Deploy PowerShell parse failed: $($deployDiagnostics[0].Message)"}
$deploySource=$deployAst.Extent.Text
$parameterNames=@($deployAst.ParamBlock.Parameters|ForEach-Object{$_.Name.VariablePath.UserPath})
foreach($requiredParameter in @('AllowLAN','ApiTokenFile')){if($requiredParameter-notin$parameterNames){throw "Deploy LAN parameter is missing: $requiredParameter"}}
$functions=@{};$deployAst.FindAll({param($node)$node-is[Management.Automation.Language.FunctionDefinitionAst]},$true)|ForEach-Object{$functions[$_.Name]=$_.Body.Extent.Text}
if(-not$functions.ContainsKey('Start-Agent')-or$functions['Start-Agent']-notmatch"'--allow-lan','--listen','0\.0\.0\.0:7912','--api-token-file',\`$script:remoteTokenPath"){throw 'Deploy LAN Agent argv is incomplete'}
if($functions['Start-Agent']-match'\$script:apiToken|\$ApiTokenFile'){throw 'Deploy Agent argv can contain host token data or its host path'}
if($deploySource-notmatch"ContainsKey\('ApiTokenFile'\)-and-not\`$AllowLAN"-or$deploySource-notmatch"\`$Rollback-and\(\`$AllowLAN-or\`$PSBoundParameters\.ContainsKey\('ApiTokenFile'\)\)"){throw 'Deploy LAN option exclusions are incomplete'}
if($functions['Start-Agent']-notmatch'\$LegacyUnsafeAPI'-or$functions['Start-Agent']-notmatch'\$UseLAN'){throw 'Deploy cannot combine authenticated LAN access with legacy unsafe APIs'}
if($functions['Wait-Health']-notmatch'Headers=@\{Authorization="Bearer \$script:apiToken"\}'){throw 'Deploy LAN health requests do not use Bearer authentication'}
if($functions['Write-DeviceEndpoint']-notmatch'http://\$\{wlanIPv4\}:7912/'-or$functions['Write-DeviceEndpoint']-match'\$script:apiToken'){throw 'Deploy LAN URL output is missing or exposes token material'}
if($deploySource-notmatch'\[Guid\]::NewGuid\(\)'-or$deploySource-notmatch"Push-Verified \`$tokenItem\.FullName \`$tokenStaged"-or$deploySource-notmatch"'chmod','600',\`$tokenStaged"-or$deploySource-notmatch"'mv','-f',\`$tokenStaged,\`$script:remoteTokenPath"-or$deploySource-notmatch"'mv','-f',\`$tokenBackup,\`$script:remoteTokenPath"){throw 'Deploy token staging, permissions, atomic switch, or rollback is incomplete'}
if(([regex]::Matches($deploySource,'\$script:apiToken=\$null')).Count-lt 2){throw 'Deploy does not clear plaintext token state on success and failure'}
$outputCommands=$deployAst.FindAll({param($node)$node-is[Management.Automation.Language.CommandAst]-and$node.GetCommandName()-in@('Write-Host','Write-Warning','Write-Output')},$true)
if(@($outputCommands|Where-Object{$_.Extent.Text-match'\$script:apiToken|\$ApiTokenFile|\$tokenItem'}).Count){throw 'Deploy output can expose API token material'}
$daemonSource=Get-Content -LiteralPath (Join-Path $repoRoot 'agent/cmd/xtest-nova-agent/daemon_unix.go') -Raw
if($daemonSource-notmatch'request\.Header\.Set\("Authorization", "Bearer "\+string\(apiToken\)\)'){throw 'Daemon LAN health probe does not use Bearer authentication'}

$generatedTokenPath=Join-Path ([IO.Path]::GetTempPath()) ('xtest-nova-token-'+[Guid]::NewGuid().ToString('N'))
try{
    & (Join-Path $PSScriptRoot 'new-lan-token.ps1') -OutputPath $generatedTokenPath|Out-Null
    $generatedTokenBytes=[IO.File]::ReadAllBytes($generatedTokenPath)
    if($generatedTokenBytes.Length-ge 3-and$generatedTokenBytes[0]-eq0xEF-and$generatedTokenBytes[1]-eq0xBB-and$generatedTokenBytes[2]-eq0xBF){throw 'LAN token generator emitted a UTF-8 BOM'}
    $generatedToken=[Text.Encoding]::UTF8.GetString($generatedTokenBytes)
    if([Convert]::FromBase64String($generatedToken).Length-ne 32){throw 'LAN token generator did not produce 32 random bytes'}
    try{& (Join-Path $PSScriptRoot 'new-lan-token.ps1') -OutputPath $generatedTokenPath|Out-Null;throw 'LAN token generator overwrote an existing token without -Force'}catch{if($_.Exception.Message-notmatch'Token file already exists'){throw}}
}finally{
    $generatedToken=$null
    Remove-Item -LiteralPath $generatedTokenPath -Force -ErrorAction SilentlyContinue
}

$bundleDir=Join-Path $repoRoot 'agent\internal\runtimebundle\assets'
$bundle=Get-Content -LiteralPath (Join-Path $bundleDir 'manifest.json') -Raw|ConvertFrom-Json
foreach($component in @($bundle.components)){
    $path=Join-Path $bundleDir $component.file
    if(-not(Test-Path -LiteralPath $path)-or(Get-Item -LiteralPath $path).Length-ne$component.size-or(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash-ne$component.sha256){throw "Checked-in runtime bundle metadata is stale: $($component.file)"}
}
$global:LASTEXITCODE=0
Write-Host 'Build and release script tests passed'
