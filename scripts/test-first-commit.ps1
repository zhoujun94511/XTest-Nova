param([string]$ExpectedRemote='')
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$gitRepoRoot=$repoRoot.Replace('\','/')
Push-Location $repoRoot
try{
    $files=@(git -c "safe.directory=$gitRepoRoot" ls-files --cached --others --exclude-standard)
    if(-not$files){throw 'No first-commit candidates found'}
    foreach($required in @('LICENSE','NOTICE.md','CHANGELOG.md','README.md','docs/compliance/compatibility.md','docs/compliance/compatibility-waivers.md')){if($required-notin$files){throw "Missing required candidate: $required"}}
    $runtimeAssets=@('agent/internal/runtimebundle/assets/xtest-nova-runner.jar','agent/internal/runtimebundle/assets/xtest-nova-companion.apk','agent/internal/runtimebundle/assets/xtest-nova-uiautomator-host.apk','agent/internal/runtimebundle/assets/xtest-nova-uiautomator-test.apk')
    $forbidden=$files|Where-Object{($_-match'(^|/)(dist|reports|build|\.private)(/|$)|\.(apk|dex|keystore|jks|credentials\.psd1)$')-and$_-notin$runtimeAssets};if($forbidden){throw "Generated or private candidate found: $($forbidden-join', ')"}
    foreach($file in $files){$item=Get-Item -LiteralPath $file;if($item.Length-gt 1MB-and$file-ne'agent/internal/scrcpy/scrcpy-server-v4.1.jar'-and$file-notin$runtimeAssets){throw "Unexpected large candidate: $file"}}
    $jar='agent/internal/scrcpy/scrcpy-server-v4.1.jar';if((Get-FileHash -LiteralPath $jar -Algorithm SHA256).Hash-ne'DEACB991ED2509715160FFDC7907E47B4160EB30D1566217E9047FD5B8850CAE'){throw 'scrcpy server hash mismatch'}
    $textFiles=$files|Where-Object{$_-match'\.(go|java|ps1|md|json|xml|html|css|js|mod|sum|work|gitignore)$'-or$_-in@('LICENSE','NOTICE.md')}
    $patterns=@(
        '(?m)^\s*'+('Co-authored'+'-by:')
        'BEGIN '+('(RSA |EC |OPENSSH )?'+'PRIVATE KEY')
        ('github_'+'pat_')
        ('gh'+'p_')
        ('AI'+'za')
        ('Store'+'Password\s*=\s*["''][^"'']+')
        ('Key'+'Password\s*=\s*["''][^"'']+')
    )
    $matches=@($textFiles|ForEach-Object{Select-String -LiteralPath $_ -Pattern $patterns -CaseSensitive:$false})
    if($matches){throw "Sensitive or unwanted attribution text found: $($matches.Path|Sort-Object -Unique)"}
    $remote=''
    if(@(git -c "safe.directory=$gitRepoRoot" remote)-contains'origin'){$remote=(git -c "safe.directory=$gitRepoRoot" remote get-url origin|Out-String).Trim()}
    if($ExpectedRemote-and$remote.TrimEnd('/')-ne$ExpectedRemote.TrimEnd('/')){throw "Unexpected origin: $remote"}
    Write-Host "First-commit gate passed: $($files.Count) candidates"
}finally{Pop-Location}
