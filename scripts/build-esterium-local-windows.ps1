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

foreach ($tool in @('go', 'npm', 'curl.exe')) {
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
$toolsDir = Join-Path $OutputDir 'tools'
New-Item -ItemType Directory -Path $toolsDir -Force | Out-Null
# Garnet 2.2.3 is framework-dependent. Embed Microsoft's pinned .NET Runtime
# so a clean Windows installation does not depend on a preinstalled runtime.
$dotnetVersion = '10.0.14'
$dotnetZipSha512 = 'a176095f1db69b42c81acc60940744078d6bf2d2ac643e418342aa9c81889f4bd13e42f4621b131f9e77e4bc781ad46e375956219df9a4f979031d2a5a8d93cd'
$dotnetZip = Join-Path ([IO.Path]::GetTempPath()) ('esterium-dotnet-runtime-' + [Guid]::NewGuid().ToString('N') + '.zip')
try {
    $dotnetUrl = 'https://builds.dotnet.microsoft.com/dotnet/Runtime/' + $dotnetVersion + '/dotnet-runtime-' + $dotnetVersion + '-win-x64.zip'
    & curl.exe -fLsS --retry 2 --connect-timeout 25 --max-time 300 -o $dotnetZip $dotnetUrl
    if ($LASTEXITCODE -ne 0) { throw 'Official .NET runtime download failed.' }
    if ((Get-FileHash -LiteralPath $dotnetZip -Algorithm SHA512).Hash.ToLowerInvariant() -cne $dotnetZipSha512) {
        throw 'Official .NET runtime SHA-512 mismatch.'
    }
    $dotnetDir = Join-Path $toolsDir 'dotnet'
    New-Item -ItemType Directory -Path $dotnetDir -ErrorAction Stop | Out-Null
    Expand-Archive -LiteralPath $dotnetZip -DestinationPath $dotnetDir -ErrorAction Stop
    $dotnetExe = Join-Path $dotnetDir 'dotnet.exe'
    $runtimeCore = Join-Path $dotnetDir ('shared\Microsoft.NETCore.App\' + $dotnetVersion + '\System.Private.CoreLib.dll')
    if (-not (Test-Path -LiteralPath $dotnetExe -PathType Leaf) -or
        -not (Test-Path -LiteralPath $runtimeCore -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetDir 'ThirdPartyNotices.txt') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetDir 'LICENSE.txt') -PathType Leaf)) {
        throw 'Pinned .NET runtime package is incomplete or missing its license/notices.'
    }
    $runtimeList = & $dotnetExe --list-runtimes
    if ($LASTEXITCODE -ne 0 -or -not ($runtimeList -match ('Microsoft\.NETCore\.App ' + [Regex]::Escape($dotnetVersion)))) {
        throw 'Embedded .NET runtime smoke failed.'
    }
} finally {
    if (Test-Path -LiteralPath $dotnetZip) {
        Remove-Item -LiteralPath $dotnetZip -Force -ErrorAction SilentlyContinue
    }
}
# Compile local certificate generation with the standard library only; the
# Windows store receives a signed executable, never a Go toolchain or CA key.
$oldGOOS=$env:GOOS
$oldGOARCH=$env:GOARCH
try {
    $env:GOOS='windows'
    $env:GOARCH='amd64'
    Push-Location (Join-Path $Root 'backend')
    try {
        & go build -trimpath -o (Join-Path $toolsDir 'esterium-bootstrap-admin.exe') ./cmd/esterium-bootstrap-admin
        if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar bootstrap seguro do primeiro administrador.' }
        & go build -trimpath -o (Join-Path $toolsDir 'esterium-pg-tls-certgen.exe') ./cmd/pg-tls-certgen
        if ($LASTEXITCODE -ne 0) { throw 'Falha ao compilar gerador TLS PostgreSQL.' }
    } finally {
        Pop-Location
    }
} finally {
    $env:GOOS=$oldGOOS
    $env:GOARCH=$oldGOARCH
}
# The official golang-migrate CLI is build-time only and pinned to the same
# release used by the existing disposable database migration CI container.
# Never put databases, credentials or a Go toolchain in the retail bundle.
$migratorBuildDir = Join-Path ([IO.Path]::GetTempPath()) ('esterium-migrate-build-' + [Guid]::NewGuid().ToString('N'))
$previousGOBIN = $env:GOBIN
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
try {
    [IO.Directory]::CreateDirectory($migratorBuildDir) | Out-Null
    $env:GOBIN = $migratorBuildDir
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    & go install -tags postgres 'github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.1'
    if ($LASTEXITCODE -ne 0) { throw 'Build do migrador PostgreSQL falhou' }
    Copy-Item -LiteralPath (Join-Path $migratorBuildDir 'migrate.exe') -Destination (Join-Path $toolsDir 'esterium-migrate.exe') -ErrorAction Stop
} finally {
    $env:GOBIN = $previousGOBIN
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    if (Test-Path -LiteralPath $migratorBuildDir) {
        Remove-Item -LiteralPath $migratorBuildDir -Recurse -Force
    }
}
foreach ($tool in @(
    'register-esterium-windows-service.ps1',
        'provision-esterium-app-secrets.ps1',
    'verify-esterium-bundle.ps1',
    'postgres-daily-backup.ps1',
    'copy-verified-external-backup.ps1',
    'check-esterium-backup-integrity.ps1',
    'run-esterium-scheduled-backup.ps1',
    'register-esterium-backup-task.ps1',
    'check-esterium-windows-host.ps1',
    'unregister-esterium-windows-service.ps1',
    'check-esterium-local-dependencies.ps1',
    'check-esterium-lan-terminal.ps1',
    'plan-esterium-windows-upgrade.ps1',
    'plan-esterium-local-setup.ps1',
    'install-esterium-local.ps1',
    'apply-esterium-app-update.ps1',
    'restore-esterium-app-version.ps1',
    'check-esterium-restore-archive.ps1',
    'check-esterium-signed-release.ps1',
    'check-esterium-offline-dependency-packages.ps1',
    'provision-esterium-garnet-config.ps1',
    'provision-esterium-postgres-optionfile.ps1',
    'prepare-esterium-local-dependency-configs.ps1',
    'install-esterium-local-dependencies.ps1',
        'provision-esterium-dependency-bootstrap.ps1',
    'setup-esterium-local-dependencies.ps1',
    'check-esterium-postgres-tls-policy.ps1',
        'provision-esterium-postgres-tls-policy.ps1',
    'stage-esterium-postgres-tls.ps1',
    'initialize-esterium-postgres-app-db.ps1',
    'bootstrap-esterium-local-schema.ps1',
    'stage-esterium-approved-windows-release.ps1',
    'probe-esterium-local-first-run.ps1',
    'activate-esterium-local-first-install.ps1',
    'repair-esterium-local-install.ps1',
    'uninstall-esterium-local-app.ps1',
    'finish-esterium-local-first-install.ps1',
    'install-esterium-local-complete.ps1',
    'apply-esterium-initial-schema.ps1'
)) {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $tool) -Destination (Join-Path $toolsDir $tool) -ErrorAction Stop
}
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'bootstrap-esterium-first-admin.ps1') -Destination (Join-Path $toolsDir 'bootstrap-esterium-first-admin.ps1') -ErrorAction Stop
Copy-Item -LiteralPath (Join-Path $Root 'backend/seed/seed.prod.sql') -Destination (Join-Path $toolsDir 'seed.prod.sql') -ErrorAction Stop
Copy-Item -LiteralPath (Join-Path $Root 'backend/migrations') -Destination (Join-Path $OutputDir 'migrations') -Recurse -ErrorAction Stop
Copy-Item -LiteralPath (Join-Path $Root 'docs/esterium-local-service-registration.md') -Destination (Join-Path $OutputDir 'README_LOCAL.md') -ErrorAction Stop
& (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $OutputDir
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $OutputDir
Write-Host "Bundle ERP verificado em $OutputDir"
Write-Host "Este build gera os arquivos do ERP e os helpers da instalação assinada; não executa a instalação nem inclui o pacote PostgreSQL/Garnet."
