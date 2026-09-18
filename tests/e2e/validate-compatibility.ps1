param([string[]]$Serial,[string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\compatibility-latest.json'))
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
function Invoke-Adb([string]$DeviceSerial,[string[]]$Arguments){$output=& adb -s $DeviceSerial @Arguments;if($LASTEXITCODE -ne 0){throw "adb failed for ${DeviceSerial}: $Arguments"};return $output}
function Get-AgentPid([string]$DeviceSerial){$value=(& adb -s $DeviceSerial shell pidof xtest-nova-agent 2>$null|Out-String).Trim();if($LASTEXITCODE -ne 0){return ''};return $value}
function Stop-Agent([string]$DeviceSerial){foreach($value in ((Get-AgentPid $DeviceSerial)-split '\s+')){if($value -match '^\d+$'){& adb -s $DeviceSerial shell kill $value 2>$null|Out-Null}}}
function Start-Agent([string]$DeviceSerial){Invoke-Adb $DeviceSerial @('shell','sh','-c',"'nohup /data/local/tmp/xtest-nova-agent server --no-popup >/data/local/tmp/xtest-nova-agent.log 2>&1 &'")|Out-Null}
function Add-Forward([string]$DeviceSerial){$port=(Invoke-Adb $DeviceSerial @('forward','tcp:0','tcp:7912')|Out-String).Trim();if($port -notmatch '^\d+$'){throw 'adb did not allocate a local port'};return [int]$port}
function Remove-Forward([string]$DeviceSerial,[int]$Port){if($Port -gt 0){& adb -s $DeviceSerial forward --remove "tcp:$Port" 2>$null|Out-Null}}
function Wait-Health([int]$Port){$deadline=[DateTime]::UtcNow.AddSeconds(12);do{try{return Invoke-RestMethod "http://127.0.0.1:$Port/v1/health" -TimeoutSec 3}catch{Start-Sleep -Milliseconds 250}}while([DateTime]::UtcNow -lt $deadline);throw 'Agent health timeout'}
function Add-Check([System.Collections.Generic.List[object]]$Checks,[string]$Name,[bool]$Passed,[string]$Detail){$Checks.Add([pscustomobject]@{name=$Name;passed=$Passed;detail=$Detail})}
function Invoke-SafeCheck([System.Collections.Generic.List[object]]$Checks,[string]$Name,[scriptblock]$Action){try{$detail=& $Action;Add-Check $Checks $Name $true ([string]$detail)}catch{Add-Check $Checks $Name $false $_.Exception.Message}}
if(-not $Serial -or $Serial.Count -eq 0){$Serial=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_ -match '^([^\s]+)\s+device$'){$matches[1]}})}
if($Serial.Count -eq 0){throw 'No online Android devices found'}
foreach($value in $Serial){if($value -notmatch '^[A-Za-z0-9._:-]+$'){throw "Invalid device serial: $value"}}
$results=@()
foreach($deviceSerial in $Serial){
    $port=0;$owned=$false;$checks=[System.Collections.Generic.List[object]]::new()
    $result=[ordered]@{serial=$deviceSerial;manufacturer='';model='';sdk=0;abi='';version='';passed=$false;checks=$checks}
    try{
        $result.manufacturer=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.manufacturer')|Out-String).Trim();$result.model=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.model')|Out-String).Trim();$result.sdk=[int]((Invoke-Adb $deviceSerial @('shell','getprop','ro.build.version.sdk')|Out-String).Trim());$result.abi=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
        if(Get-AgentPid $deviceSerial){throw 'Existing Agent found; refusing to replace it'}
        Assert-ValidationRemotePathsAbsent $deviceSerial @('/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.log')
        Assert-ValidationRuntimeAbsent $deviceSerial
$artifact=if($result.abi -match '^arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}elseif($result.abi -eq 'armeabi-v7a'){"$repoRoot\dist\xtest-nova-agent-armv7"}else{throw "Unsupported ABI: $($result.abi)"}
        if(-not(Test-Path -LiteralPath $artifact)){throw "Missing artifact: $artifact"}
        Invoke-Adb $deviceSerial @('push',$artifact,'/data/local/tmp/xtest-nova-agent')|Out-Null;Invoke-Adb $deviceSerial @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null;Start-Agent $deviceSerial;$owned=$true;$port=Add-Forward $deviceSerial
        $health=Wait-Health $port;$result.version=$health.version;$base="http://127.0.0.1:$port"
        Invoke-SafeCheck $checks 'device-info' {$value=Invoke-RestMethod "$base/v1/device" -TimeoutSec 10;if(-not $value.platform -or -not $value.sdk -or -not $value.abi){throw 'missing platform, sdk, or abi'};"$($value.platform) SDK $($value.sdk) $($value.abi)"}
        Invoke-SafeCheck $checks 'foreground-package' {$value=Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 10;if($value.package -notmatch '^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$'){throw "invalid package: $($value.package)"};$value.package}
        Invoke-SafeCheck $checks 'package-list' {$value=Invoke-RestMethod "$base/packages?system=true" -TimeoutSec 20;if($value.Count -lt 10){throw "unexpected package count: $($value.Count)"};"$($value.Count) packages"}
        Invoke-SafeCheck $checks 'package-info' {$value=Invoke-RestMethod "$base/packages/com.android.settings/info" -TimeoutSec 15;if(-not $value.success -or $value.data.packageName -ne 'com.android.settings' -or -not $value.data.apkPath){throw 'settings package metadata incomplete'};"versionCode=$($value.data.versionCode)"}
        Invoke-SafeCheck $checks 'process-list' {$value=Invoke-RestMethod "$base/proc/list" -TimeoutSec 20;if($value.Count -lt 20){throw "unexpected process count: $($value.Count)"};"$($value.Count) processes"}
        Invoke-SafeCheck $checks 'process-pid' {$value=(Invoke-WebRequest "$base/pidof/com.android.systemui" -TimeoutSec 10).Content.Trim();if($value -notmatch '^\d+$'){throw "invalid pid: $value"};$value}
        Invoke-SafeCheck $checks 'process-memory' {$value=Invoke-RestMethod "$base/proc/com.android.systemui/meminfo" -TimeoutSec 20;if($value.'total pss' -le 0){throw 'total pss missing'};"totalPss=$($value.'total pss')"}
        Invoke-SafeCheck $checks 'process-cpu' {$value=Invoke-RestMethod "$base/proc/com.android.systemui/cpuinfo" -TimeoutSec 20;if($value.pid -le 0 -or $value.coreCount -le 0){throw 'cpu response incomplete'};"pid=$($value.pid), cores=$($value.coreCount)"}
        Invoke-SafeCheck $checks 'process-performance' {$value=Invoke-RestMethod "$base/proc/com.android.systemui/perf" -TimeoutSec 25;if($value.cpuinfo.pid -le 0 -or $value.memoinfo.'total pss' -le 0){throw 'performance response incomplete'};'cpu, memory, and network present'}
        Invoke-SafeCheck $checks 'device-memory' {$value=Invoke-RestMethod "$base/device/memory" -TimeoutSec 10;if($value.total -le 0 -or $value.available -lt 0){throw 'memory response incomplete'};"total=$($value.total)"}
        Invoke-SafeCheck $checks 'network-info' {$value=Invoke-RestMethod "$base/network/info" -TimeoutSec 10;if($null -eq $value.connected){throw 'connected flag missing'};"connected=$($value.connected)"}
        Invoke-SafeCheck $checks 'disk-info' {$value=Invoke-RestMethod "$base/disk/info" -TimeoutSec 10;if($value.total -le 0 -or $value.available -lt 0){throw 'storage response incomplete'};"total=$($value.total)"}
        Invoke-SafeCheck $checks 'ime-status' {$value=Invoke-RestMethod "$base/imeStatus" -TimeoutSec 10;if(-not $value.currentIme -or @($value.enabledImes).Count -eq 0){throw 'IME response incomplete'};$value.currentIme}
        Invoke-SafeCheck $checks 'webviews' {$value=@(Invoke-RestMethod "$base/webviews" -TimeoutSec 10);"$($value.Count) sockets"}
        Invoke-SafeCheck $checks 'wlan-ip' {$value=Invoke-RestMethod "$base/wlan/ip" -TimeoutSec 10;if($value.ip -notmatch '^\d{1,3}(\.\d{1,3}){3}$'){throw "invalid IPv4: $($value.ip)"};$value.ip}
        Invoke-SafeCheck $checks 'screenshot' {$client=[Net.Http.HttpClient]::new();try{$client.Timeout=[TimeSpan]::FromSeconds(20);$bytes=$client.GetByteArrayAsync("$base/screenshot/0").GetAwaiter().GetResult()}finally{$client.Dispose()};if($bytes.Length -lt 1024 -or $bytes[0] -ne 137 -or $bytes[1] -ne 80 -or $bytes[2] -ne 78 -or $bytes[3] -ne 71){throw 'invalid PNG'};"$($bytes.Length) bytes"}
        Invoke-SafeCheck $checks 'window-hierarchy' {$value=Invoke-RestMethod "$base/dump/hierarchy" -TimeoutSec 30;if($value.result -notmatch '<hierarchy'){throw 'hierarchy XML missing'};"$($value.result.Length) characters"}
        Invoke-SafeCheck $checks 'uiautomator-status' {$value=Invoke-RestMethod "$base/services/uiautomator" -TimeoutSec 10;if($null -eq $value.running){throw 'running flag missing'};"running=$($value.running)"}
        $failed=@($checks|Where-Object{-not $_.passed});$result.passed=($failed.Count -eq 0)
    }catch{Add-Check $checks 'harness' $false $_.Exception.Message;$result.passed=$false}
    finally{if($owned){Stop-Agent $deviceSerial;Remove-OwnedValidationRuntime $deviceSerial;& adb -s $deviceSerial shell rm -f /data/local/tmp/xtest-nova-agent /data/local/tmp/xtest-nova-agent.log 2>$null|Out-Null};Remove-Forward $deviceSerial $port}
    $results+=[pscustomobject]$result
}
$versions=@($results|Where-Object passed|Select-Object -ExpandProperty version -Unique)
$report=[ordered]@{schemaVersion='xtest-compatibility/v1';generatedAt=[DateTime]::UtcNow.ToString('o');passed=(($results|Where-Object{-not $_.passed}).Count -eq 0 -and $versions.Count -eq 1);devices=$results}
$directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null};$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8;$report|ConvertTo-Json -Depth 8
if(-not $report.passed){exit 1}
