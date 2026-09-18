param(
    [string]$OutputPath=(Join-Path $HOME '.xtest-nova-api-token'),
    [switch]$Force
)
$ErrorActionPreference='Stop'

$resolved=[IO.Path]::GetFullPath($OutputPath)
$parent=Split-Path -Parent $resolved
if(-not(Test-Path -LiteralPath $parent -PathType Container)){throw "Token directory does not exist: $parent"}
if((Test-Path -LiteralPath $resolved)-and-not$Force){throw "Token file already exists; use -Force to rotate it: $resolved"}

$bytes=New-Object byte[] 32
$rng=[Security.Cryptography.RandomNumberGenerator]::Create()
$temporary=Join-Path $parent ('.xtest-nova-api-token.'+[Guid]::NewGuid().ToString('N')+'.tmp')
try{
    $rng.GetBytes($bytes)
    $token=[Convert]::ToBase64String($bytes)
    [IO.File]::WriteAllText($temporary,$token,[Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporary -Destination $resolved -Force:$Force
}finally{
    $rng.Dispose()
    [Array]::Clear($bytes,0,$bytes.Length)
    $token=$null
    Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
}

Write-Host "LAN API token file created: $resolved"
Write-Host 'Keep this file outside the repository and provide it with deploy.ps1 -ApiTokenFile.'
