#requires -Version 5.1
<#
  Register the prebuilt Esterium Go process as a Windows service.
  This script does NOT install PostgreSQL/Redis or generate any credentials.
  PlanOnly is side-effect-free; regular mode requires elevation.
#>
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [Parameter(Mandatory = $true)][string]$DatabaseURLFile,
    [Parameter(Mandatory = $true)][string]$RedisURLFile,
    [Parameter(Mandatory = $true)][string]$JWTSecretFile,
    [Parameter(Mandatory = $true)][string]$MetricsBearerTokenFile,
    [Parameter(Mandatory = $true)][string]$AllowedOrigin,
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [switch]$PlanOnly
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT') { throw 'Esterium Windows service installation requires Windows.' }
$serviceName = 'EsteriumERP'
$origin = $null
if (-not [Uri]::TryCreate($AllowedOrigin, [UriKind]::Absolute, [ref]$origin) -or
    $origin.Scheme -ne 'https' -or [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.Host -in @('localhost', '127.0.0.1', '::1') -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or $origin.Fragment -ne '') {
    throw 'AllowedOrigin must be an actual HTTPS origin for this installation (not localhost).'
}
$source = (Resolve-Path -LiteralPath $BundleDir -ErrorAction Stop).Path
$binarySource = Join-Path $source 'esterium-server.exe'
$webSource = Join-Path $source 'web'
if (-not (Test-Path -LiteralPath $binarySource -PathType Leaf)) {
    throw 'The Windows bundle has no esterium-server.exe'
}
if (-not (Test-Path -LiteralPath (Join-Path $webSource 'index.html') -PathType Leaf)) {
    throw 'The Windows bundle has no compiled web/index.html'
}

$installPath = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
$programFilesRoot = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
if (-not $installPath.StartsWith($programFilesRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'InstallDir must be a dedicated subdirectory of Program Files.'
}
if (Test-Path -LiteralPath $installPath) {
    throw 'The installation directory already exists; refusing to replace existing program or data.'
}
$secrets = @(
    @{ name = 'DATABASE_URL_FILE'; file = $DatabaseURLFile },
    @{ name = 'REDIS_URL_FILE'; file = $RedisURLFile },
    @{ name = 'JWT_SECRET_FILE'; file = $JWTSecretFile },
    @{ name = 'METRICS_BEARER_TOKEN_FILE'; file = $MetricsBearerTokenFile }
)
foreach ($secret in $secrets) {
    if (-not [IO.Path]::IsPathRooted($secret.file) -or
        -not (Test-Path -LiteralPath $secret.file -PathType Leaf) -or
        (Get-Item -LiteralPath $secret.file).Length -le 0) {
        throw "Missing, empty or non-absolute secret file: $($secret.name)"
    }
    $secretPath = [IO.Path]::GetFullPath($secret.file)
    if ($secretPath.StartsWith($source.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase) -or
        $secretPath.StartsWith($installPath + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw "Secret files must be outside the bundle and install folder: $($secret.name)"
    }
}
$sc = Join-Path $env:SystemRoot 'System32\sc.exe'
if (-not (Test-Path -LiteralPath $sc)) { throw 'sc.exe is unavailable.' }

# Do not inspect secret contents or echo credential values.
$serviceExists = & $sc query $serviceName 2>$null
if ($LASTEXITCODE -eq 0) { throw "Windows service $serviceName already exists; refusing replacement." }

if ($PlanOnly) {
    Write-Output 'PLAN: verify signed/approved bundle and service-account ACLs; no changes made.'
    Write-Output "PLAN: stage Windows x64 executable and static frontend in $installPath"
    Write-Output 'PLAN: register EsteriumERP as LocalService; use protected secret file paths only.'
    Write-Output 'PLAN: APP_ENV=prod, FISCAL_PROVIDER=disabled, explicit CORS origin, loopback HTTP only.'
    Write-Output 'PLAN: no database installation, no migration, no service start, no SEFAZ.'
    return
}
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($current)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run elevated as administrator to register a Windows service.'
}

$parent = Split-Path -Parent $installPath
$staging = Join-Path $parent ('.esterium-stage-' + [Guid]::NewGuid().ToString('N'))
$registered = $false
try {
    New-Item -ItemType Directory -Path $staging -Force | Out-Null
    Copy-Item -LiteralPath $binarySource -Destination (Join-Path $staging 'esterium-server.exe') -ErrorAction Stop
    Copy-Item -LiteralPath $webSource -Destination (Join-Path $staging 'web') -Recurse -ErrorAction Stop
    if (-not (Test-Path -LiteralPath (Join-Path $staging 'web\index.html'))) {
        throw 'Staging bundle validation failed.'
    }
    Move-Item -LiteralPath $staging -Destination $installPath -ErrorAction Stop
    $binary = Join-Path $installPath 'esterium-server.exe'
    $out = & $sc create $serviceName 'binPath=' ('"' + $binary + '"') 'start=' 'demand' 'obj=' 'NT AUTHORITY\LocalService' 'DisplayName=' 'Esterium ERP' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Windows service registration failed: $out" }
    $registered = $true
    $regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName"
    $entries = @(
        'APP_ENV=prod',
        'FISCAL_PROVIDER=disabled',
        'NFCE_SEFAZ_PRODUCTION_ENABLED=false',
        'NFCE_SEFAZ_HOMOLOGATION_ENABLED=false',
        'HTTP_ADDR=127.0.0.1:8080',
        "CORS_ALLOWED_ORIGINS=$($origin.GetLeftPart([UriPartial]::Authority))",
        "ESTERIUM_WEB_DIR=$(Join-Path $installPath 'web')"
    )
    foreach ($secret in $secrets) {
        $entries += "$($secret.name)=$([IO.Path]::GetFullPath($secret.file))"
    }
    New-ItemProperty -Path $regPath -Name Environment -PropertyType MultiString -Value $entries -Force -ErrorAction Stop | Out-Null
    $sidResult = & $sc sidtype $serviceName unrestricted 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not configure restricted service SID: $sidResult" }
    Write-Output 'PASS: service registered with only secret file paths (not credential values).'
    Write-Output 'Service start remains manual until database/migrations, ACL and backup restoration are tested.'
    Write-Output 'Do not enable SEFAZ. Local installation is not a certified production deployment.'
}
finally {
    if (Test-Path -LiteralPath $staging) {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (-not $registered -and (Test-Path -LiteralPath $installPath)) {
        Write-Warning 'Unregistered program files remain for manual inspection; they were not deleted.'
    }
}
