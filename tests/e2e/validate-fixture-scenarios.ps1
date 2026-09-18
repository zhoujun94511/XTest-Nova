param(
    [string]$Serial,
    [Parameter(Mandatory)][string]$FixturePath,
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\fixture-scenarios-latest.json')
)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
$package='com.xtest.nova.fixture';$remoteAgent='/data/local/tmp/xtest-nova-agent';$port=0;$owned=$false;$installed=$false;$recording=$false;$recordingState=$null;$cameraWasGranted=$false
function Invoke-Adb([string[]]$Arguments){$output=& adb -s $Serial @Arguments;if($LASTEXITCODE-ne 0){throw "adb failed: $Arguments"};return $output}
function Agent-Pid(){return (& adb -s $Serial shell pidof xtest-nova-agent 2>$null|Out-String).Trim()}
function Stop-Agent(){foreach($value in((Agent-Pid)-split'\s+')){if($value-match'^\d+$'){& adb -s $Serial shell kill $value 2>$null|Out-Null}}}
function Add-Step($Steps,[string]$Name,[bool]$Passed,[string]$Detail){$Steps.Add([pscustomobject]@{name=$Name;passed=$Passed;detail=$Detail})}
function Wait-Health(){ $deadline=[DateTime]::UtcNow.AddSeconds(12);do{try{return Invoke-RestMethod "http://127.0.0.1:$port/v1/health" -TimeoutSec 3}catch{Start-Sleep -Milliseconds 250}}while([DateTime]::UtcNow-lt$deadline);throw 'Agent health timeout' }
function Start-Activity([string]$Activity,[string[]]$Extras=@()){Invoke-Adb (@('shell','am','start','-W','-n',"$package/$Activity")+$Extras)|Out-Null;Start-Sleep -Milliseconds 800}
function Wait-Hierarchy([string[]]$Markers,[int]$Seconds=12){$deadline=[DateTime]::UtcNow.AddSeconds($Seconds);do{try{$text=(Invoke-WebRequest "http://127.0.0.1:$port/v1/hierarchy/raw" -TimeoutSec 8).Content;$missing=@($Markers|Where-Object{-not$text.Contains($_)});if($missing.Count-eq 0){return $text}}catch{};Start-Sleep -Milliseconds 300}while([DateTime]::UtcNow-lt$deadline);throw "Hierarchy markers missing: $($Markers-join', ')"}
if(-not$Serial){$Serial=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_-match'^([^\s]+)\s+device$'){$matches[1]}}|Select-Object -First 1)}
if(-not$Serial){throw 'No online Android device found'}
if($Serial-notmatch'^[A-Za-z0-9._:-]+$'){throw 'Invalid serial'}
if(-not(Test-Path -LiteralPath $FixturePath)){throw "Fixture APK not found: $FixturePath"}
$catalog=(Get-Content -LiteralPath "$repoRoot\tests\scenarios\validation-fixture-matrix.json" -Raw)|ConvertFrom-Json
if($catalog.package-ne$package-or$catalog.scenarios.Count-lt 9){throw 'Fixture scenario catalog is incomplete'}
$steps=[System.Collections.Generic.List[object]]::new();$result=[ordered]@{serial=$Serial;fixture=$FixturePath;version='';passed=$false;steps=$steps}
try{
    if(Agent-Pid){throw 'Existing Agent found; refusing to replace it'}
    Assert-ValidationRemotePathsAbsent $Serial @($remoteAgent,'/data/local/tmp/xtest-nova-agent.log')
    if((& adb -s $Serial shell pm path $package 2>$null|Out-String).Trim()){throw 'Validation fixture already installed'}
    Invoke-Adb @('install','-r',$FixturePath)|Out-Null;$installed=$true
    # `cmd package check-permission` is not available on every Android build.
    # Parse dumpsys instead so the harness also works on vendor Android 13 images.
    $packageDump=(Invoke-Adb @('shell','dumpsys','package',$package)|Out-String)
    $cameraWasGranted=$packageDump -match 'android\.permission\.CAMERA:\s+granted=true'
    $abi=(Invoke-Adb @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim();$agentPath=if($abi-match'^arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}elseif($abi-eq'armeabi-v7a'){"$repoRoot\dist\xtest-nova-agent-armv7"}else{throw "Unsupported ABI: $abi"}
    Invoke-Adb @('push',$agentPath,$remoteAgent)|Out-Null;Invoke-Adb @('shell','chmod','755',$remoteAgent)|Out-Null;Invoke-Adb @('shell','sh','-c',"'nohup $remoteAgent server --no-popup >/data/local/tmp/xtest-nova-agent.log 2>&1 &'")|Out-Null;$owned=$true
    $port=[int]((Invoke-Adb @('forward','tcp:0','tcp:7912')|Out-String).Trim());$health=Wait-Health;$result.version=$health.version;$base="http://127.0.0.1:$port"
    $session=Invoke-RestMethod "$base/session/$package" -Method Post -TimeoutSec 12;if(-not$session.success){throw 'Package session launch failed'}
    # The legacy focused-text field intentionally opens the IME and scrolls to the
    # bottom. Hide it and return the ScrollView to the top before checking nav nodes.
    Wait-Hierarchy @('最终文本-杭州')|Out-Null
    Invoke-Adb @('shell','input','keyevent','4')|Out-Null
    Invoke-Adb @('shell','input','swipe','500','500','500','1800','250')|Out-Null
    Start-Sleep -Milliseconds 500
    Wait-Hierarchy @('复杂表单','弹窗与异步 UI','运行时权限')|Out-Null;Add-Step $steps 'F1-navigation' $true 'session launch and main hierarchy markers'

    Start-Activity '.FormScenarioActivity'
    Wait-Hierarchy @('form-plain')|Out-Null
    Invoke-Adb @('shell','input','keyevent','4')|Out-Null
    $form=Wait-Hierarchy @('form-plain','form-password')
    if($form-notmatch'password="true"'){
        $passwordNode=[regex]::Match($form,'<node[^>]*content-desc="form-password"[^>]*/?>').Value
        throw "Password field was not marked by hierarchy: $passwordNode"
    }
    $recordBody=@{requestId="fixture-form-$([DateTime]::UtcNow.Ticks)";package=$package;task='fixture-matrix';name='focused-plain'}|ConvertTo-Json
    $recordStart=Invoke-WebRequest "$base/v1/recordings" -Method Post -ContentType 'application/json' -Body $recordBody -SkipHttpErrorCheck -TimeoutSec 12;if($recordStart.StatusCode-ne 202){throw "Recording start failed: $($recordStart.Content)"};$recordingState=$recordStart.Content|ConvertFrom-Json;$recording=$true
    $focused=Invoke-WebRequest "$base/v1/recordings/current/focused-text" -Method Post -Headers (Get-XTestOwnedHeaders $recordingState) -SkipHttpErrorCheck -TimeoutSec 12;if($focused.StatusCode-ne 200-or$focused.Content-notmatch'杭州 Nova'){throw "Focused plain text was not recorded: $($focused.Content)"}
    Stop-XTestOwnedExecution $base '/v1/recordings/current' $recordingState 12|Out-Null;$recording=$false;$recordingState=$null
    $sawDuplicate=$false;$sawDisabled=$false
    foreach($attempt in 1..6){
        $visible=(Invoke-WebRequest "$base/v1/hierarchy/raw" -TimeoutSec 8).Content
        $sawDuplicate=$sawDuplicate-or$visible.Contains('重复操作')
        $sawDisabled=$sawDisabled-or$visible.Contains('禁用操作')
        if($sawDuplicate-and$sawDisabled){break}
        Invoke-Adb @('shell','input','swipe','500','1700','500','350','350')|Out-Null
        Start-Sleep -Milliseconds 350
    }
    if(-not($sawDuplicate-and$sawDisabled)){throw "Scrollable form nodes not observed: duplicate=$sawDuplicate disabled=$sawDisabled"}
    Start-Activity '.FormScenarioActivity' @('--es','focus','password')
    $recordBody=@{requestId="fixture-password-$([DateTime]::UtcNow.Ticks)";package=$package;task='fixture-matrix';name='focused-password'}|ConvertTo-Json
    $recordStart=Invoke-WebRequest "$base/v1/recordings" -Method Post -ContentType 'application/json' -Body $recordBody -SkipHttpErrorCheck -TimeoutSec 12;if($recordStart.StatusCode-ne 202){throw 'Password recording start failed'};$recordingState=$recordStart.Content|ConvertFrom-Json;$recording=$true
    $password=Invoke-WebRequest "$base/v1/recordings/current/focused-text" -Method Post -Headers (Get-XTestOwnedHeaders $recordingState) -SkipHttpErrorCheck -TimeoutSec 12
    if($password.StatusCode-ne 409-or$password.Content-notmatch'password'){throw "Password field was not rejected: $($password.StatusCode) $($password.Content)"}
    Stop-XTestOwnedExecution $base '/v1/recordings/current' $recordingState 12|Out-Null;$recording=$false;$recordingState=$null;Add-Step $steps 'F2-form-recording' $true 'plain focused text recorded; password focused text rejected'

    Start-Activity '.DialogScenarioActivity' @('--ez','auto','true');Wait-Hierarchy @('确认弹窗场景','确认执行','取消执行')|Out-Null;Add-Step $steps 'F3-dialog' $true 'delayed non-cancelable dialog observed';Invoke-Adb @('shell','am','force-stop',$package)|Out-Null

    Invoke-Adb @('shell','pm','revoke',$package,'android.permission.CAMERA')|Out-Null
    Start-Activity '.PermissionScenarioActivity' @('--ez','auto','true');$permissionHierarchy=Wait-Hierarchy @('permissioncontroller') 12
    $foreground=(Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 8).package;if($foreground-notmatch'permissioncontroller|packageinstaller'){throw "Permission controller was not foreground: $foreground"};Invoke-Adb @('shell','input','keyevent','4')|Out-Null
    $permissionDeadline=[DateTime]::UtcNow.AddSeconds(8);do{$permissionReturn=(Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 8).package;if($permissionReturn-eq$package){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$permissionDeadline)
    if($permissionReturn-ne$package){throw "Permission cancellation did not return to fixture: $permissionReturn"};Add-Step $steps 'F4-permission' $true "controller=$foreground; returned=$permissionReturn"

    Start-Activity '.WebViewScenarioActivity';$webDeadline=[DateTime]::UtcNow.AddSeconds(10);do{$webviews=@(Invoke-RestMethod "$base/webviews/$package" -TimeoutSec 8);if($webviews.Count-gt 0){break};Start-Sleep -Milliseconds 400}while([DateTime]::UtcNow-lt$webDeadline)
    if($webviews.Count-eq 0){throw 'Debug WebView socket was not discovered'};Add-Step $steps 'F5-webview' $true "pid=$($webviews[0].pid), socket=$($webviews[0].socketPath)"

    Invoke-Adb @('shell','am','force-stop',$package)|Out-Null;Start-Activity '.LifecycleScenarioActivity';$first=Wait-Hierarchy @('lifecycle-status');Invoke-Adb @('shell','am','force-stop',$package)|Out-Null;Start-Activity '.LifecycleScenarioActivity';$second=Wait-Hierarchy @('lifecycle-status')
    if($first-notmatch'launches=1'-or$second-notmatch'launches=2'){throw 'Lifecycle persistent launch counter did not advance'}
    Invoke-Adb @('shell','am','start','-W','-a','android.settings.APPLICATION_DETAILS_SETTINGS','-d',"package:$package")|Out-Null
    $settingsPackage=(Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 8).package;if($settingsPackage-eq$package){throw 'Application settings did not leave the fixture'}
    Invoke-Adb @('shell','input','keyevent','4')|Out-Null;$returnDeadline=[DateTime]::UtcNow.AddSeconds(8);do{$returnPackage=(Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 8).package;if($returnPackage-eq$package){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$returnDeadline)
    if($returnPackage-ne$package){throw "Settings back navigation did not recover fixture: $returnPackage"};Add-Step $steps 'F6-lifecycle' $true "launches 1->2; settings=$settingsPackage; returned=$returnPackage"
    $result.passed=$true
}catch{Add-Step $steps 'harness' $false $_.Exception.Message}
finally{
    if($recording-and$port-gt 0){try{Stop-XTestOwnedExecution "http://127.0.0.1:$port" '/v1/recordings/current' $recordingState 8|Out-Null}catch{}}
    if($installed){if($cameraWasGranted){& adb -s $Serial shell pm grant $package android.permission.CAMERA 2>$null|Out-Null}else{& adb -s $Serial shell pm revoke $package android.permission.CAMERA 2>$null|Out-Null}; & adb -s $Serial shell am force-stop $package 2>$null|Out-Null; & adb -s $Serial uninstall $package 2>$null|Out-Null}
    if($port-gt 0){try{Invoke-RestMethod "http://127.0.0.1:$port/stop" -Method Post -Headers $XTestControlHeaders -TimeoutSec 15|Out-Null}catch{};& adb -s $Serial forward --remove "tcp:$port" 2>$null|Out-Null}
    if($owned){Stop-Agent};& adb -s $Serial shell rm -f $remoteAgent /data/local/tmp/xtest-nova-agent.log 2>$null|Out-Null
}
$report=[ordered]@{schemaVersion='xtest-fixture-scenarios/v1';generatedAt=[DateTime]::UtcNow.ToString('o');passed=$result.passed;device=[pscustomobject]$result};$directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null};$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8;$report|ConvertTo-Json -Depth 8;if(-not$report.passed){exit 1}
