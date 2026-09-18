function Get-ReleaseTimestamp {
    param([string]$RepoRoot)
    $epochText=$env:SOURCE_DATE_EPOCH
    if([string]::IsNullOrWhiteSpace($epochText)){
        $previousErrorAction=$ErrorActionPreference
        try{
            $ErrorActionPreference='Continue'
            $epochText=(& git -C $RepoRoot log -1 --format=%ct 2>$null|Out-String).Trim()
            $gitExitCode=$LASTEXITCODE
        }finally{$ErrorActionPreference=$previousErrorAction}
        if($gitExitCode-ne 0-or[string]::IsNullOrWhiteSpace($epochText)){$epochText=[DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString([Globalization.CultureInfo]::InvariantCulture)}
    }
    [int64]$epoch=0
    if(-not[Int64]::TryParse($epochText,[Globalization.NumberStyles]::None,[Globalization.CultureInfo]::InvariantCulture,[ref]$epoch)-or$epoch-lt 0){throw 'SOURCE_DATE_EPOCH must be a non-negative integer'}
    return [DateTimeOffset]::FromUnixTimeSeconds($epoch).UtcDateTime.ToString('yyyy-MM-ddTHH:mm:ssZ',[Globalization.CultureInfo]::InvariantCulture)
}

function ConvertTo-DeterministicText {
    param([AllowEmptyString()][string]$Text)
    return (($Text-replace"`r`n","`n")-replace"`r","`n").TrimEnd()+"`n"
}

function Write-Utf8NoBomText {
    param([string]$Path,[AllowEmptyString()][string]$Text)
    [IO.File]::WriteAllText($Path,(ConvertTo-DeterministicText $Text),(New-Object Text.UTF8Encoding($false)))
}
