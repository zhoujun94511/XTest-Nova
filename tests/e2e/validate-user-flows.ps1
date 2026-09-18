param([string[]]$Serial,[string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\user-flows-latest.json'),[switch]$SkipEstablished)
$ErrorActionPreference='Stop';$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'));$popupPackage='com.openatx.xtest.popup'
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
function Invoke-Adb([string]$DeviceSerial,[string[]]$Arguments){$output=& adb -s $DeviceSerial @Arguments;if($LASTEXITCODE-ne 0){throw "adb failed for $DeviceSerial $Arguments"};return $output}
function Resolve-Serials {if($Serial){return @($Serial)};return @(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_-match '^([^\s]+)\s+device$'){$matches[1]}})}
function Get-AgentPid([string]$DeviceSerial){return (& adb -s $DeviceSerial shell pidof xtest-nova-agent 2>$null|Out-String).Trim()}
function Get-Foreground([string]$DeviceSerial){$text=(& adb -s $DeviceSerial shell dumpsys activity activities|Out-String);if($text-match'(?:topResumedActivity|mResumedActivity|ResumedActivity).*? ([A-Za-z0-9._]+)/'){return $matches[1]};return ''}
function Add-Forward([string]$DeviceSerial){$value=(Invoke-Adb $DeviceSerial @('forward','tcp:0','tcp:7912')|Out-String).Trim();if($value-notmatch'^\d+$'){throw 'forward failed'};return [int]$value}
function Test-PopupInstalled([string]$DeviceSerial){& adb -s $DeviceSerial shell pm path $popupPackage 2>$null|Out-Null;return $LASTEXITCODE-eq 0}
function Stop-Agent([string]$DeviceSerial){& adb -s $DeviceSerial shell /data/local/tmp/xtest-nova-agent server -d --stop 2>$null|Out-Null}
function Invoke-TerminalFlow([string]$Base){
  $previousGoCache=$env:GOCACHE;$previousGoProxy=$env:GOPROXY;$previousGoFlags=$env:GOFLAGS
  Push-Location (Join-Path $repoRoot 'agent')
  try{
    $env:GOCACHE=Join-Path $repoRoot '.gocache';$env:GOPROXY='off';$env:GOFLAGS='-mod=vendor'
    $result=& go run ./cmd/terminal-flow-check -base $Base
    if($LASTEXITCODE-ne 0){throw 'Terminal flow failed'}
    return $result
  }finally{
    Pop-Location
    if($null-eq$previousGoCache){Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue}else{$env:GOCACHE=$previousGoCache}
    if($null-eq$previousGoProxy){Remove-Item Env:GOPROXY -ErrorAction SilentlyContinue}else{$env:GOPROXY=$previousGoProxy}
    if($null-eq$previousGoFlags){Remove-Item Env:GOFLAGS -ErrorAction SilentlyContinue}else{$env:GOFLAGS=$previousGoFlags}
  }
}
$devices=Resolve-Serials;if($devices.Count-eq 0){throw 'No online Android devices'}
$runnerReport="$repoRoot\tests\reports\user-flows-runner.json";$m56Report="$repoRoot\tests\reports\user-flows-m56.json"
if(-not$SkipEstablished){
  $runnerAttempt="$runnerReport.attempt";$m56Attempt="$m56Report.attempt"
  & "$PSScriptRoot\validate-runner.ps1" -Serial $devices -OutputPath $runnerAttempt|Out-Null
  if($LASTEXITCODE-ne 0){Move-Item -Force $runnerAttempt "$runnerReport.failed";throw 'Runner user flow failed'}
  Move-Item -Force $runnerAttempt $runnerReport
  & "$PSScriptRoot\validate-m56.ps1" -Serial $devices -OutputPath $m56Attempt|Out-Null
  if($LASTEXITCODE-ne 0){Move-Item -Force $m56Attempt "$m56Report.failed";throw 'Popup/record/replay user flow failed'}
  Move-Item -Force $m56Attempt $m56Report
}elseif(-not(Test-Path -LiteralPath $runnerReport)-or-not(Test-Path -LiteralPath $m56Report)){throw 'Established flow reports are missing'}
$results=[System.Collections.Generic.List[object]]::new()
foreach($deviceSerial in $devices){
  $port=0;$owned=$false;$popupAdded=$false;$originalForeground=Get-Foreground $deviceSerial
  $result=[ordered]@{serial=$deviceSerial;sdk=0;version='';terminal=$null;popup=$null;passed=$false}
  try{
    if(Get-AgentPid $deviceSerial){throw 'xtest-nova-agent already running'}
    Assert-ValidationRemotePathsAbsent $deviceSerial @('/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.pid','/data/local/tmp/xtest-nova-agent.log')
    Assert-ValidationRuntimeAbsent $deviceSerial
    $result.sdk=[int]((Invoke-Adb $deviceSerial @('shell','getprop','ro.build.version.sdk')|Out-String).Trim())
    $abi=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
    $agent=if($abi-match'^arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}elseif($abi-eq'armeabi-v7a'){"$repoRoot\dist\xtest-nova-agent-armv7"}else{throw "Unsupported ABI: $abi"}
    Invoke-Adb $deviceSerial @('push',$agent,'/data/local/tmp/xtest-nova-agent')|Out-Null;Invoke-Adb $deviceSerial @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null
    Invoke-Adb $deviceSerial @('shell','/data/local/tmp/xtest-nova-agent','server','-d','--legacy-unsafe-api')|Out-Null;$owned=$true;Start-Sleep -Milliseconds 800
    $port=Add-Forward $deviceSerial;$base="http://127.0.0.1:$port";$health=$null;$deadline=[DateTime]::UtcNow.AddSeconds(10)
    do{try{$health=Invoke-RestMethod "$base/v1/health" -TimeoutSec 2;if($health.status-eq'ok'){break}}catch{};Start-Sleep -Milliseconds 200}while([DateTime]::UtcNow-lt$deadline)
    if(-not$health-or$health.status-ne'ok'){throw 'Agent health failed'};$result.version=$health.version
    $terminalJSON=Invoke-TerminalFlow $base;$result.terminal=$terminalJSON|ConvertFrom-Json
    if(-not(Test-PopupInstalled $deviceSerial)){throw 'Bundled runtime did not install Companion'}
    $popupAdded=$true;Start-Sleep -Milliseconds 800
    $foreground=Get-Foreground $deviceSerial;if($foreground-eq$popupPackage){throw 'Popup launcher left a visible activity in foreground'}
    $windows=(Invoke-Adb $deviceSerial @('shell','dumpsys','window','windows')|Out-String)
    if($windows-notmatch'package=com\.openatx\.xtest\.popup'-or$windows-notmatch'ty=APPLICATION_OVERLAY'){throw 'Popup overlay window is not visible'}
    $services=(Invoke-Adb $deviceSerial @('shell','dumpsys','activity','services',$popupPackage)|Out-String);if($services-notmatch'OverlayService'){throw 'Popup overlay service did not start'}
    $status=(Invoke-Adb $deviceSerial @('shell','/data/local/tmp/xtest-nova-agent','popup','status')|Out-String).Trim();if($status-notmatch'installed=true'-or$status-notmatch'running=true'-or$status-notmatch'versionCode=30718'){throw 'Popup status mismatch'}
    $result.popup=[ordered]@{mode='server-auto-bootstrap-overlay';passed=$true;status=$status}
    if(-not$result.terminal.passed-or-not$result.popup.passed){throw 'User flow result incomplete'};$result.passed=$true
  }catch{$result.error=$_.Exception.Message}
  finally{
    if($popupAdded){& adb -s $deviceSerial shell am force-stop $popupPackage 2>$null|Out-Null}
    if($originalForeground){& adb -s $deviceSerial shell monkey -p $originalForeground -c android.intent.category.LAUNCHER 1 2>$null|Out-Null}
    if($owned){Stop-Agent $deviceSerial;Remove-OwnedValidationRuntime $deviceSerial};if($port-gt 0){& adb -s $deviceSerial forward --remove "tcp:$port" 2>$null|Out-Null}
    & adb -s $deviceSerial shell rm -f /data/local/tmp/xtest-nova-agent /data/local/tmp/xtest-nova-agent.pid /data/local/tmp/xtest-nova-agent.log 2>$null|Out-Null
  }
  $results.Add([pscustomobject]$result)
}
$runner=Get-Content -LiteralPath $runnerReport -Raw|ConvertFrom-Json;$m56=Get-Content -LiteralPath $m56Report -Raw|ConvertFrom-Json
$report=[ordered]@{schemaVersion='xtest-user-flows/v1';generatedAt=[DateTime]::UtcNow.ToString('o');passed=($runner.passed-and$m56.passed-and(($results|Where-Object{-not$_.passed}).Count-eq 0));runner=$runner.passed;popupRecordReplay=$m56.passed;devices=$results}
$directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force $directory|Out-Null};$report|ConvertTo-Json -Depth 10|Set-Content -LiteralPath $OutputPath -Encoding utf8;$report|ConvertTo-Json -Depth 10
if(-not$report.passed){exit 1}
