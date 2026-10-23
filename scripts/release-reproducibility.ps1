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

function ConvertFrom-GitQuotedPath {
    param([AllowEmptyString()][string]$Path)
    if([string]::IsNullOrEmpty($Path)-or-not$Path.StartsWith('"')-or-not$Path.EndsWith('"')){return $Path}
    $inner=$Path.Substring(1,$Path.Length-2)
    $bytes=New-Object System.Collections.Generic.List[byte]
    $simple=@{a=7;b=8;t=9;n=10;v=11;f=12;r=13;'\'=92;'"'=34}
    for($i=0;$i-lt$inner.Length;$i++){
        $char=$inner[$i]
        if($char-eq'\'){
            if($i+1-ge$inner.Length){throw "Truncated Git quoted path: $Path"}
            $next=[string]$inner[$i+1]
            if($simple.ContainsKey($next)){$bytes.Add([byte]$simple[$next]);$i++;continue}
            if($next-match'^[0-7]$'-and$i+3-lt$inner.Length){
                $octal=$inner.Substring($i+1,3)
                if($octal-match'^[0-7]{3}$'){$bytes.Add([Convert]::ToByte($octal,8));$i+=3;continue}
            }
            throw "Unsupported Git path escape in $Path"
        }
        $code=[int]$char
        if($code-gt 127){throw "Unescaped non-ASCII in Git quoted path: $Path"}
        $bytes.Add([byte]$code)
    }
    return [Text.Encoding]::UTF8.GetString($bytes.ToArray())
}
