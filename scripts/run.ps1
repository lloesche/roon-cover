param([Parameter(ValueFromRemainingArguments=$true)][string[]]$AppArgs)
$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    # Prefer the official Go installation over an MSYS2 toolchain on PATH.
    $goExe = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    if (!(Test-Path -LiteralPath $goExe)) { $goExe = (Get-Command go -ErrorAction Stop).Source }
    $oldCgo = $env:CGO_ENABLED
    try { $env:CGO_ENABLED = '0'; & $goExe run ./cmd/roon-cover @AppArgs; $result = $LASTEXITCODE }
    finally { $env:CGO_ENABLED = $oldCgo }
} finally { Pop-Location }
exit $result
