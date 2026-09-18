param(
    [Parameter(Mandatory)][string]$Serial,
    [Parameter(Mandatory)][ValidateRange(1,65535)][int]$AgentPort,
    [string]$FixturePackage='com.xtest.nova.fixture',
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\side-effect-fixtures-latest.json')
)
$ErrorActionPreference='Stop'
$controlHeaders=@{'X-XTest-Control'='true'}
function Get-OwnedHeaders($State){return @{'X-XTest-Session-Id'=[string]$State.identity.sessionId;'X-XTest-Owner-Token'=[string]$State.identity.ownerToken}}

function Invoke-Adb([string[]]$Arguments) {
    $output=& adb -s $Serial @Arguments
    if($LASTEXITCODE -ne 0){throw "adb failed: $Arguments"}
    return $output
}
function Add-Step([System.Collections.Generic.List[object]]$Steps,[string]$Name,[string]$Status,[string]$Detail) {
    $Steps.Add([pscustomobject]@{name=$Name;status=$Status;passed=($Status -ne 'failed');detail=$Detail})
}
function Get-Wakefulness {
    $text=(Invoke-Adb @('shell','dumpsys','power')|Out-String)
    if($text -match 'mWakefulness=(Awake|Asleep|Dreaming|Dozing)'){return $matches[1]}
    return 'Unknown'
}
function Remove-RemoteFile([string]$Path) {
    $allowed=$Path.StartsWith('/data/local/tmp/xtest-nova-fixture-') -or $Path.StartsWith("/sdcard/xtest-nova/$FixturePackage/")
    if(-not $allowed){throw "refusing cleanup outside fixture roots: $Path"}
    Invoke-Adb @('shell','rm','-f','--',$Path)|Out-Null
    & adb -s $Serial shell test '!' -e $Path
    if($LASTEXITCODE -ne 0){throw "remote file remains after cleanup: $Path"}
}

if($Serial -notmatch '^[A-Za-z0-9._:-]+$'){throw'Invalid device serial'}
if($FixturePackage -notmatch '^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$'){throw'Invalid fixture package'}
$base="http://127.0.0.1:$AgentPort"
$steps=[System.Collections.Generic.List[object]]::new()
$cleanup=[System.Collections.Generic.List[string]]::new()
$originalConfig=$null;$originalConfigJSON='';$configChanged=$false;$performanceStarted=$false;$recordStarted=$false;$popupStarted=$false;$minitouchOwned=$false
$originalWakefulness=Get-Wakefulness
$report=[ordered]@{
    schemaVersion='xtest-side-effect-fixtures/v1';generatedAt=[DateTime]::UtcNow.ToString('o');serial=$Serial
    sdk=[int]((Invoke-Adb @('shell','getprop','ro.build.version.sdk')|Out-String).Trim())
    model=(Invoke-Adb @('shell','getprop','ro.product.model')|Out-String).Trim()
    package=$FixturePackage;passed=$false;steps=$steps
}
try {
    $health=Invoke-RestMethod "$base/v1/health" -TimeoutSec 5
    if($health.status -ne 'ok'){throw'Agent is not healthy'}
    $runtime=Invoke-RestMethod "$base/v1/diagnostics/runtime" -TimeoutSec 5
    foreach($property in $runtime.sessions.PSObject.Properties){if($property.Value -eq $true){throw "active session blocks isolated fixture: $($property.Name)"}}
    Add-Step $steps 'preflight-isolation' 'passed' "version=$($health.version); no active sessions"

    $token=[Guid]::NewGuid().ToString('N')
    $remote="/data/local/tmp/xtest-nova-fixture-$token.txt";$cleanup.Add($remote)
    $local=[IO.Path]::GetTempFileName()
    try {
        $expected="xtest-side-effect-$token";[IO.File]::WriteAllText($local,$expected,[Text.UTF8Encoding]::new($false))
        $upload=Invoke-RestMethod "$base/upload$remote" -Method Post -Form @{file=Get-Item -LiteralPath $local;mode='0600'} -TimeoutSec 20
        $actual=(Invoke-WebRequest "$base/raw$remote" -TimeoutSec 10).Content
        $info=Invoke-RestMethod "$base/finfo$remote" -TimeoutSec 10
        if($actual -ne $expected -or $upload.target -ne $remote -or $info.path -ne $remote){throw'file round-trip mismatch'}
        Add-Step $steps 'file-upload-read-info-cleanup' 'passed' "$remote; bytes=$($expected.Length)"
    } finally {Remove-Item -LiteralPath $local -Force -ErrorAction SilentlyContinue}

    $originalConfig=Invoke-RestMethod "$base/pullConfig" -TimeoutSec 10
    $originalConfigJSON=$originalConfig|ConvertTo-Json -Depth 30 -Compress
    $emptyConfig=@{autoClickByText=@();autoClickByResourceId=@();autoInputByHint=@();autoInputByResourceId=@()}
    $null=Invoke-RestMethod "$base/pushConfig" -Method Post -Headers $controlHeaders -ContentType 'application/json' -Body($emptyConfig|ConvertTo-Json -Depth 5 -Compress) -TimeoutSec 10
    $configChanged=$true
    $roundTrip=Invoke-RestMethod "$base/pullConfig" -TimeoutSec 10
    if(@($roundTrip.autoClickByText).Count -ne 0){throw'isolated config did not persist'}
    Add-Step $steps 'config-replace-restore' 'passed' 'empty non-clicking configuration persisted; original queued for restore'

    $popup=Invoke-RestMethod "$base/popupBoxAssistant" -Method Post -TimeoutSec 15;$popupStarted=$true
    if(-not $popup.running){throw'AutoPopup did not start'}
    $popup=Invoke-RestMethod "$base/popupBoxAssistant" -Method Delete -TimeoutSec 15;$popupStarted=$false
    if($popup.running -or $popup.stopping){throw'AutoPopup did not stop'}
    Add-Step $steps 'autopopup-owned-lifecycle' 'passed' 'started with empty rules and stopped cleanly'

    $null=Invoke-RestMethod "$base/session/$FixturePackage" -Method Post -TimeoutSec 20
    Start-Sleep -Milliseconds 500
    $foreground=(Invoke-RestMethod "$base/foregroundPkg" -TimeoutSec 10).package
    if($foreground -ne $FixturePackage){throw"fixture not foreground: $foreground"}
    Add-Step $steps 'target-launch' 'passed' $foreground

    $performanceState=Invoke-RestMethod "$base/v1/performance/sessions" -Method Post -ContentType 'application/json' -Body(@{package=$FixturePackage}|ConvertTo-Json -Compress) -TimeoutSec 15;$performanceStarted=$true
    Start-Sleep -Seconds 4
    $performance=Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers (Get-OwnedHeaders $performanceState) -TimeoutSec 15;$performanceStarted=$false
    if($performance.rows -lt 2 -or -not $performance.path){throw'performance artifact incomplete'}
    $cleanup.Add([string]$performance.path)
    Add-Step $steps 'performance-session-cleanup' 'passed' "rows=$($performance.rows); path=$($performance.path)"

    $null=Invoke-WebRequest "$base/screenrecord?package=$FixturePackage" -Method Post -TimeoutSec 15;$recordStarted=$true
    Start-Sleep -Seconds 3
    $record=Invoke-RestMethod "$base/screenrecord" -Method Put -TimeoutSec 20;$recordStarted=$false
    $video=@($record.videos)|Select-Object -First 1
    if(-not $video){throw'screenrecord returned no artifact'}
    $size=[int64]((Invoke-Adb @('shell','stat','-c','%s',$video)|Out-String).Trim())
    if($size -le 0){throw'screenrecord artifact is empty'}
    $cleanup.Add([string]$video)
    Add-Step $steps 'screenrecord-session-cleanup' 'passed' "bytes=$size; path=$video"

    $miniBefore=Invoke-WebRequest "$base/minitouch" -Method Put -SkipHttpErrorCheck -TimeoutSec 15
    if($miniBefore.StatusCode -eq 200){$minitouchOwned=$true;$null=Invoke-RestMethod "$base/minitouch" -Method Delete -TimeoutSec 15;$minitouchOwned=$false;Add-Step $steps 'minitouch-owned-lifecycle' 'passed' 'owned start and stop verified'}
    elseif($miniBefore.StatusCode -eq 503){Add-Step $steps 'minitouch-owned-lifecycle' 'qualified-skip' 'trusted device binary unavailable on this fixture'}
    else{throw"unexpected minitouch status: $($miniBefore.StatusCode)"}

    $beforeWake=Get-Wakefulness;$null=Invoke-RestMethod "$base/wakeupScreen" -TimeoutSec 10;$afterWake=Get-Wakefulness
    if($afterWake -ne 'Awake'){throw"wakeup failed: $beforeWake -> $afterWake"}
    Add-Step $steps 'wakeup-idempotence' 'passed' "$beforeWake -> $afterWake"
    $report.passed=(@($steps|Where-Object status -eq 'failed').Count -eq 0)
} catch {
    Add-Step $steps 'harness' 'failed' $_.Exception.Message
} finally {
    if($performanceStarted){try{$null=Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers (Get-OwnedHeaders $performanceState) -TimeoutSec 10}catch{}}
    if($recordStarted){try{$null=Invoke-RestMethod "$base/screenrecord" -Method Put -TimeoutSec 15}catch{}}
    if($popupStarted){try{$null=Invoke-RestMethod "$base/popupBoxAssistant" -Method Delete -TimeoutSec 10}catch{}}
    if($minitouchOwned){try{$null=Invoke-RestMethod "$base/minitouch" -Method Delete -TimeoutSec 10}catch{}}
    if($configChanged -and $null -ne $originalConfig){try{$null=Invoke-RestMethod "$base/pushConfig" -Method Post -Headers $controlHeaders -ContentType 'application/json' -Body $originalConfigJSON -TimeoutSec 10;$restoredConfig=Invoke-RestMethod "$base/pullConfig" -TimeoutSec 10;if(($restoredConfig|ConvertTo-Json -Depth 30 -Compress) -ne $originalConfigJSON){throw'configuration differs after restore'};Add-Step $steps 'config-final-restore' 'passed' 'original configuration restored and re-read'}catch{Add-Step $steps 'config-final-restore' 'failed' $_.Exception.Message}}
    $cleanupFailures=0
    foreach($path in $cleanup){try{Remove-RemoteFile $path}catch{$cleanupFailures++;Add-Step $steps 'artifact-cleanup' 'failed' "$path`: $($_.Exception.Message)"}}
    if($cleanupFailures -eq 0){Add-Step $steps 'artifact-cleanup' 'passed' "$($cleanup.Count) exact files removed and verified absent"}
    $currentWakefulness=Get-Wakefulness
    if($originalWakefulness -eq 'Asleep' -and $currentWakefulness -eq 'Awake'){try{Invoke-Adb @('shell','input','keyevent','KEYCODE_SLEEP')|Out-Null}catch{}}
    $post=Invoke-RestMethod "$base/v1/diagnostics/runtime" -TimeoutSec 5
    $active=@($post.sessions.PSObject.Properties|Where-Object Value -eq $true)
    if($active.Count){Add-Step $steps 'postflight-isolation' 'failed' "active sessions: $($active.Name -join ',')"}else{Add-Step $steps 'postflight-isolation' 'passed' 'no active sessions remain'}
    $report.passed=(@($steps|Where-Object status -eq 'failed').Count -eq 0)
    $directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force $directory|Out-Null}
    $report|ConvertTo-Json -Depth 10|Set-Content -LiteralPath $OutputPath -Encoding utf8
}
$report|ConvertTo-Json -Depth 10
if(-not $report.passed){exit 1}
