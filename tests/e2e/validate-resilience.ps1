param(
    [string[]]$Serial,
    [ValidateRange(30,43200)][int]$DurationSeconds=300,
    [ValidateRange(1,300)][int]$SampleIntervalSeconds=5,
    [switch]$ExerciseOverlayPermission,
    [switch]$SimulateLowBattery,
    [ValidateRange(1,15)][int]$LowBatteryLevel=5,
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\resilience-latest.json')
)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')
$companionPackage='com.openatx.xtest.popup'

function Invoke-Adb([string]$DeviceSerial,[string[]]$Arguments) {
    $output=& adb -s $DeviceSerial @Arguments
    if($LASTEXITCODE -ne 0){throw "adb failed for ${DeviceSerial}: $Arguments"}
    return $output
}
function Get-AgentPid([string]$DeviceSerial) {
    $value=(& adb -s $DeviceSerial shell pidof xtest-nova-agent 2>$null|Out-String).Trim()
    if($LASTEXITCODE -ne 0){return ''}
    return $value
}
function Start-Agent([string]$DeviceSerial) {
    Invoke-Adb $DeviceSerial @('shell','sh','-c',"'nohup /data/local/tmp/xtest-nova-agent server --no-popup >/data/local/tmp/xtest-nova-agent.log 2>&1 &'")|Out-Null
}
function Stop-Agent([string]$DeviceSerial) {
    foreach($value in ((Get-AgentPid $DeviceSerial) -split '\s+')){
        if($value -match '^\d+$'){& adb -s $DeviceSerial shell kill $value 2>$null|Out-Null}
    }
}
function Add-Forward([string]$DeviceSerial) {
    $port=(Invoke-Adb $DeviceSerial @('forward','tcp:0','tcp:7912')|Out-String).Trim()
    if($port -notmatch '^\d+$'){throw "adb did not allocate a local port for $DeviceSerial"}
    return [int]$port
}
function Remove-Forward([string]$DeviceSerial,[int]$Port) {
    if($Port -gt 0){& adb -s $DeviceSerial forward --remove "tcp:$Port" 2>$null|Out-Null}
}
function Wait-Health([int]$Port) {
    $deadline=[DateTime]::UtcNow.AddSeconds(12)
    do {
        try{return Invoke-RestMethod "http://127.0.0.1:$Port/v1/health" -TimeoutSec 3}
        catch{Start-Sleep -Milliseconds 250}
    } while([DateTime]::UtcNow -lt $deadline)
    throw "Agent health timeout on local port $Port"
}
function Get-Battery([string]$DeviceSerial) {
    $text=(Invoke-Adb $DeviceSerial @('shell','dumpsys','battery')|Out-String)
    $level=[regex]::Match($text,'(?m)^\s*level:\s*(\d+)').Groups[1].Value
    $temperature=[regex]::Match($text,'(?m)^\s*temperature:\s*(\d+)').Groups[1].Value
    return [pscustomobject]@{level=if($level){[int]$level}else{-1};temperatureDeciC=if($temperature){[int]$temperature}else{-1};raw=$text}
}
function Get-OverlayMode([string]$DeviceSerial) {
    $path=(& adb -s $DeviceSerial shell pm path $companionPackage 2>$null|Out-String).Trim()
    if($LASTEXITCODE -ne 0 -or -not $path){return $null}
    $text=(Invoke-Adb $DeviceSerial @('shell','cmd','appops','get',$companionPackage,'SYSTEM_ALERT_WINDOW')|Out-String)
    $match=[regex]::Match($text,'(?i)SYSTEM_ALERT_WINDOW:\s*(allow|ignore|deny|default)')
    if(-not $match.Success){throw "Unable to parse overlay permission for $DeviceSerial"}
    return $match.Groups[1].Value.ToLowerInvariant()
}

if(-not $Serial -or $Serial.Count -eq 0){
    $Serial=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_ -match '^([^\s]+)\s+device$'){$matches[1]}})
}
if($Serial.Count -eq 0){throw 'No online Android devices found'}
foreach($value in $Serial){if($value -notmatch '^[A-Za-z0-9._:-]+$'){throw "Invalid device serial: $value"}}

$results=@()
foreach($deviceSerial in $Serial){
    $port=0;$owned=$false;$batterySimulated=$false;$originalOverlayMode=$null
    $result=[ordered]@{serial=$deviceSerial;passed=$false;error='';manufacturer='';model='';sdk=0;abi='';version='';durationSeconds=$DurationSeconds;samples=0;requestAttempts=0;requestFailures=0;pidStable=$false;idleSessions=$true;initialGoroutines=0;maxGoroutines=0;heapGrowthBytes=0;systemGrowthBytes=0;initialBatteryLevel=-1;finalBatteryLevel=-1;maximumTemperatureDeciC=-1;panicCount=0;overlayPermission=[ordered]@{requested=[bool]$ExerciseOverlayPermission;supported=$false;originalMode='';temporaryMode='';restored=$false};lowBattery=[ordered]@{requested=[bool]$SimulateLowBattery;level=$LowBatteryLevel;observed=$false;restored=$false}}
    try {
        $result.manufacturer=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.manufacturer')|Out-String).Trim()
        $result.model=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.model')|Out-String).Trim()
        $result.sdk=[int]((Invoke-Adb $deviceSerial @('shell','getprop','ro.build.version.sdk')|Out-String).Trim())
        $result.abi=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
        if(Get-AgentPid $deviceSerial){throw 'An existing xtest-nova-agent process is active; refusing to replace it'}
        Assert-ValidationRemotePathsAbsent $deviceSerial @('/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.log')
        Assert-ValidationRuntimeAbsent $deviceSerial
$artifact=if($result.abi -match 'arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}else{"$repoRoot\dist\xtest-nova-agent-armv7"}
        if(-not(Test-Path -LiteralPath $artifact)){throw "Missing artifact: $artifact"}
        Invoke-Adb $deviceSerial @('push',$artifact,'/data/local/tmp/xtest-nova-agent')|Out-Null
        Invoke-Adb $deviceSerial @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null
        Start-Agent $deviceSerial;$owned=$true;$port=Add-Forward $deviceSerial
        $health=Wait-Health $port;$result.version=$health.version
        $initial=Invoke-RestMethod "http://127.0.0.1:$port/v1/diagnostics/runtime" -TimeoutSec 3
        $expectedPid=Get-AgentPid $deviceSerial
        $initialHeap=[int64]$initial.heapAllocBytes;$initialSystem=[int64]$initial.systemBytes
        $maxHeap=$initialHeap;$maxSystem=$initialSystem;$result.initialGoroutines=[int]$initial.goroutines;$result.maxGoroutines=[int]$initial.goroutines
        $battery=Get-Battery $deviceSerial;$result.initialBatteryLevel=$battery.level;$result.maximumTemperatureDeciC=$battery.temperatureDeciC
        $deadline=[DateTime]::UtcNow.AddSeconds($DurationSeconds)
        do {
            foreach($path in @('/v1/health','/v1/diagnostics/runtime','/v1/capabilities','/v1/device')){
                $result.requestAttempts++
                try {
                    $response=Invoke-RestMethod "http://127.0.0.1:$port$path" -TimeoutSec 4
                    if($path -eq '/v1/diagnostics/runtime'){
                        $result.samples++
                        $maxHeap=[Math]::Max($maxHeap,[int64]$response.heapAllocBytes);$maxSystem=[Math]::Max($maxSystem,[int64]$response.systemBytes);$result.maxGoroutines=[Math]::Max($result.maxGoroutines,[int]$response.goroutines)
                        foreach($property in $response.sessions.PSObject.Properties){if($property.Value -eq $true){$result.idleSessions=$false}}
                    }
                } catch {$result.requestFailures++}
            }
            $battery=Get-Battery $deviceSerial
            $result.finalBatteryLevel=$battery.level
            $result.maximumTemperatureDeciC=[Math]::Max($result.maximumTemperatureDeciC,$battery.temperatureDeciC)
            if((Get-AgentPid $deviceSerial) -ne $expectedPid){$result.requestFailures++}
            if([DateTime]::UtcNow -lt $deadline){Start-Sleep -Seconds $SampleIntervalSeconds}
        } while([DateTime]::UtcNow -lt $deadline)
        $result.pidStable=((Get-AgentPid $deviceSerial) -eq $expectedPid)
        $result.heapGrowthBytes=$maxHeap-$initialHeap;$result.systemGrowthBytes=$maxSystem-$initialSystem

        if($ExerciseOverlayPermission){
            $originalOverlayMode=Get-OverlayMode $deviceSerial
            if($null -ne $originalOverlayMode){
                $companionPid=(& adb -s $deviceSerial shell pidof $companionPackage 2>$null|Out-String).Trim()
                if($companionPid){throw 'Companion is running; refusing to mutate its overlay permission'}
                $result.overlayPermission.supported=$true;$result.overlayPermission.originalMode=$originalOverlayMode
                $temporaryMode=if($originalOverlayMode -eq 'allow'){'ignore'}else{'allow'}
                $result.overlayPermission.temporaryMode=$temporaryMode
                Invoke-Adb $deviceSerial @('shell','cmd','appops','set',$companionPackage,'SYSTEM_ALERT_WINDOW',$temporaryMode)|Out-Null
                $null=Wait-Health $port
                Invoke-Adb $deviceSerial @('shell','cmd','appops','set',$companionPackage,'SYSTEM_ALERT_WINDOW',$originalOverlayMode)|Out-Null
                $result.overlayPermission.restored=((Get-OverlayMode $deviceSerial) -eq $originalOverlayMode)
                $originalOverlayMode=$null
            }
        }
        if($SimulateLowBattery){
            Invoke-Adb $deviceSerial @('shell','dumpsys','battery','set','level',[string]$LowBatteryLevel)|Out-Null
            $batterySimulated=$true
            $simulated=Get-Battery $deviceSerial
            $result.lowBattery.observed=($simulated.level -eq $LowBatteryLevel)
            $null=Wait-Health $port
            Invoke-Adb $deviceSerial @('shell','dumpsys','battery','reset')|Out-Null
            $batterySimulated=$false
            $restoredBattery=Get-Battery $deviceSerial
            $result.lowBattery.restored=([Math]::Abs($restoredBattery.level-$result.initialBatteryLevel) -le 1)
        }
        $log=(Invoke-Adb $deviceSerial @('shell','cat','/data/local/tmp/xtest-nova-agent.log')|Out-String)
        $result.panicCount=([regex]::Matches($log,'(?im)^panic:')).Count
        $overlayPassed=(-not $ExerciseOverlayPermission -or -not $result.overlayPermission.supported -or $result.overlayPermission.restored)
        $batteryPassed=(-not $SimulateLowBattery -or ($result.lowBattery.observed -and $result.lowBattery.restored))
        $result.passed=($result.requestFailures -eq 0 -and $result.pidStable -and $result.idleSessions -and $result.panicCount -eq 0 -and $result.maxGoroutines -le ($result.initialGoroutines+20) -and $result.heapGrowthBytes -lt 67108864 -and $result.systemGrowthBytes -lt 67108864 -and $overlayPassed -and $batteryPassed)
    } catch {$result.error=$_.Exception.Message}
    finally {
        if($null -ne $originalOverlayMode){& adb -s $deviceSerial shell cmd appops set $companionPackage SYSTEM_ALERT_WINDOW $originalOverlayMode 2>$null|Out-Null}
        if($batterySimulated){& adb -s $deviceSerial shell dumpsys battery reset 2>$null|Out-Null}
        if($owned){Stop-Agent $deviceSerial;Remove-OwnedValidationRuntime $deviceSerial;& adb -s $deviceSerial shell rm -f /data/local/tmp/xtest-nova-agent /data/local/tmp/xtest-nova-agent.log 2>$null|Out-Null}
        Remove-Forward $deviceSerial $port
    }
    $results+=[pscustomobject]$result
}
$versions=@($results|Where-Object passed|Select-Object -ExpandProperty version -Unique)
$report=[ordered]@{schemaVersion='xtest-resilience/v1';generatedAt=[DateTime]::UtcNow.ToString('o');passed=(($results|Where-Object{-not $_.passed}).Count -eq 0 -and $versions.Count -eq 1);devices=$results}
$directory=Split-Path -Parent $OutputPath
if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null}
$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8
$report|ConvertTo-Json -Depth 8
if(-not $report.passed){exit 1}
