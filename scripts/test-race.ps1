param([ValidateRange(1,100)][int]$Count=1)
$ErrorActionPreference='Stop'
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))

$previousCGO=$env:CGO_ENABLED
$previousCC=$env:CC
$previousPath=$env:PATH
$previousGoCache=$env:GOCACHE
try {
	$env:CGO_ENABLED='1'
	$env:GOCACHE=Join-Path $repoRoot '.tmp\gocache-race'
	New-Item -ItemType Directory -Force -Path $env:GOCACHE|Out-Null
    $arguments=@('test','-race')
    $runningOnWindows=$env:OS-eq'Windows_NT'
    if($runningOnWindows){
        $gcc='D:\Programs\msys64\ucrt64\bin\gcc.exe'
        if(-not(Test-Path -LiteralPath $gcc)){throw "Windows race tests require MSYS2 UCRT64 GCC: $gcc"}
        $env:CC=$gcc
        $env:PATH="$(Split-Path -Parent $gcc);$env:PATH"
        # Go's Windows ThreadSanitizer reserves a fixed shadow-memory range.
        # Disable ASLR only for temporary race-test binaries to avoid error 87
        # address collisions; production artifacts are not built by this script.
        $arguments+='-ldflags=-linkmode=external -extldflags=-Wl,--disable-dynamicbase'
    }
    $arguments+=@(((Join-Path $repoRoot 'agent')+'/...'),"-count=$Count")
    & go @arguments
    if($LASTEXITCODE-ne 0){throw "Go race tests failed: $LASTEXITCODE"}
} finally {
    if($null-eq$previousCGO){Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue}else{$env:CGO_ENABLED=$previousCGO}
	if($null-eq$previousCC){Remove-Item Env:CC -ErrorAction SilentlyContinue}else{$env:CC=$previousCC}
	if($null-eq$previousGoCache){Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue}else{$env:GOCACHE=$previousGoCache}
	$env:PATH=$previousPath
}
