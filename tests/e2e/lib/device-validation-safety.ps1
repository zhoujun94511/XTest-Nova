function Assert-ValidationRemotePathsAbsent {
    param([Parameter(Mandatory)][string]$DeviceSerial,[Parameter(Mandatory)][string[]]$Paths)
    if($DeviceSerial-notmatch'^[A-Za-z0-9._:-]+$'){throw "Invalid device serial: $DeviceSerial"}
    foreach($path in $Paths){
        if($path-notmatch'^/data/local/tmp/[A-Za-z0-9._/-]+$'){throw "Unsafe validation path: $path"}
        $probe=(& adb -s $DeviceSerial shell sh -c "'if [ -e $path ]; then echo present; else echo absent; fi'" 2>$null|Out-String).Trim()
        if($LASTEXITCODE-ne 0-or$probe-notin@('present','absent')){throw "Unable to establish ownership of remote validation path: $path"}
        if($probe-eq'present'){throw "Remote validation path already exists; refusing to overwrite or delete it: $path"}
    }
}

$XTestBundledRuntimePackages=@('com.openatx.xtest.popup','com.openatx.xtest.nova.uiautomator','com.openatx.xtest.nova.uiautomator.test')
$XTestBundledRuntimePaths=@('/data/local/tmp/xtest-nova-runner.jar','/data/local/tmp/xtest-nova-companion.apk','/data/local/tmp/xtest-nova-uiautomator-host.apk','/data/local/tmp/xtest-nova-uiautomator-test.apk')
$XTestControlHeaders=@{'X-XTest-Control'='true'}

function Get-XTestOwnedHeaders {
    param([Parameter(Mandatory)]$State)
    if(-not$State.identity.sessionId-or-not$State.identity.ownerToken){throw'Execution ownership credentials are missing'}
    return @{
        'X-XTest-Control'='true'
        'X-XTest-Session-Id'=[string]$State.identity.sessionId
        'X-XTest-Owner-Token'=[string]$State.identity.ownerToken
    }
}

function Assert-ValidationRuntimeAbsent {
    param([Parameter(Mandatory)][string]$DeviceSerial)
    Assert-ValidationRemotePathsAbsent $DeviceSerial $XTestBundledRuntimePaths
    foreach($package in $XTestBundledRuntimePackages){
        $installedPath=(& adb -s $DeviceSerial shell pm list packages $package 2>$null|Out-String).Trim()
        if($LASTEXITCODE-ne 0){throw "Unable to inspect bundled runtime package ownership: $package"}
        if(@($installedPath-split"`n"|ForEach-Object{$_.Trim()}|Where-Object{$_-eq"package:$package"}).Count){throw "Bundled runtime package already exists; refusing to replace it in an isolated validation: $package"}
        if($installedPath){throw "Unexpected package inspection output for ${package}: $installedPath"}
    }
}

function Remove-OwnedValidationRuntime {
    param([Parameter(Mandatory)][string]$DeviceSerial)
    foreach($package in $XTestBundledRuntimePackages){& adb -s $DeviceSerial uninstall $package 2>$null|Out-Null}
    & adb -s $DeviceSerial shell rm -f @XTestBundledRuntimePaths 2>$null|Out-Null
}

function Stop-XTestOwnedExecution {
    param(
        [Parameter(Mandatory)][string]$BaseUrl,
        [Parameter(Mandatory)][string]$Path,
        $State,
        [int]$TimeoutSec=15
    )
    if($null-eq$State){
        $State=Invoke-RestMethod -Uri "$BaseUrl$Path" -TimeoutSec $TimeoutSec
    }
    if(-not$State.running){return $State}
    if(-not$State.identity.sessionId-or-not$State.identity.ownerToken){
        throw "Active execution does not expose ownership credentials: $Path"
    }
    $headers=Get-XTestOwnedHeaders $State
    return Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Delete -Headers $headers -TimeoutSec $TimeoutSec
}
