param(
    [string[]]$Serial,
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\runner-latest.json'),
    [string]$FixturePath=(Join-Path $PSScriptRoot '..\..\dist\xtest-nova-validation.apk')
)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
$fixturePackage='com.xtest.nova.fixture';$fixturePath=$FixturePath
$remoteAgent='/data/local/tmp/xtest-nova-agent';$remoteRunner='/data/local/tmp/xtest-nova-runner.jar';$remoteLog='/data/local/tmp/xtest-nova-monkey.log';$remoteStop='/data/local/tmp/.xtest-nova-monkey.stop'
function Invoke-Adb([string]$DeviceSerial,[string[]]$Arguments){$output=& adb -s $DeviceSerial @Arguments;if($LASTEXITCODE -ne 0){throw "adb failed for ${DeviceSerial}: $Arguments"};return $output}
function Get-AgentPid([string]$DeviceSerial){$value=(& adb -s $DeviceSerial shell pidof xtest-nova-agent 2>$null|Out-String).Trim();if($LASTEXITCODE -ne 0){return ''};return $value}
function Test-RemoteFile([string]$DeviceSerial,[string]$Path){& adb -s $DeviceSerial shell test -s $Path 2>$null;return $LASTEXITCODE-eq 0}
function Stop-Agent([string]$DeviceSerial){foreach($value in((Get-AgentPid $DeviceSerial)-split'\s+')){if($value-match'^\d+$'){& adb -s $DeviceSerial shell kill $value 2>$null|Out-Null}}}
function Add-Forward([string]$DeviceSerial){$value=(Invoke-Adb $DeviceSerial @('forward','tcp:0','tcp:7912')|Out-String).Trim();if($value-notmatch'^\d+$'){throw 'adb did not allocate a local port'};return [int]$value}
function Wait-Health([int]$Port){$deadline=[DateTime]::UtcNow.AddSeconds(12);do{try{return Invoke-RestMethod "http://127.0.0.1:$Port/v1/health" -TimeoutSec 3}catch{Start-Sleep -Milliseconds 250}}while([DateTime]::UtcNow-lt$deadline);throw 'Agent health timeout'}
function Get-Foreground([int]$Port){try{return (Invoke-RestMethod "http://127.0.0.1:$Port/foregroundPkg" -TimeoutSec 10).package}catch{return ''}}
function Restore-Foreground([string]$DeviceSerial,[string]$Package){if($Package-notmatch'^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$'){return};$activity=@(Invoke-Adb $DeviceSerial @('shell','cmd','package','resolve-activity','--brief',$Package)|Where-Object{$_-match'^[A-Za-z0-9_.]+/[A-Za-z0-9_.$]+$'}|Select-Object -Last 1);if($activity){Invoke-Adb $DeviceSerial @('shell','am','start','-n',$activity[0])|Out-Null}}
function Add-Step($Steps,[string]$Name,[bool]$Passed,[string]$Detail){$Steps.Add([pscustomobject]@{name=$Name;passed=$Passed;detail=$Detail})}
if(-not$Serial){$Serial=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_-match'^([^\s]+)\s+device$'){$matches[1]}})}
if(-not$Serial){throw 'No online Android devices found'}
if(-not(Test-Path -LiteralPath $fixturePath)){throw 'Missing signed validation fixture'}
$results=@()
foreach($deviceSerial in $Serial){
    $port=0;$owned=$false;$fixtureInstalled=$false;$originalForeground='';$steps=[System.Collections.Generic.List[object]]::new();$result=[ordered]@{serial=$deviceSerial;sdk=0;abi='';fixtureVersionCode=0;version='';passed=$false;steps=$steps}
    try{
        if($deviceSerial-notmatch'^[A-Za-z0-9._:-]+$'){throw 'Invalid serial'}
        $result.sdk=[int]((Invoke-Adb $deviceSerial @('shell','getprop','ro.build.version.sdk')|Out-String).Trim());$result.abi=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
        if(Get-AgentPid $deviceSerial){throw 'Existing Agent found; refusing to replace it'}
        Assert-ValidationRemotePathsAbsent $deviceSerial @($remoteAgent,$remoteRunner,$remoteLog,$remoteStop,'/data/local/tmp/xtest-nova-agent.log')
        Assert-ValidationRuntimeAbsent $deviceSerial
        if((& adb -s $deviceSerial shell pm path $fixturePackage 2>$null|Out-String).Trim()){throw 'Validation fixture already installed'}
$agentPath=if($result.abi-match'^arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}elseif($result.abi-eq'armeabi-v7a'){"$repoRoot\dist\xtest-nova-agent-armv7"}else{throw "Unsupported ABI: $($result.abi)"}
        Invoke-Adb $deviceSerial @('install','-r',$fixturePath)|Out-Null;$fixtureInstalled=$true
        $fixtureDump=(Invoke-Adb $deviceSerial @('shell','dumpsys','package',$fixturePackage)|Out-String);$fixtureVersionMatch=[regex]::Match($fixtureDump,'versionCode=(\d+)')
        if($fixtureVersionMatch.Success){$result.fixtureVersionCode=[int]$fixtureVersionMatch.Groups[1].Value}
        Invoke-Adb $deviceSerial @('push',$agentPath,$remoteAgent)|Out-Null;Invoke-Adb $deviceSerial @('shell','chmod','755',$remoteAgent)|Out-Null;Invoke-Adb $deviceSerial @('shell','rm','-f',$remoteLog,$remoteStop)|Out-Null
        Invoke-Adb $deviceSerial @('shell','sh','-c',"'nohup $remoteAgent server --no-popup >/data/local/tmp/xtest-nova-agent.log 2>&1 &'")|Out-Null;$owned=$true;$port=Add-Forward $deviceSerial;$health=Wait-Health $port;$result.version=$health.version;$base="http://127.0.0.1:$port";$originalForeground=Get-Foreground $port
        $requestId="m55-$deviceSerial";$body=@{requestId=$requestId;package=$fixturePackage;durationSeconds=30;throttleMillis=250;seed=20260903}|ConvertTo-Json
        $response=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $body -SkipHttpErrorCheck -TimeoutSec 15;$started=$response.Content|ConvertFrom-Json
        if($response.StatusCode-ne 202-or-not$started.running-or$started.pid-le 0){throw 'Runner did not enter running state'};Add-Step $steps 'runner-start' $true "pid=$($started.pid)"
        $repeat=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $body -SkipHttpErrorCheck -TimeoutSec 15;$repeatState=$repeat.Content|ConvertFrom-Json
        if($repeat.StatusCode-ne 202-or$repeatState.pid-ne$started.pid){throw 'Runner idempotency check failed'};Add-Step $steps 'runner-idempotency' $true 'same request kept the original process'
        $conflictBody=@{requestId="$requestId-conflict";package=$fixturePackage;durationSeconds=30;throttleMillis=250;seed=7}|ConvertTo-Json;$conflict=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $conflictBody -SkipHttpErrorCheck -TimeoutSec 15
        if($conflict.StatusCode-ne 409){throw 'Concurrent Runner request was not rejected'};Add-Step $steps 'runner-single-owner' $true 'concurrent request returned HTTP 409'
        Start-Sleep -Seconds 3;Stop-XTestOwnedExecution $base '/v1/monkey/runs/current' $started 15|Out-Null;$deadline=[DateTime]::UtcNow.AddSeconds(8)
        do{$state=Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10;if(-not$state.running-and-not$state.finalizing){break};Start-Sleep -Milliseconds 200}while([DateTime]::UtcNow-lt$deadline)
        if($state.running-or$state.finalizing){throw 'Runner remained active or finalizing after stop'};Add-Step $steps 'runner-stop' $true 'process stopped and artifact finalization completed'
        $logText=(Invoke-Adb $deviceSerial @('shell','cat',$remoteLog)|Out-String).Trim();$events=@($logText-split"`r?`n"|ForEach-Object{try{$_|ConvertFrom-Json}catch{}});$startedLog=$events|Where-Object{$_.state-eq'started'-and$_.package-eq$fixturePackage}|Select-Object -Last 1;$stoppedLog=$events|Where-Object{$_.state-eq'stopped'-and$_.package-eq$fixturePackage}|Select-Object -Last 1
        if(-not$startedLog-or-not$stoppedLog-or$stoppedLog.events-lt 1){throw 'Runner lifecycle log is incomplete'};$sizeBefore=[int64]((Invoke-Adb $deviceSerial @('shell','stat','-c','%s',$remoteLog)|Out-String).Trim());Start-Sleep -Seconds 1;$sizeAfter=[int64]((Invoke-Adb $deviceSerial @('shell','stat','-c','%s',$remoteLog)|Out-String).Trim())
        if($sizeBefore-ne$sizeAfter){throw 'Runner log changed after stop'};Add-Step $steps 'runner-log-stop-gate' $true "events=$($stoppedLog.events), bytes=$sizeAfter"
        $crashId="m59-crash-$deviceSerial";$crashBody=@{requestId=$crashId;package=$fixturePackage;durationSeconds=30;throttleMillis=250;seed=20260912}|ConvertTo-Json
        $crashStart=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $crashBody -SkipHttpErrorCheck -TimeoutSec 15;$crashState=$crashStart.Content|ConvertFrom-Json
        if($crashStart.StatusCode-ne 202-or-not$crashState.running){throw 'Crash diagnostics Runner did not start'};Start-Sleep -Seconds 2;Invoke-Adb $deviceSerial @('shell','am','crash',$fixturePackage)|Out-Null
        $crashDeadline=[DateTime]::UtcNow.AddSeconds(12);do{$crashState=Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10;if(-not$crashState.running-and-not$crashState.finalizing){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$crashDeadline)
        if($crashState.running){Stop-XTestOwnedExecution $base '/v1/monkey/runs/current' $crashState 10|Out-Null;$crashDeadline=[DateTime]::UtcNow.AddSeconds(8);do{$crashState=Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10;if(-not$crashState.running-and-not$crashState.finalizing){break};Start-Sleep -Milliseconds 200}while([DateTime]::UtcNow-lt$crashDeadline)}
        if($crashState.running-or$crashState.finalizing-or$crashState.crashCount-lt 1-or$crashState.failureType-ne'app_crash'){throw "VM crash was not classified: running=$($crashState.running), finalizing=$($crashState.finalizing), crashes=$($crashState.crashCount), failure=$($crashState.failureType)"}
        if(-not(Test-RemoteFile $deviceSerial "$($crashState.artifactDir)/crash.json")-or-not(Test-RemoteFile $deviceSerial "$($crashState.artifactDir)/failure.png")){throw 'VM crash evidence files are incomplete'};Add-Step $steps 'java-crash-diagnostics' $true "crashes=$($crashState.crashCount), failureType=$($crashState.failureType)"
        if($result.fixtureVersionCode-ge 2){
            $anrId="m59-anr-$deviceSerial";$anrBody=@{requestId=$anrId;package=$fixturePackage;durationSeconds=30;throttleMillis=250;seed=20260913}|ConvertTo-Json
            $anrStart=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $anrBody -SkipHttpErrorCheck -TimeoutSec 15;$anrState=$anrStart.Content|ConvertFrom-Json
            if($anrStart.StatusCode-ne 202-or-not$anrState.running){throw 'ANR diagnostics Runner did not start'}
            Invoke-Adb $deviceSerial @('shell','am','start','-W','-n',"$fixturePackage/.FaultScenarioActivity",'--es','mode','anr')|Out-Null
            Start-Sleep -Seconds 1;Invoke-Adb $deviceSerial @('shell','input','tap','180','480')|Out-Null
            # Android's input-dispatch ANR threshold is normally five seconds. Keep the
            # fixture blocked long enough for the system report, then recover explicitly.
            Start-Sleep -Seconds 8;Invoke-Adb $deviceSerial @('shell','am','force-stop',$fixturePackage)|Out-Null
            $anrState=Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10
            if($anrState.running){Stop-XTestOwnedExecution $base '/v1/monkey/runs/current' $anrState 10|Out-Null}
            $anrDeadline=[DateTime]::UtcNow.AddSeconds(12);do{$anrState=Invoke-RestMethod "$base/v1/monkey/runs/current" -TimeoutSec 10;if(-not$anrState.running-and-not$anrState.finalizing){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$anrDeadline)
            if($anrState.running-or$anrState.finalizing-or$anrState.anrCount-lt 1-or$anrState.failureType-ne'app_anr'){throw "ANR was not classified: running=$($anrState.running), finalizing=$($anrState.finalizing), anrs=$($anrState.anrCount), failure=$($anrState.failureType)"}
            if(-not(Test-RemoteFile $deviceSerial "$($anrState.artifactDir)/anr.json")-or-not(Test-RemoteFile $deviceSerial "$($anrState.artifactDir)/failure.png")){throw 'ANR evidence files are incomplete'};Add-Step $steps 'anr-diagnostics' $true "anrs=$($anrState.anrCount), failureType=$($anrState.failureType)"
            Invoke-Adb $deviceSerial @('shell','am','start','-W','-n',"$fixturePackage/.ValidationActivity")|Out-Null
            $recoveryDeadline=[DateTime]::UtcNow.AddSeconds(8);do{$recoveredForeground=Get-Foreground $port;if($recoveredForeground-eq$fixturePackage){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$recoveryDeadline)
            if($recoveredForeground-ne$fixturePackage){throw "Fixture did not recover after ANR: foreground=$recoveredForeground"};Add-Step $steps 'fault-recovery' $true 'fixture relaunched after forced ANR recovery'
        }else{Add-Step $steps 'extended-fixture' $true "versionCode=$($result.fixtureVersionCode); controlled ANR scenario skipped"}
        $shutdownId="m59-stop-$deviceSerial";$shutdownBody=@{requestId=$shutdownId;package=$fixturePackage;durationSeconds=30;throttleMillis=250;seed=20260911}|ConvertTo-Json
        $shutdownStart=Invoke-WebRequest "$base/v1/monkey/runs/current" -Method Post -ContentType 'application/json' -Body $shutdownBody -SkipHttpErrorCheck -TimeoutSec 15;$shutdownState=$shutdownStart.Content|ConvertFrom-Json
        if($shutdownStart.StatusCode-ne 202-or-not$shutdownState.running-or-not$shutdownState.artifactDir){throw 'Shutdown finalization Runner did not start'};Start-Sleep -Seconds 3
        $shutdownResponse=Invoke-WebRequest "$base/stop" -Method Post -Headers $XTestControlHeaders -SkipHttpErrorCheck -TimeoutSec 20
        if($shutdownResponse.StatusCode-ne 200-or$shutdownResponse.Content-notmatch'Finished!'){throw 'Global stop failed'}
        $exitDeadline=[DateTime]::UtcNow.AddSeconds(8);do{if(-not(Get-AgentPid $deviceSerial)){break};Start-Sleep -Milliseconds 200}while([DateTime]::UtcNow-lt$exitDeadline)
        if(Get-AgentPid $deviceSerial){throw 'Agent remained alive after global stop'}
        $requiredArtifacts=@('events.jsonl','run.json','activity_coverage.json','activity_coverage.txt','exploration_graph.json','start.png','finish.png','logcat.txt','crash.json','anr.json','native_crash.txt','diagnostics.txt','exit_info.json');$missingArtifacts=@($requiredArtifacts|Where-Object{-not(Test-RemoteFile $deviceSerial "$($shutdownState.artifactDir)/$_")})
        if($missingArtifacts.Count-gt 0){throw "Global stop truncated Runner artifacts: $($missingArtifacts-join', ')"};$finalManifest=((Invoke-Adb $deviceSerial @('shell','cat',"$($shutdownState.artifactDir)/run.json"))|Out-String)|ConvertFrom-Json
        if($finalManifest.state.finalizing){throw 'Final run.json retained finalizing=true'};if(-not$finalManifest.state.diagnosticsAvailable){throw "Logcat diagnostics were unavailable: $($finalManifest.state.diagnosticErrors-join', ')"};Add-Step $steps 'global-stop-artifact-finalization' $true ($requiredArtifacts-join', ');$result.passed=$true
    }catch{Add-Step $steps 'harness' $false $_.Exception.Message}
    finally{if($port-gt 0){try{Stop-XTestOwnedExecution "http://127.0.0.1:$port" '/v1/monkey/runs/current' $null 8|Out-Null}catch{}};if($fixtureInstalled){& adb -s $deviceSerial uninstall $fixturePackage 2>$null|Out-Null};if($originalForeground){try{Restore-Foreground $deviceSerial $originalForeground}catch{}};if($owned){Stop-Agent $deviceSerial;Remove-OwnedValidationRuntime $deviceSerial};& adb -s $deviceSerial shell rm -f $remoteAgent /data/local/tmp/xtest-nova-agent.log $remoteLog $remoteStop 2>$null|Out-Null;if($port-gt 0){& adb -s $deviceSerial forward --remove "tcp:$port" 2>$null|Out-Null}}
    $results+=[pscustomobject]$result
}
$report=[ordered]@{schemaVersion='xtest-runner-validation/v1';generatedAt=[DateTime]::UtcNow.ToString('o');passed=(($results|Where-Object{-not$_.passed}).Count-eq 0);devices=$results};$directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null};$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8;$report|ConvertTo-Json -Depth 8;if(-not$report.passed){exit 1}
