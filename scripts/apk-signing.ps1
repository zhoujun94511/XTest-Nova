function Invoke-ApkSigner {
    param(
        [Parameter(Mandatory=$true)][string]$Signer,
        [Parameter(Mandatory=$true)][string]$KeyStore,
        [Parameter(Mandatory=$true)][string]$KeyAlias,
        [Parameter(Mandatory=$true)][securestring]$StorePassword,
        [Parameter(Mandatory=$true)][securestring]$KeyPassword,
        [Parameter(Mandatory=$true)][string]$OutputPath,
        [Parameter(Mandatory=$true)][string]$InputPath
    )

    $storeVariable='XTEST_APKSIGNER_STORE_'+[Guid]::NewGuid().ToString('N')
    $keyVariable='XTEST_APKSIGNER_KEY_'+[Guid]::NewGuid().ToString('N')
    $storeText=[System.Net.NetworkCredential]::new('', $StorePassword).Password
    $keyText=[System.Net.NetworkCredential]::new('', $KeyPassword).Password
    try {
        Set-Item -LiteralPath "Env:$storeVariable" -Value $storeText
        Set-Item -LiteralPath "Env:$keyVariable" -Value $keyText
        & $Signer sign --ks $KeyStore --ks-key-alias $KeyAlias --ks-pass "env:$storeVariable" --key-pass "env:$keyVariable" --out $OutputPath $InputPath
        if($LASTEXITCODE-ne 0){throw "apksigner failed: $LASTEXITCODE"}
    } finally {
        Remove-Item -LiteralPath "Env:$storeVariable","Env:$keyVariable" -Force -ErrorAction SilentlyContinue
        $storeText=$null
        $keyText=$null
    }
}
