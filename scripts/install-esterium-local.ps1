#requires -Version 5.1
<#
  Install only the prebuilt Esterium Windows application/service.
  Strict default: read-only PLAN. -Apply requires administrator elevation and
  an Authenticode-signed server. Unsigned fixtures are allowed only on a
  disposable GitHub-hosted Windows CI runner, never on a shop computer.
  PostgreSQL and Redis must be provisioned separately. Does not start ERP,
  open a database, apply SQL, activate backup tasks, or enable SEFAZ.
#>
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [Parameter(Mandatory = $true)][string]$PostgresServiceName,
    [Parameter(Mandatory = $true)][string]$RedisServiceName,
    [Parameter(Mandatory = $true)][string]$PostgresBin,
    [Parameter(Mandatory = $true)][string]$DatabaseURLFile,
    [Parameter(Mandatory = $true)][string]$RedisURLFile,
    [Parameter(Mandatory = $true)][string]$JWTSecretFile,
    [Parameter(Mandatory = $true)][string]$MetricsBearerTokenFile,
    [Parameter(Mandatory = $true)][string]$AllowedOrigin,
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$ListenAddress = '127.0.0.1:8080',
    [string]$TLSCertFile = '',
    [string]$TLSKeyFile = '',
    [ValidateRange(1, 512)][int]$MinimumFreeGiB = 10,
    [ValidateRange(1, 65535)][int]$PostgresPort = 5432,
    [ValidateRange(1, 65535)][int]$RedisPort = 6379,
    [switch]$Apply,
    [string]$ApprovedManifestSHA256 = '',
    [string]$TrustedPublisherThumbprint = '',
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Esterium Local installation requires Windows.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned installation fixtures are permitted only on disposable hosted Windows SCM CI.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ([string]::IsNullOrWhiteSpace($ApprovedManifestSHA256) -or
        [string]::IsNullOrWhiteSpace($TrustedPublisherThumbprint)) {
        throw 'Real installation requires a separately approved manifest SHA-256 and trusted publisher thumbprint.'
    }
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Real deployment requires a valid independently approved publisher identity.'
    }
    # The OS/operator must authenticate this launcher BEFORE starting it:
    # an arbitrary unsigned PowerShell script cannot bootstrap its own trust.
    $gateScript = Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
    # Authentication of the launcher alone is not enough: its delegated
    # helper scripts also run in this elevated PowerShell process.
    # Validate every helper's approved publisher BEFORE the first helper call.
    $privilegedHelpers = @(
        'plan-esterium-local-setup.ps1',
        'check-esterium-windows-host.ps1',
        'check-esterium-local-dependencies.ps1',
        'register-esterium-windows-service.ps1',
        'verify-esterium-bundle.ps1'
    )
    $signedEntryPoints = @($PSCommandPath, $gateScript)
    $signedEntryPoints += @($privilegedHelpers | ForEach-Object { Join-Path $PSScriptRoot $_ })
    foreach ($scriptPath in $signedEntryPoints) {
        $signature = Get-AuthenticodeSignature -LiteralPath $scriptPath
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Privileged PowerShell entrypoint is not signed by the independently approved publisher.'
        }
    }
    $approvalArgs = @{
        BundleDir = $BundleDir
        ExpectedManifestSHA256 = $ApprovedManifestSHA256
        ExpectedPublisherThumbprint = $TrustedPublisherThumbprint
    }
    & $gateScript @approvalArgs | Out-Null
}
$planArgs = @{
    BundleDir=$BundleDir
    PostgresServiceName=$PostgresServiceName
    RedisServiceName=$RedisServiceName
    PostgresBin=$PostgresBin
    DatabaseURLFile=$DatabaseURLFile
    RedisURLFile=$RedisURLFile
    JWTSecretFile=$JWTSecretFile
    MetricsBearerTokenFile=$MetricsBearerTokenFile
    AllowedOrigin=$AllowedOrigin
    InstallDir=$InstallDir
    ListenAddress=$ListenAddress
    TLSCertFile=$TLSCertFile
    TLSKeyFile=$TLSKeyFile
    MinimumFreeGiB=$MinimumFreeGiB
    PostgresPort=$PostgresPort
    RedisPort=$RedisPort
    RequireSignedServer=(-not $UnsignedDisposableCIFixture)
    RequireServiceOwnership=$Apply
}
$planner = Join-Path $PSScriptRoot 'plan-esterium-local-setup.ps1'
& $planner @planArgs | Out-Null
if (-not $Apply) {
    Write-Output 'PLAN: Windows host, local dependencies, signed release and service registration checks passed.'
    Write-Output 'PLAN: explicit administrator -Apply would register ONLY the prebuilt Windows service, STOPPED and MANUAL.'
    Write-Output 'PLAN: no database, cache, schema, backup, network firewall or fiscal operation performed.'
    return
}
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($current)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator approval is required before any application files or Windows service are created.'
}
$registrar = Join-Path $PSScriptRoot 'register-esterium-windows-service.ps1'
$serviceArgs = @{
    BundleDir=$BundleDir
    DatabaseURLFile=$DatabaseURLFile
    RedisURLFile=$RedisURLFile
    JWTSecretFile=$JWTSecretFile
    MetricsBearerTokenFile=$MetricsBearerTokenFile
    AllowedOrigin=$AllowedOrigin
    ListenAddress=$ListenAddress
    TLSCertFile=$TLSCertFile
    TLSKeyFile=$TLSKeyFile
    InstallDir=$InstallDir
}
# The registrar also enforces release provenance independently because it
# can be called directly without the high-level installer. Forward approval
# only for an actual installation, not for planning.
if ($UnsignedDisposableCIFixture) {
    $serviceArgs.UnsignedDisposableCIFixture = $true
} else {
    $serviceArgs.ApprovedManifestSHA256 = $ApprovedManifestSHA256
    $serviceArgs.TrustedPublisherThumbprint = $TrustedPublisherThumbprint
}
# The registrar performs its own integrity and ACL checks and rolls back
# a partial SCM registration; it never starts or migrates anything.
& $registrar @serviceArgs | Out-Null
Write-Output 'PASS: Esterium Local app files and Windows service registered; service remains manual and stopped.'
Write-Output 'NOT INSTALLED: PostgreSQL/Redis, database schema, firewall rules, trusted LAN TLS or automatic backups.'
Write-Output 'NEXT: verify disposable schema and Redis primitives, independently test recovery; never enable fiscal transmission before SEFAZ acceptance.'
