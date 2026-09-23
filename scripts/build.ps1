$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    $goExe = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    if (!(Test-Path -LiteralPath $goExe)) { $goExe = (Get-Command go -ErrorAction Stop).Source }
    $oldCgo = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = '0'
        New-Item -ItemType Directory -Force build | Out-Null
        & $goExe build -o build/roon-cover.exe ./cmd/roon-cover
        if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
        Write-Output 'Built build/roon-cover.exe; no SDL or Pango DLLs are required.'
    } finally { $env:CGO_ENABLED = $oldCgo }
} finally { Pop-Location }
