param(
    [Parameter(Mandatory)][string]$Serial,
    [Parameter(Mandatory)][ValidateRange(1,65535)][int]$AgentPort,
    [string]$FixturePackage='com.xtest.nova.fixture',
    [string]$ComposePackage='',
    [string]$GamePackage='',
    [string]$OutputPath=(Join-Path $PSScriptRoot '..\reports\rendering-qualification-latest.json')
)
$ErrorActionPreference='Stop'
function Invoke-Adb([string[]]$Arguments){$output=& adb -s $Serial @Arguments;if($LASTEXITCODE -ne 0){throw "adb failed: $Arguments"};return $output}
if($Serial -notmatch '^[A-Za-z0-9._:-]+$'){throw'Invalid serial'}
$base="http://127.0.0.1:$AgentPort";$performanceStarted=$false
$report=[ordered]@{schemaVersion='xtest-rendering-qualification/v1';generatedAt=[DateTime]::UtcNow.ToString('o');serial=$Serial;model='';sdk=0;passed=$false;surface=$null;gpu=$null;compose=$null;game=$null}
try{
    $report.model=(Invoke-Adb @('shell','getprop','ro.product.model')|Out-String).Trim();$report.sdk=[int]((Invoke-Adb @('shell','getprop','ro.build.version.sdk')|Out-String).Trim())
    $health=Invoke-RestMethod "$base/v1/health" -TimeoutSec 5;if($health.status -ne 'ok'){throw'Agent unhealthy'}
    Invoke-Adb @('shell','am','force-stop',$FixturePackage)|Out-Null
    Invoke-Adb @('shell','am','start','-n',"$FixturePackage/.SurfaceLoopActivity")|Out-Null
    Start-Sleep -Seconds 1
    $layers=(Invoke-Adb @('shell','dumpsys','SurfaceFlinger','--list')|Out-String)
    if($layers -notmatch [regex]::Escape("SurfaceView[$FixturePackage/")){throw'target SurfaceView layer not found'}
    $performanceState=Invoke-RestMethod "$base/v1/performance/sessions" -Method Post -ContentType 'application/json' -Body(@{package=$FixturePackage}|ConvertTo-Json -Compress) -TimeoutSec 15;$performanceStarted=$true
    Start-Sleep -Seconds 10
    $headers=@{'X-XTest-Session-Id'=[string]$performanceState.identity.sessionId;'X-XTest-Owner-Token'=[string]$performanceState.identity.ownerToken};$state=Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers $headers -TimeoutSec 20;$performanceStarted=$false
    $csvText=(Invoke-Adb @('shell','cat',$state.path)|Out-String);$samples=@($csvText|ConvertFrom-Csv|ForEach-Object{if($_.fps -ne ''){[double]$_.fps}})
    $validSurfaceSamples=@($samples|Where-Object{$_ -ge 20.0})
    if($validSurfaceSamples.Count -lt 3){throw"Surface loop produced fewer than 3 valid FPS samples (>=20 FPS): $($validSurfaceSamples.Count) of $($samples.Count)"}
    $display=(Invoke-Adb @('shell','dumpsys','display')|Out-String);$match=[regex]::Match($display,'mActiveRenderFrameRate=([0-9.]+)')
    if(-not$match.Success){throw'active display refresh rate unavailable'};$refresh=[double]$match.Groups[1].Value
    $minimum=($validSurfaceSamples|Measure-Object -Minimum).Minimum;$maximum=($validSurfaceSamples|Measure-Object -Maximum).Maximum;$average=($validSurfaceSamples|Measure-Object -Average).Average
    $report.surface=[ordered]@{status='passed';activity="$FixturePackage/.SurfaceLoopActivity";displayRefreshHz=$refresh;fpsSamples=$samples.Count;qualifiedFpsSamples=$validSurfaceSamples.Count;fpsMin=[math]::Round($minimum,2);fpsMax=[math]::Round($maximum,2);fpsAverage=[math]::Round($average,2);artifactPath=$state.path}

    Invoke-Adb @('shell','am','force-stop',$FixturePackage)|Out-Null
    Invoke-Adb @('shell','am','start','-n',"$FixturePackage/.GLESLoopActivity")|Out-Null
    Start-Sleep -Seconds 1
    $performanceState=Invoke-RestMethod "$base/v1/performance/sessions" -Method Post -ContentType 'application/json' -Body(@{package=$FixturePackage}|ConvertTo-Json -Compress) -TimeoutSec 15;$performanceStarted=$true
    Start-Sleep -Seconds 8
    $headers=@{'X-XTest-Session-Id'=[string]$performanceState.identity.sessionId;'X-XTest-Owner-Token'=[string]$performanceState.identity.ownerToken};$gpuState=Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers $headers -TimeoutSec 20;$performanceStarted=$false
    $gpuCSV=(Invoke-Adb @('shell','cat',$gpuState.path)|Out-String)|ConvertFrom-Csv
    $gpuSamples=@($gpuCSV|ForEach-Object{if($_.gpu_percent -ne ''){[double]$_.gpu_percent}})
    if($gpuSamples.Count -lt 3){throw"insufficient GPU samples: $($gpuSamples.Count)"}
    $gpuMaximum=($gpuSamples|Measure-Object -Maximum).Maximum;$gpuAverage=($gpuSamples|Measure-Object -Average).Average
    if($gpuMaximum -le 0){throw'OpenGL ES loop produced no measurable GPU utilization'}
    $report.gpu=[ordered]@{status='passed';activity="$FixturePackage/.GLESLoopActivity";source=$gpuState.last.gpu.source;samples=$gpuSamples.Count;percentMax=[math]::Round($gpuMaximum,2);percentAverage=[math]::Round($gpuAverage,2);artifactPath=$gpuState.path}
    $report.compose=if($ComposePackage){[ordered]@{status='not-run';package=$ComposePackage;reason='Compose package execution is not implemented by this generic Surface fixture'}}else{[ordered]@{status='pending';reason='no controlled Compose target APK supplied'}}
    $report.game=if($GamePackage){[ordered]@{status='not-run';package=$GamePackage;reason='real-game package requires a separately approved, non-destructive scenario'}}else{[ordered]@{status='synthetic-gpu-loop';package=$FixturePackage;reason='independent Surface and OpenGL ES game loops passed; no commercial game target supplied'}}
    $report.passed=$true
}catch{$report.error=$_.Exception.Message}
finally{
    if($performanceStarted){try{$headers=@{'X-XTest-Session-Id'=[string]$performanceState.identity.sessionId;'X-XTest-Owner-Token'=[string]$performanceState.identity.ownerToken};$null=Invoke-RestMethod "$base/v1/performance/sessions/current" -Method Delete -Headers $headers -TimeoutSec 15}catch{}}
    & adb -s $Serial shell am force-stop $FixturePackage 2>$null|Out-Null
    $directory=Split-Path -Parent $OutputPath;if($directory){New-Item -ItemType Directory -Force $directory|Out-Null};$report|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $OutputPath -Encoding utf8
}
$report|ConvertTo-Json -Depth 8
if(-not$report.passed){exit 1}
