#requires -Version 5.1
<#
  One-command read-only planning for a NEW Esterium Local Windows PC.
  Never installs, updates, starts, stops or removes applications/services.
  PostgreSQL/Redis and protected app secrets must already be provisioned.
  A PASS is technical planning only; never a production/fiscal release gate.
#>
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [Parameter(Mandatory = $true)][string]$PostgresServiceName,
    [Parameter(Mandatory = $true)][string]$RedisServiceName,
    [Parameter(Mandatory = $true)][string]$PostgresBin,
    [Parameter(Mandatory = $true)][string]$DatabaseURLFile,
    [Parameter(Mandatory = $true)][string]$RedisURLFile,
    [string]$RedisCACertFile = '',
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
    [switch]$RequireSignedServer,
    [switch]$RequireServiceOwnership
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') {
    throw 'Esterium Local readiness planning requires Windows x64.'
}
$portMatch = [regex]::Match($ListenAddress, ':([0-9]{1,5})$')
if (-not $portMatch.Success) { throw 'ListenAddress requires an explicit numeric port.' }
$listenPort = [int]$portMatch.Groups[1].Value
if ($listenPort -lt 1 -or $listenPort -gt 65535) {
    throw 'ListenAddress port must be 1-65535.'
}
$toolDir = $PSScriptRoot
$hostCheck = Join-Path $toolDir 'check-esterium-windows-host.ps1'
$dependencyCheck = Join-Path $toolDir 'check-esterium-local-dependencies.ps1'
$servicePlan = Join-Path $toolDir 'register-esterium-windows-service.ps1'
foreach ($tool in @($hostCheck, $dependencyCheck, $servicePlan)) {
    if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) {
        throw 'An expected local readiness tool is missing.'
    }
}
Write-Output 'STEP 1/3: Windows host, executable and bundle integrity (read only).'
$hostArgs = @{
    BundleDir=$BundleDir; InstallDir=$InstallDir
    ListenPort=$listenPort; MinimumFreeGiB=$MinimumFreeGiB
}
if ($RequireSignedServer) { $hostArgs.RequireSignedServer = $true }
& $hostCheck @hostArgs | Out-Null
Write-Output 'PASS 1/3: host and technical package checks passed.'
Write-Output 'STEP 2/3: already installed PostgreSQL and Redis-compatible services (read only).'
$dependencyArgs = @{
    PostgresServiceName=$PostgresServiceName
    RedisServiceName=$RedisServiceName
    PostgresBin=$PostgresBin
    PostgresPort=$PostgresPort
    RedisPort=$RedisPort
    RequireServiceOwnership=$RequireServiceOwnership
}
& $dependencyCheck @dependencyArgs | Out-Null
Write-Output 'PASS 2/3: Windows dependency inventory checks passed (NOT TLS-certified).'
Write-Output 'STEP 3/3: proposed Windows service registration (dry run only).'
$serviceArgs = @{
    BundleDir=$BundleDir
    DatabaseURLFile=$DatabaseURLFile
    RedisURLFile=$RedisURLFile
    RedisCACertFile=$RedisCACertFile
    JWTSecretFile=$JWTSecretFile
    MetricsBearerTokenFile=$MetricsBearerTokenFile
    AllowedOrigin=$AllowedOrigin
    ListenAddress=$ListenAddress
    InstallDir=$InstallDir
    TLSCertFile=$TLSCertFile
    TLSKeyFile=$TLSKeyFile
    PlanOnly=$true
}
& $servicePlan @serviceArgs | Out-Null
Write-Output 'PASS 3/3: service registration plan passed without modifying Windows.'
Write-Output 'PASS: Esterium Local NEW-installation technical readiness plan only. NO CHANGES MADE.'
Write-Output 'NOT VERIFIED: file ACLs in active service context, PostgreSQL/Redis TLS identities and versions, schema, LAN browsers, backups/restore, printers or SEFAZ.'
Write-Output 'NEXT: provision trusted package signing, run Go --check-config and --check-dependencies with real configuration; then complete physical pilot checklist.'
