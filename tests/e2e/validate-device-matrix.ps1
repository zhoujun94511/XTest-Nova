param(
    [string[]]$Serial,
    [ValidateRange(10,3600)][int]$DurationSeconds=60,
    [ValidateRange(1,60)][int]$SampleIntervalSeconds=2,
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\device-matrix-latest.json')
)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
. (Join-Path $PSScriptRoot 'lib\device-validation-safety.ps1')

function Invoke-Adb([string]$DeviceSerial,[string[]]$Arguments) {
    $output=& adb -s $DeviceSerial @Arguments
    if($LASTEXITCODE -ne 0){throw "adb failed for ${DeviceSerial}: $Arguments"}
    return $output
}
function Get-AgentPid([string]$DeviceSerial) {
    $value=(& adb -s $DeviceSerial shell pidof xtest-nova-agent | Out-String).Trim()
    if($LASTEXITCODE -ne 0){return ''}
    return $value
}
function Start-Agent([string]$DeviceSerial) {
    Invoke-Adb $DeviceSerial @('shell','sh','-c',"'nohup /data/local/tmp/xtest-nova-agent server --no-popup >/data/local/tmp/xtest-nova-agent.log 2>&1 &'")|Out-Null
}
function Add-AgentForward([string]$DeviceSerial) {
    $port=(Invoke-Adb $DeviceSerial @('forward','tcp:0','tcp:7912')|Out-String).Trim()
    if($port -notmatch '^\d+$'){throw "adb did not allocate a local port for $DeviceSerial"}
    return [int]$port
}
function Remove-AgentForward([string]$DeviceSerial,[int]$Port) {
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
function Stop-OwnedAgent([string]$DeviceSerial) {
    $pidText=Get-AgentPid $DeviceSerial
    foreach($value in ($pidText -split '\s+')){
        if($value -match '^\d+$'){& adb -s $DeviceSerial shell kill $value 2>$null|Out-Null}
    }
}

if(-not $Serial -or $Serial.Count -eq 0){
    $Serial=@(& adb devices|Select-Object -Skip 1|ForEach-Object{if($_ -match '^([^\s]+)\s+device$'){$matches[1]}})
}
if($Serial.Count -eq 0){throw 'No online Android devices found'}
foreach($value in $Serial){if($value -notmatch '^[A-Za-z0-9._:-]+$'){throw "Invalid device serial: $value"}}

$results=@()
foreach($deviceSerial in $Serial){
    $localPort=0;$owned=$false
    $result=[ordered]@{serial=$deviceSerial;passed=$false;error='';manufacturer='';model='';sdk=0;abi='';version='';samples=0;healthFailures=0;pidStable=$false;idleSessions=$true;forwardRecovered=$false;processRecovered=$false;initialPid='';restartedPid='';initialStartedAt='';restartedAt='';initialGoroutines=0;maxGoroutines=0;heapGrowthBytes=0;systemGrowthBytes=0;panicCount=0}
    try {
        $result.manufacturer=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.manufacturer')|Out-String).Trim()
        $result.model=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.model')|Out-String).Trim()
        $result.sdk=[int]((Invoke-Adb $deviceSerial @('shell','getprop','ro.build.version.sdk')|Out-String).Trim())
        $result.abi=(Invoke-Adb $deviceSerial @('shell','getprop','ro.product.cpu.abi')|Out-String).Trim()
        if(Get-AgentPid $deviceSerial){throw 'An existing xtest-nova-agent process is active; refusing to replace it'}
        Assert-ValidationRemotePathsAbsent $deviceSerial @('/data/local/tmp/xtest-nova-agent','/data/local/tmp/xtest-nova-agent.log','/data/local/tmp/xtest-nova-runner.jar')
        Assert-ValidationRuntimeAbsent $deviceSerial
$artifact=if($result.abi -match 'arm64'){"$repoRoot\dist\xtest-nova-agent-arm64"}else{"$repoRoot\dist\xtest-nova-agent-armv7"}
        if(-not(Test-Path -LiteralPath $artifact)){throw "Missing artifact: $artifact"}
        Invoke-Adb $deviceSerial @('push',$artifact,'/data/local/tmp/xtest-nova-agent')|Out-Null
        Invoke-Adb $deviceSerial @('shell','chmod','755','/data/local/tmp/xtest-nova-agent')|Out-Null
        Start-Agent $deviceSerial;$owned=$true
        $localPort=Add-AgentForward $deviceSerial
        $health=Wait-Health $localPort
        $result.version=$health.version
        $initial=Invoke-RestMethod "http://127.0.0.1:$localPort/v1/diagnostics/runtime" -TimeoutSec 3
        $result.initialPid=Get-AgentPid $deviceSerial;$result.initialStartedAt=$initial.startedAt
        $initialHeap=[int64]$initial.heapAllocBytes;$initialSystem=[int64]$initial.systemBytes
        $maxHeap=$initialHeap;$maxSystem=$initialSystem;$maxGoroutines=[int]$initial.goroutines;$result.initialGoroutines=[int]$initial.goroutines
        $expectedPid=$result.initialPid
        $deadline=[DateTime]::UtcNow.AddSeconds($DurationSeconds)
        do {
            try {
                $snapshot=Invoke-RestMethod "http://127.0.0.1:$localPort/v1/diagnostics/runtime" -TimeoutSec 3
                $result.samples++
                $maxHeap=[Math]::Max($maxHeap,[int64]$snapshot.heapAllocBytes);$maxSystem=[Math]::Max($maxSystem,[int64]$snapshot.systemBytes);$maxGoroutines=[Math]::Max($maxGoroutines,[int]$snapshot.goroutines)
                foreach($property in $snapshot.sessions.PSObject.Properties){if($property.Value -eq $true){$result.idleSessions=$false}}
                if((Get-AgentPid $deviceSerial) -ne $expectedPid){$result.healthFailures++}
            } catch {$result.healthFailures++}
            if([DateTime]::UtcNow -lt $deadline){Start-Sleep -Seconds $SampleIntervalSeconds}
        } while([DateTime]::UtcNow -lt $deadline)
        $result.pidStable=((Get-AgentPid $deviceSerial) -eq $expectedPid)
        $result.maxGoroutines=$maxGoroutines;$result.heapGrowthBytes=$maxHeap-$initialHeap;$result.systemGrowthBytes=$maxSystem-$initialSystem

        Remove-AgentForward $deviceSerial $localPort;$localPort=0
        $localPort=Add-AgentForward $deviceSerial
        $null=Wait-Health $localPort;$result.forwardRecovered=$true

        $initialLog=(Invoke-Adb $deviceSerial @('shell','cat','/data/local/tmp/xtest-nova-agent.log')|Out-String)
        Stop-OwnedAgent $deviceSerial;Start-Sleep -Milliseconds 300;Start-Agent $deviceSerial
        $null=Wait-Health $localPort
        $restarted=Invoke-RestMethod "http://127.0.0.1:$localPort/v1/diagnostics/runtime" -TimeoutSec 3
        $result.restartedPid=Get-AgentPid $deviceSerial;$result.restartedAt=$restarted.startedAt
        $result.processRecovered=($result.restartedAt -ne $result.initialStartedAt -and $result.restartedPid -ne '' -and $result.restartedPid -ne $result.initialPid)
        $logText=(Invoke-Adb $deviceSerial @('shell','cat','/data/local/tmp/xtest-nova-agent.log')|Out-String)
        $result.panicCount=([regex]::Matches($initialLog+$logText,'(?im)^panic:')).Count
        $result.passed=($result.healthFailures -eq 0 -and $result.pidStable -and $result.idleSessions -and $result.forwardRecovered -and $result.processRecovered -and $result.panicCount -eq 0 -and $result.maxGoroutines -le ($result.initialGoroutines+20) -and $result.heapGrowthBytes -lt 67108864 -and $result.systemGrowthBytes -lt 67108864)
    } catch {$result.error=$_.Exception.Message}
    finally {if($owned){Stop-OwnedAgent $deviceSerial;Remove-OwnedValidationRuntime $deviceSerial;& adb -s $deviceSerial shell rm -f /data/local/tmp/xtest-nova-agent /data/local/tmp/xtest-nova-agent.log 2>$null|Out-Null};Remove-AgentForward $deviceSerial $localPort}
    $results+=[pscustomobject]$result
}
$versions=@($results|Where-Object passed|Select-Object -ExpandProperty version -Unique)
$report=[ordered]@{schemaVersion='xtest-device-matrix/v1';generatedAt=[DateTime]::UtcNow.ToString('o');durationSeconds=$DurationSeconds;sampleIntervalSeconds=$SampleIntervalSeconds;passed=(($results|Where-Object{-not $_.passed}).Count -eq 0 -and $versions.Count -eq 1);devices=$results}
$directory=Split-Path -Parent $OutputPath
if($directory){New-Item -ItemType Directory -Force -Path $directory|Out-Null}
$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8
$report|ConvertTo-Json -Depth 8
if(-not $report.passed){exit 1}
