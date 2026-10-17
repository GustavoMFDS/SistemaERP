param(
    [string]$OutputDir = ""
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $Root 'dist\esterium-local-win-x64'
}
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)

foreach ($tool in @('go', 'npm')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "Ferramenta de compilação ausente: $tool. Execute este script na máquina de build, nunca no computador da loja."
    }
}

# Do not package or embed .env, credentials, demo seed, certificates or data.
$PreviousAPIBase = $env:VITE_API_BASE_URL
try {
    $env:VITE_API_BASE_URL = ''
    Push-Location (Join-Path $Root 'web')
    try {
        & npm ci
        if ($LASTEXITCODE -ne 0) { throw 'npm ci falhou' }
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw 'npm run build falhou' }
    } finally {
        Pop-Location
    }
} finally {
    $env:VITE_API_BASE_URL = $PreviousAPIBase
}

if (Test-Path $OutputDir) {
    throw "Pasta de saída já existe: $OutputDir. Escolha um caminho novo para não substituir dados."
}
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
$WebOut = Join-Path $OutputDir 'web'
Copy-Item -Path (Join-Path $Root 'web\dist') -Destination $WebOut -Recurse

Push-Location (Join-Path $Root 'backend')
try {
    $PreviousGOOS = $env:GOOS
    $PreviousGOARCH = $env:GOARCH
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        & go build -trimpath -o (Join-Path $OutputDir 'esterium-server.exe') ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw 'go build falhou' }
    } finally {
        $env:GOOS = $PreviousGOOS
        $env:GOARCH = $PreviousGOARCH
    }
} finally {
    Pop-Location
}

if (-not (Test-Path (Join-Path $WebOut 'index.html'))) {
    throw 'O frontend compilado não contém index.html'
}
Write-Host "Bundle L0 gerado em $OutputDir"
Write-Host "Este pacote não é instalador: PostgreSQL/Redis, segredos, serviço Windows, HTTPS LAN e backup externo ainda precisam ser configurados."
