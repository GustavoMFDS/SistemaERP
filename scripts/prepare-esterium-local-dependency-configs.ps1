#requires -Version 5.1
<#
  One command to prepare *new* Windows PostgreSQL installer options and Garnet
  TLS/ACL/Lua configuration. Default: dry run. No third-party binaries run.
  Explicit Apply requires an approved signed launcher or disposable hosted CI.
  Secrets are NEVER passed as process arguments or printed to the console.
#>
param(
    [Parameter(Mandatory=$true)][string]$PostgresSuperPasswordFile,
    [Parameter(Mandatory=$true)][string]$PostgresServicePasswordFile,
    [Parameter(Mandatory=$true)][string]$GarnetAclFile,
    [Parameter(Mandatory=$true)][string]$GarnetCertificatePfxFile,
    [Parameter(Mandatory=$true)][string]$GarnetCertificatePasswordFile,
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$GarnetConfigDir=(Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [string]$PostgresInstallDir=(Join-Path $env:ProgramFiles 'PostgreSQL\16'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Dependency configuration requires Windows.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned dependency configuration fixture requires disposable hosted Windows CI.'
}
if ($PostgresPort -eq $GarnetPort -or $PostgresPort -in @(6379,8080) -or
    $GarnetPort -in @(5432,8080)) {
    throw 'Database and cache must have distinct local ports, not the ERP web port.'
}
$pgOptions=[IO.Path]::GetFullPath($PostgresOptionsDir).TrimEnd('\')
$garnetOptions=[IO.Path]::GetFullPath($GarnetConfigDir).TrimEnd('\')
if ([string]::Equals($pgOptions,$garnetOptions,[StringComparison]::OrdinalIgnoreCase) -or
    $pgOptions.StartsWith($garnetOptions + '\',[StringComparison]::OrdinalIgnoreCase) -or
    $garnetOptions.StartsWith($pgOptions + '\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'PostgreSQL and Garnet configuration directories must be separate.'
}
$pgTool=Join-Path $PSScriptRoot 'provision-esterium-postgres-optionfile.ps1'
$garnetTool=Join-Path $PSScriptRoot 'provision-esterium-garnet-config.ps1'
foreach ($path in @($pgTool,$garnetTool)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw 'Required local prerequisite configuration tool is missing.'
    }
}
$pgArgs=@{
    SuperPasswordFile=$PostgresSuperPasswordFile
    ServicePasswordFile=$PostgresServicePasswordFile
    DestinationDir=$pgOptions
    PostgresInstallDir=$PostgresInstallDir
    DataDir=$PostgresDataDir
    ServiceName=$PostgresServiceName
    ServiceAccount=$PostgresServiceAccount
    Port=$PostgresPort
}
$garnetArgs=@{
    AclFile=$GarnetAclFile
    CertificatePfxFile=$GarnetCertificatePfxFile
    CertificatePasswordFile=$GarnetCertificatePasswordFile
    DestinationDir=$garnetOptions
    Port=$GarnetPort
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[0-9A-Fa-f]{40}$') {
        throw 'Real prerequisite configuration needs an independently approved publisher thumbprint.'
    }
    foreach ($path in @($PSCommandPath,$pgTool,$garnetTool)) {
        $signature=Get-AuthenticodeSignature -LiteralPath $path
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Prerequisite configuration must be executed using signed, approved Windows tools.'
        }
    }
}
# Each helper also protects its direct -Apply entry point; forward the
# already-verified publisher identity, or only the explicitly isolated
# hosted-CI fixture identity, to both helpers.
if ($Apply) {
    if ($UnsignedDisposableCIFixture) {
        $pgArgs.UnsignedDisposableCIFixture=$true
        $garnetArgs.UnsignedDisposableCIFixture=$true
    } else {
        $pgArgs.TrustedPublisherThumbprint=$TrustedPublisherThumbprint
        $garnetArgs.TrustedPublisherThumbprint=$TrustedPublisherThumbprint
    }
}
# Always check both inputs and destinations before any file is created.
& $pgTool @pgArgs | Out-Null
Write-Output 'PASS 1/2: new PostgreSQL installer options can be prepared.'
& $garnetTool @garnetArgs | Out-Null
Write-Output 'PASS 2/2: new Garnet protected configuration can be prepared.'
if (-not $Apply) {
    Write-Output 'PLAN: both prerequisite configuration checks passed without writing anything.'
    Write-Output 'NOT INSTALLED: PostgreSQL, Garnet, database, Windows accounts/services, TLS listeners or SEFAZ.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator approval is required to create protected prerequisite settings.'
}
# Each helper is fail-closed and creates only a fresh, protected directory.
# If the second helper fails, retain the first directory as protected evidence,
# never delete a path that might have been replaced by another process.
& $pgTool @pgArgs -Apply | Out-Null
try {
    & $garnetTool @garnetArgs -Apply | Out-Null
} catch {
    Write-Warning 'Garnet configuration was not committed; protected PostgreSQL options were retained for manual inspection.'
    throw
}
Write-Output 'PASS: new PostgreSQL installer options and Garnet ACL/TLS/Lua configuration were created.'
Write-Output 'NOT EXECUTED: third-party installers, account creation, Windows service start, SQL migrations, store data or SEFAZ.'
