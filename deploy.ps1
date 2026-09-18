param(
    [string]$Serial,
    [switch]$StartServer,
    [switch]$ReplaceRunningAgent,
    [switch]$Rollback,
    [switch]$LegacyUnsafeAPI,
    [switch]$AllowLAN,
    [string]$ApiTokenFile,
    [switch]$LegacyUiAutomator,
    [switch]$NoPopup,
    [ValidateSet('system','shadow','nova')][string]$HierarchyProvider='system',
    [switch]$ForwardCompanion,
    [switch]$ForwardMonitor,
    [ValidateRange(0,65535)][int]$LocalAgentPort=0,
    [ValidateRange(0,65535)][int]$LocalCompanionPort=0,
    [ValidateRange(0,65535)][int]$LocalMonitorPort=0,
    [string]$ReleaseManifestPath="$PSScriptRoot\dist\release-manifest.json",
    [switch]$AllowUnmanifestedArtifacts,
    [Parameter(DontShow)][ValidateSet('','after-runtime-switch')][string]$FailureInjection=''
)
$ErrorActionPreference='Stop'

if($PSBoundParameters.ContainsKey('ApiTokenFile')-and-not$AllowLAN){throw '-ApiTokenFile is only valid with -AllowLAN'}
if($Rollback-and($AllowLAN-or$PSBoundParameters.ContainsKey('ApiTokenFile'))){throw '-Rollback cannot be combined with LAN deployment options; the previous Agent may not support them'}
$script:apiToken=$null
if($AllowLAN){
    if([string]::IsNullOrWhiteSpace($ApiTokenFile)){throw '-ApiTokenFile is required with -AllowLAN'}
    $tokenItem=Get-Item -LiteralPath $ApiTokenFile -Force -ErrorAction Stop
    if($tokenItem-isnot[IO.FileInfo]-or($tokenItem.Attributes-band[IO.FileAttributes]::ReparsePoint)){throw 'API token path must be a regular file'}
    $script:apiToken=[IO.File]::ReadAllText($tokenItem.FullName).Trim()
    $tokenBytes=[Text.Encoding]::UTF8.GetByteCount($script:apiToken)
    if($tokenBytes-lt 32-or$tokenBytes-gt 4KB){throw 'API token must contain 32 bytes through 4 KiB after trimming whitespace'}
}
$script:remoteTokenPath='/data/local/tmp/.xtest-nova-api-token'

function Resolve-Serial {
    if($Serial){return $Serial}
    $online=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_-match'^([^\s]+)\s+device$'){$matches[1]}})
    if($online.Count-ne 1){throw "Specify -Serial when online device count is $($online.Count)"}
    return $online[0]
}
function Invoke-Adb([string[]]$Arguments){$output=& adb -s $script:deviceSerial @Arguments;if($LASTEXITCODE-ne 0){throw "adb failed: $Arguments"};return $output}
function Test-RemoteFile([string]$Path){& adb -s $script:deviceSerial shell test -f $Path 2>$null;return $LASTEXITCODE-eq 0}
function Get-AgentPid {$value=(& adb -s $script:deviceSerial shell pidof xtest-nova-agent 2>$null|Out-String).Trim();if($LASTEXITCODE-ne 0){return ''};return $value}
function Stop-Agent {
    & adb -s $script:deviceSerial shell /data/local/tmp/xtest-nova-agent server -d --stop 2>$null|Out-Null
    if($LASTEXITCODE-ne 0){foreach($value in((Get-AgentPid)-split'\s+')){if($value-match'^\d+$'){& adb -s $script:deviceSerial shell kill $value 2>$null|Out-Null}}}
}
function Get-RemoteHash([string]$Path){$text=(Invoke-Adb @('shell','sha256sum',$Path)|Out-String).Trim();if($text-notmatch'^([a-fA-F0-9]{64})\s'){throw "Unable to verify remote hash: $Path"};return $matches[1].ToUpperInvariant()}
function Push-Verified([string]$Local,[string]$Remote){Invoke-Adb @('push',$Local,$Remote)|Out-Null;$expected=(Get-FileHash -LiteralPath $Local -Algorithm SHA256).Hash;if((Get-RemoteHash $Remote)-ne$expected){throw "Remote hash mismatch: $Remote"}}
function Start-Agent([bool]$UseLAN=$AllowLAN) {
    $arguments=@('shell','/data/local/tmp/xtest-nova-agent','server','-d')
    if($LegacyUnsafeAPI){$arguments+='--legacy-unsafe-api'}
    if($LegacyUiAutomator){$arguments+='--legacy-uiautomator'}
    if($NoPopup){$arguments+='--no-popup'}
    if($HierarchyProvider-ne'system'){$arguments+=@('--hierarchy-provider',$HierarchyProvider)}
    if($UseLAN){$arguments+=@('--allow-lan','--listen','0.0.0.0:7912','--api-token-file',$script:remoteTokenPath)}
    Invoke-Adb $arguments|Out-Null
}
function Add-Forward {
    if($LocalAgentPort-eq 0){$value=(Invoke-Adb @('forward','tcp:0','tcp:7912')|Out-String).Trim();if($value-notmatch'^\d+$'){throw 'adb did not allocate a local port'};$script:agentForwardOwned=$true;return [int]$value}
    $existing=@(& adb forward --list)
    $same=@($existing|Where-Object{$_-match "^$([regex]::Escape($script:deviceSerial))\s+tcp:$LocalAgentPort\s+tcp:7912$"})
    if($same.Count-gt 0){$script:agentForwardOwned=$false;return $LocalAgentPort}
    $occupied=@($existing|Where-Object{$_-match "\s+tcp:$LocalAgentPort\s+"})
    if($occupied.Count-gt 0){throw "Local agent port $LocalAgentPort is already forwarded for another device or service"}
    Invoke-Adb @('forward','--no-rebind',"tcp:$LocalAgentPort",'tcp:7912')|Out-Null;$script:agentForwardOwned=$true;return $LocalAgentPort
}
function Add-ServiceForward([int]$RequestedPort,[int]$RemotePort){if($RequestedPort-eq 0){$value=(Invoke-Adb @('forward','tcp:0',"tcp:$RemotePort")|Out-String).Trim();if($value-notmatch'^\d+$'){throw "adb did not allocate a local port for $RemotePort"};return [int]$value};Invoke-Adb @('forward','--no-rebind',"tcp:$RequestedPort","tcp:$RemotePort")|Out-Null;return $RequestedPort}
function Add-OptionalForwards {if($ForwardCompanion){$script:companionPort=Add-ServiceForward $LocalCompanionPort 8912;$script:companionForwardOwned=$true};if($ForwardMonitor){$script:monitorPort=Add-ServiceForward $LocalMonitorPort 7890;$script:monitorForwardOwned=$true}}
function Remove-DeploymentForwards {if($script:agentForwardOwned-and$script:port-gt 0){& adb -s $script:deviceSerial forward --remove "tcp:$($script:port)" 2>$null|Out-Null;$script:agentForwardOwned=$false};if($script:companionForwardOwned-and$script:companionPort-gt 0){& adb -s $script:deviceSerial forward --remove "tcp:$($script:companionPort)" 2>$null|Out-Null;$script:companionForwardOwned=$false};if($script:monitorForwardOwned-and$script:monitorPort-gt 0){& adb -s $script:deviceSerial forward --remove "tcp:$($script:monitorPort)" 2>$null|Out-Null;$script:monitorForwardOwned=$false}}
function Get-DeviceWLANIPv4 {
    $addresses=@()
    foreach($probe in @(
        @('shell','ip','-4','-o','addr','show','scope','global'),
        @('shell','ip','-4','addr','show','wlan0'),
        @('shell','getprop','dhcp.wlan0.ipaddress')
    )){
        try{
            foreach($line in @(Invoke-Adb $probe)){
                if($line-match'^\d+:\s+([^\s:]+).*?\sinet\s+((?:\d{1,3}\.){3}\d{1,3})/'){$addresses+=[pscustomobject]@{Interface=$matches[1];Address=$matches[2]}}
                elseif($line-match'^\s*inet\s+((?:\d{1,3}\.){3}\d{1,3})/'){$addresses+=[pscustomobject]@{Interface='wlan0';Address=$matches[1]}}
                elseif($line.Trim()-match'^((?:\d{1,3}\.){3}\d{1,3})$'){$addresses+=[pscustomobject]@{Interface='wlan0';Address=$matches[1]}}
            }
        }catch{}
    }
    foreach($selected in @($addresses|Sort-Object @{Expression={if($_.Interface-match'^(wlan|wifi)'){0}else{1}}})){
        $parsed=$null
        if($selected-and[Net.IPAddress]::TryParse($selected.Address,[ref]$parsed)-and-not$parsed.IsLoopback){return $selected.Address}
    }
    return ''
}
function Write-DeviceEndpoint([int]$AgentPort){
    $model=(Invoke-Adb @('shell','getprop','ro.product.model')|Out-String).Trim()
    Write-Host "Web console (ADB forward): http://127.0.0.1:$AgentPort/" -ForegroundColor Cyan
    Write-Host "Device: serial=$script:deviceSerial model=$model; the ADB recovery forward remains active."
    Write-Host "Health endpoint: http://127.0.0.1:$AgentPort/v1/health"
    if($AllowLAN){$wlanIPv4=Get-DeviceWLANIPv4;if($wlanIPv4){Write-Host "Web console (device WLAN): http://${wlanIPv4}:7912/" -ForegroundColor Cyan;Write-Host 'LAN requests require Authorization: Bearer authentication; the token is not displayed.'}else{Write-Warning 'Unable to determine a reliable device WLAN IPv4 address; the ADB forward remains available.'}}
    if($script:companionPort-gt 0){Write-Host "Companion endpoint: http://127.0.0.1:$($script:companionPort)/"}
    if($script:monitorPort-gt 0){Write-Host "Monitor endpoint: http://127.0.0.1:$($script:monitorPort)/"}
}
function Wait-Health([int]$Port,[string]$ExpectedVersion,[bool]$UseLAN=$AllowLAN){
    $required=@('runtimeBootstrap','runnerPayload','companionPayload','uiautomatorHostPayload','uiautomatorTestPayload')
    if(-not$NoPopup){$required+='popupOverlayStartup'}
    $requestParameters=@{TimeoutSec=3}
    if($UseLAN){$requestParameters.Headers=@{Authorization="Bearer $script:apiToken"}}
    $popupStableSince=$null
    $deadline=[DateTime]::UtcNow.AddSeconds(120)
    do{
        try{
            $health=Invoke-RestMethod "http://127.0.0.1:$Port/v1/health" @requestParameters
            $components=Invoke-RestMethod "http://127.0.0.1:$Port/v1/diagnostics/components" @requestParameters
            $ready=@($components.components|Where-Object{$_.ready}|ForEach-Object{$_.name})
            $missing=@($required|Where-Object{$_-notin$ready})
            $popupStable=$true
            if(-not$NoPopup){
                $popupStatus=(Invoke-Adb @('shell','/data/local/tmp/xtest-nova-agent','popup','status')|Out-String)
                if($popupStatus-notmatch'installed=true'-or$popupStatus-notmatch'running=true'){$popupStableSince=$null;$popupStable=$false}
                elseif($null-eq$popupStableSince){$popupStableSince=[DateTime]::UtcNow;$popupStable=$false}
                else{$popupStable=([DateTime]::UtcNow-$popupStableSince).TotalSeconds-ge 4}
            }
            if($health.status-eq'ok'-and(!$ExpectedVersion-or$health.version-eq$ExpectedVersion)-and$missing.Count-eq 0-and$popupStable){return $health}
        }catch{}
        Start-Sleep -Milliseconds 250
    }while([DateTime]::UtcNow-lt$deadline)
    throw 'Deployed Agent did not report every bundled runtime component ready'
}
function Restore-PreviousRuntime {Stop-Agent;if(Test-RemoteFile '/data/local/tmp/xtest-nova-agent.previous'){Invoke-Adb @('shell','cp','-f','/data/local/tmp/xtest-nova-agent.previous','/data/local/tmp/xtest-nova-agent')|Out-Null;Invoke-Adb @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null}}
function Restore-DeploymentRuntime {Stop-Agent;if($script:agentExisted){Invoke-Adb @('shell','cp','-f','/data/local/tmp/xtest-nova-agent.previous','/data/local/tmp/xtest-nova-agent')|Out-Null;Invoke-Adb @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null}else{Invoke-Adb @('shell','rm','-f','/data/local/tmp/xtest-nova-agent')|Out-Null}}

$deviceSerial=Resolve-Serial
if($deviceSerial-notmatch'^[A-Za-z0-9._:-]+$'){throw 'Invalid serial'}
$existingPid=Get-AgentPid;$agentExisted=Test-RemoteFile '/data/local/tmp/xtest-nova-agent'
if($existingPid-and-not$ReplaceRunningAgent-and-not$Rollback){throw 'Agent is running; use -ReplaceRunningAgent to authorize replacement'}
$port=0;$companionPort=0;$monitorPort=0;$agentForwardOwned=$false;$companionForwardOwned=$false;$monitorForwardOwned=$false
$tokenStaged='';$tokenBackup='';$tokenExisted=$false;$tokenSwitchStarted=$false
if($Rollback){
    if(-not(Test-RemoteFile '/data/local/tmp/xtest-nova-agent.previous')){throw 'No previous Agent is available for rollback'}
    if(Test-RemoteFile '/data/local/tmp/xtest-nova-agent'){Invoke-Adb @('shell','cp','-f','/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.failed')|Out-Null}
    Restore-PreviousRuntime
    if($StartServer){Start-Agent;$port=Add-Forward;Add-OptionalForwards;$health=Wait-Health $port '';Write-DeviceEndpoint $port;Write-Host "Rolled back and healthy: $($health.version) on local port $port"}else{Write-Host 'Rolled back Agent; runtime components reconcile on next start'}
    exit 0
}
$abi=(Invoke-Adb @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
switch -Regex($abi){'^arm64'{$agent="$PSScriptRoot\dist\xtest-nova-agent-arm64";break};'^armeabi-v7a$'{$agent="$PSScriptRoot\dist\xtest-nova-agent-armv7";break};default{throw "Unsupported ABI: $abi"}}
if(-not(Test-Path -LiteralPath $agent)){throw "Missing self-contained Agent: $agent"}
$versionSource=Get-Content -LiteralPath "$PSScriptRoot\agent\internal\buildinfo\version.go"|Out-String
$versionMatch=[regex]::Match($versionSource,'const Version = "(.+)"')
if(-not $versionMatch.Success){throw 'Unable to determine expected Agent version'}
$expectedVersion=$versionMatch.Groups.Item(1).Value
if(-not$AllowUnmanifestedArtifacts){
    if(-not(Test-Path -LiteralPath $ReleaseManifestPath)){throw 'Release manifest is required'}
    $manifest=Get-Content -LiteralPath $ReleaseManifestPath -Raw|ConvertFrom-Json
    if($manifest.schemaVersion-ne'xtest-nova-release/v2'-or$manifest.agentVersion-ne$expectedVersion-or@($manifest.artifacts).Count-ne 2){throw 'Single-binary release manifest is incomplete or mismatched'}
    $metadata=@($manifest.artifacts|Where-Object{$_.name-eq(Split-Path -Leaf $agent)});if($metadata.Count-ne 1){throw 'Release manifest does not contain the selected Agent'}
    $item=Get-Item -LiteralPath $agent;if($item.Length -ne $metadata[0].size -or (Get-FileHash -LiteralPath $agent -Algorithm SHA256).Hash -ne $metadata[0].sha256){throw 'Self-contained Agent does not match release manifest'}
}
$switchStarted=$false
try{
    Push-Verified $agent '/data/local/tmp/xtest-nova-agent.staged'
    if($AllowLAN){
        $transactionId=[Guid]::NewGuid().ToString('N')
        $tokenStaged="/data/local/tmp/.xtest-nova-api-token.staged.$transactionId"
        $tokenBackup="/data/local/tmp/.xtest-nova-api-token.backup.$transactionId"
        Push-Verified $tokenItem.FullName $tokenStaged
        Invoke-Adb @('shell','chmod','600',$tokenStaged)|Out-Null
        $tokenExisted=Test-RemoteFile $script:remoteTokenPath
    }
    if($agentExisted){Invoke-Adb @('shell','cp','-f','/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.previous')|Out-Null}else{Invoke-Adb @('shell','rm','-f','/data/local/tmp/xtest-nova-agent.previous')|Out-Null}
    $switchStarted=$true;if($existingPid){Stop-Agent;Start-Sleep -Milliseconds 300}
    Invoke-Adb @('shell','mv','-f','/data/local/tmp/xtest-nova-agent.staged','/data/local/tmp/xtest-nova-agent')|Out-Null;Invoke-Adb @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null
    if($AllowLAN){
        if($tokenExisted){Invoke-Adb @('shell','cp','-p',$script:remoteTokenPath,$tokenBackup)|Out-Null}
        $tokenSwitchStarted=$true
        Invoke-Adb @('shell','mv','-f',$tokenStaged,$script:remoteTokenPath)|Out-Null
        Invoke-Adb @('shell','chmod','600',$script:remoteTokenPath)|Out-Null
        if((Get-RemoteHash $script:remoteTokenPath)-ne(Get-FileHash -LiteralPath $tokenItem.FullName -Algorithm SHA256).Hash){throw 'Installed API token hash mismatch'}
    }
    if($FailureInjection-eq'after-runtime-switch'){throw 'Injected deployment failure after runtime switch'}
    if($StartServer){Start-Agent;$port=Add-Forward;Add-OptionalForwards;$health=Wait-Health $port $expectedVersion;Write-DeviceEndpoint $port;Write-Host "Healthy with complete bundled runtime: $($health.version) on local port $port"}
    $script:apiToken=$null
    if($tokenBackup){Invoke-Adb @('shell','rm','-f',$tokenBackup)|Out-Null}
    $retainedToken=(-not $AllowLAN) -and (Test-RemoteFile $script:remoteTokenPath)
    if($retainedToken){
        Write-Host 'An existing LAN token file was retained. Without LAN flags it does not enable authentication or network listening.'
    }
    Write-Host "Deployed one self-contained Agent for ABI $abi"
}catch{
    $failure=$_;$script:apiToken=$null;Remove-DeploymentForwards
    $rollbackErrors=@();$tokenRollbackFailed=$false
    if($tokenSwitchStarted){try{if($tokenExisted){Invoke-Adb @('shell','mv','-f',$tokenBackup,$script:remoteTokenPath)|Out-Null;Invoke-Adb @('shell','chmod','600',$script:remoteTokenPath)|Out-Null}else{Invoke-Adb @('shell','rm','-f',$script:remoteTokenPath)|Out-Null}}catch{$tokenRollbackFailed=$true;$rollbackErrors+="API token rollback failed: $($_.Exception.Message)"}}
    if($switchStarted){try{Restore-DeploymentRuntime;if($existingPid){Start-Agent $false;$port=Add-Forward;try{Wait-Health $port '' $false|Out-Null}finally{Remove-DeploymentForwards}}}catch{$rollbackErrors+="Agent rollback failed: $($_.Exception.Message)"}}
    & adb -s $deviceSerial shell rm -f /data/local/tmp/xtest-nova-agent.staged 2>$null|Out-Null
    if($tokenStaged){& adb -s $deviceSerial shell rm -f $tokenStaged 2>$null|Out-Null}
    if($tokenBackup-and-not$tokenRollbackFailed){& adb -s $deviceSerial shell rm -f $tokenBackup 2>$null|Out-Null}
    if($rollbackErrors.Count){throw "$($failure.Exception.Message); $($rollbackErrors-join'; ')"}
    throw $failure
}
