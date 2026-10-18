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
    [string]$ListenAddress = '127.0.0.1:8080',
    [string]$TLSCertFile = '',
    [string]$TLSKeyFile = '',
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [switch]$PlanOnly
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT') { throw 'Esterium Windows service installation requires Windows.' }
$serviceName = 'EsteriumERP'
$origin = $null
if (-not [Uri]::TryCreate($AllowedOrigin, [UriKind]::Absolute, [ref]$origin) -or
    $origin.Scheme -ne 'https' -or $origin.UserInfo -ne '' -or [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.Host -in @('localhost', '127.0.0.1', '::1') -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or $origin.Fragment -ne '') {
    throw 'AllowedOrigin must be an actual HTTPS origin for this installation (not localhost).'
}
# Only a well-formed IP/localhost listener is accepted by this initial
# installer. Exposing 0.0.0.0, :: or a LAN IP without TLS is forbidden.
$uri = $null
$targetAddress = if ($ListenAddress.StartsWith(':')) {
    "http://0.0.0.0$ListenAddress"
} else {
    "http://$ListenAddress"
}
if (-not [Uri]::TryCreate($targetAddress, [UriKind]::Absolute, [ref]$uri) -or
    $uri.Scheme -ne 'http' -or $uri.UserInfo -ne '' -or $uri.Port -le 0 -or $uri.Port -gt 65535 -or
    $uri.AbsolutePath -ne '/' -or $uri.Query -ne '' -or $uri.Fragment -ne '') {
    throw 'ListenAddress must be an IP or localhost with a valid explicit port.'
}
$ip = $null
$isLocalhost = $uri.Host -eq 'localhost'
$isIP = [Net.IPAddress]::TryParse($uri.Host, [ref]$ip)
if (-not ($isLocalhost -or $isIP) -or
    $ListenAddress -notmatch ':\d{1,5}$') {
    throw 'ListenAddress must use localhost or an IP address and an explicit port.'
}
$isLoopback = $isLocalhost -or ($isIP -and [Net.IPAddress]::IsLoopback($ip))
$hasTLSCert = -not [string]::IsNullOrWhiteSpace($TLSCertFile)
$hasTLSKey = -not [string]::IsNullOrWhiteSpace($TLSKeyFile)
if ($hasTLSCert -ne $hasTLSKey) {
    throw 'TLSCertFile and TLSKeyFile must both be supplied for LAN HTTPS.'
}
if (-not $isLoopback -and -not $hasTLSCert) {
    throw 'Non-loopback LAN listeners require an external TLS certificate and private key.'
}
$source = (Resolve-Path -LiteralPath $BundleDir -ErrorAction Stop).Path
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $source | Out-Null
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
if ($hasTLSCert) {
    $secrets += @{ name = 'ESTERIUM_TLS_KEY_FILE'; file = $TLSKeyFile }
    $secrets += @{ name = 'ESTERIUM_TLS_CERT_FILE'; file = $TLSCertFile }
}
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
function Assert-SecureSecretFile([string]$filePath, [string]$variableName) {
    # Only the service identity, SYSTEM and Administrators may read credentials.
    # A service account must never be able to overwrite its own trust material.
    $item = Get-Item -LiteralPath $filePath -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "A credential file must not be a symbolic link: $variableName"
    }
    $acl = Get-Acl -LiteralPath $filePath
    $hasLocalServiceRead = $false
    foreach ($entry in $acl.Access) {
        try {
            $sid = $entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        } catch {
            throw "Could not verify all file ACL identities for $variableName."
        }
        if ($entry.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) {
            continue
        }
        $canRead = ($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
        if (-not $canRead) { continue }
        if ($sid -notin @('S-1-5-19', 'S-1-5-18', 'S-1-5-32-544')) {
            throw "Secret file is readable by a non-approved Windows identity: $variableName"
        }
        if ($sid -eq 'S-1-5-19') {
            $unsafeWrites = [Security.AccessControl.FileSystemRights]::WriteData -bor
                [Security.AccessControl.FileSystemRights]::AppendData -bor
                [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
                [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
                [Security.AccessControl.FileSystemRights]::Delete -bor
                [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
                [Security.AccessControl.FileSystemRights]::TakeOwnership
            if (($entry.FileSystemRights -band $unsafeWrites) -ne 0) {
                throw "LocalService must not be able to change credential files: $variableName"
            }
            $hasLocalServiceRead = $true
        }
    }
    if (-not $hasLocalServiceRead) {
        throw "LocalService needs explicit read access to $variableName (file ACL)."
    }
}
if (-not $PlanOnly) {
    foreach ($secret in $secrets) {
        Assert-SecureSecretFile -filePath $secret.file -variableName $secret.name
    }
}
$sc = Join-Path $env:SystemRoot 'System32\sc.exe'
if (-not (Test-Path -LiteralPath $sc)) { throw 'sc.exe is unavailable.' }

# Do not inspect secret contents or echo credential values.
$serviceExists = & $sc query $serviceName 2>$null
$serviceQueryCode = $LASTEXITCODE
if ($serviceQueryCode -eq 0) { throw "Windows service $serviceName already exists; refusing replacement." }
if ($serviceQueryCode -ne 1060) {
    throw "Cannot verify whether the Windows service exists (SCM status $serviceQueryCode)."
}
# Expected SC query 1060 (missing service) must not become the script exit code.
$global:LASTEXITCODE = 0

if ($PlanOnly) {
    Write-Output 'PLAN: verify signed/approved bundle and service-account ACLs; no changes made.'
    Write-Output "PLAN: stage Windows x64 executable and static frontend in $installPath"
    Write-Output 'PLAN: register EsteriumERP as LocalService; use protected secret file paths only.'
    if ($isLoopback -and -not $hasTLSCert) {
        Write-Output 'PLAN: APP_ENV=prod, fiscal disabled, loopback-only HTTP (external HTTPS proxy required for LAN).'
    } else {
        Write-Output 'PLAN: APP_ENV=prod, fiscal disabled, HTTPS listener with externally managed certificates.'
    }
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
$createdService = $false
$registrationComplete = $false
try {
    New-Item -ItemType Directory -Path $staging -Force | Out-Null
    Copy-Item -LiteralPath $binarySource -Destination (Join-Path $staging 'esterium-server.exe') -ErrorAction Stop
    Copy-Item -LiteralPath $webSource -Destination (Join-Path $staging 'web') -Recurse -ErrorAction Stop
    # Preserve the verified backup, secret and installer utilities for future
    # maintenance; the extracted ZIP does not need to remain on the shop PC.
    Copy-Item -LiteralPath (Join-Path $source 'tools') -Destination (Join-Path $staging 'tools') -Recurse -ErrorAction Stop
    Copy-Item -LiteralPath (Join-Path $source 'migrations') -Destination (Join-Path $staging 'migrations') -Recurse -ErrorAction Stop
    Copy-Item -LiteralPath (Join-Path $source 'README_LOCAL.md') -Destination (Join-Path $staging 'README_LOCAL.md') -ErrorAction Stop
    Copy-Item -LiteralPath (Join-Path $source 'bundle-manifest.json') -Destination (Join-Path $staging 'bundle-manifest.json') -ErrorAction Stop
    # Do not register any Windows service until every staged file matches the
    # original manifest; a partial/altered copy must be rejected.
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $staging | Out-Null
    Move-Item -LiteralPath $staging -Destination $installPath -ErrorAction Stop
    $binary = Join-Path $installPath 'esterium-server.exe'
    $out = & $sc create $serviceName 'binPath=' ('"' + $binary + '"') 'start=' 'demand' 'obj=' 'NT AUTHORITY\LocalService' 'DisplayName=' 'Esterium ERP' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Windows service registration failed: $out" }
    $createdService = $true
    # Deterministic fault injection only on a disposable GitHub-hosted Windows runner.
    # Never start the service. Test the rollback of the just-created SCM entry.
    if ($env:GITHUB_ACTIONS -eq 'true' -and
        $env:RUNNER_ENVIRONMENT -eq 'github-hosted' -and
        $env:ESTERIUM_SCM_DISPOSABLE_CI -eq '1' -and
        $env:ESTERIUM_TEST_FAIL_AFTER_SC_CREATE -eq '1') {
        throw 'CI synthetic fault after Windows service creation.'
    }
    $regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$serviceName"
    $entries = @(
        'APP_ENV=prod',
        'FISCAL_PROVIDER=disabled',
        'NFCE_SEFAZ_PRODUCTION_ENABLED=false',
        'NFCE_SEFAZ_HOMOLOGATION_ENABLED=false',
        "HTTP_ADDR=$ListenAddress",
        "CORS_ALLOWED_ORIGINS=$($origin.GetLeftPart([UriPartial]::Authority))",
        "APP_PUBLIC_URL=$($origin.GetLeftPart([UriPartial]::Authority))",
        "ESTERIUM_WEB_DIR=$(Join-Path $installPath 'web')",
        "ESTERIUM_MIGRATIONS_DIR=$(Join-Path $installPath 'migrations')"
    )
    foreach ($secret in $secrets) {
        $entries += "$($secret.name)=$([IO.Path]::GetFullPath($secret.file))"
    }
    New-ItemProperty -Path $regPath -Name Environment -PropertyType MultiString -Value $entries -Force -ErrorAction Stop | Out-Null
    $sidResult = & $sc sidtype $serviceName unrestricted 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not configure restricted service SID: $sidResult" }
    $registrationComplete = $true
    Write-Output 'PASS: service registered with only secret file paths (not credential values).'
    Write-Output 'Service start remains manual until database/migrations, ACL and backup restoration are tested.'
    Write-Output 'Do not enable SEFAZ. Local installation is not a certified production deployment.'
}
finally {
    if ($createdService -and -not $registrationComplete) {
        # SCM was provably absent at entry. Roll back only the service created
        # by this invocation; preserve staged files for explicit inspection.
        $deleted = & $sc delete $serviceName 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Warning 'SCM rollback failed; inspect the partially registered EsteriumERP service manually.'
        } else {
            Write-Warning 'Partial service registration rolled back; program files preserved for inspection.'
        }
        $global:LASTEXITCODE = 0
    }
    if (Test-Path -LiteralPath $staging) {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (-not $registrationComplete -and (Test-Path -LiteralPath $installPath)) {
        Write-Warning 'Unregistered program files remain for manual inspection; they were not deleted.'
    }
}
