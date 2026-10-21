#requires -Version 5.1
<#
  Restore an earlier APP-ONLY Esterium bundle retained by a successful update.
  Read-only by default. NEVER touches PostgreSQL, Garnet, SQL, backups,
  certificates, ProgramData or fiscal records. The existing signed updater
  performs the verified, reversible directory swap.
  This is NOT database migration rollback.
#>
param(
    [Parameter(Mandatory = $true)][string]$InstalledDir,
    [Parameter(Mandatory = $true)][string]$PreviousVersionDir,
    [Parameter(Mandatory = $true)][string]$BackupDir,
    [Parameter(Mandatory = $true)][string]$ExternalBackupDir,
    [switch]$Apply,
    [switch]$ConfirmRollback,
    [string]$ApprovedManifestSHA256 = '',
    [string]$TrustedPublisherThumbprint = '',
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT') { throw 'Esterium application rollback requires Windows.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned rollback fixtures are restricted to disposable GitHub-hosted Windows CI.'
}
if ($Apply -and -not $ConfirmRollback) {
    throw 'Explicit -ConfirmRollback is required before restoring the previous application.'
}
if (-not [IO.Path]::IsPathRooted($InstalledDir) -or
    -not [IO.Path]::IsPathRooted($PreviousVersionDir)) {
    throw 'InstalledDir and PreviousVersionDir must be absolute paths.'
}
$installed = (Resolve-Path -LiteralPath $InstalledDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$previous = (Resolve-Path -LiteralPath $PreviousVersionDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if (-not $installed.StartsWith($programFiles + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Installed application must be inside a dedicated Program Files directory.'
}
$parent = Split-Path -Parent $installed
$previousParent = Split-Path -Parent $previous
$previousName = Split-Path -Leaf $previous
if (-not [string]::Equals($parent, $previousParent, [StringComparison]::OrdinalIgnoreCase) -or
    $previousName -notmatch '^\.esterium-update-rollback-[0-9a-f]{32}$') {
    throw 'PreviousVersionDir must be a preserved rollback sibling from the application updater.'
}
foreach ($path in @($installed, $previous)) {
    $info = Get-Item -LiteralPath $path -Force -ErrorAction Stop
    if (-not $info.PSIsContainer -or
        ($info.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Application and rollback bundles must be physical directories, not junctions.'
    }
}
$updater = Join-Path $PSScriptRoot 'apply-esterium-app-update.ps1'
if (-not (Test-Path -LiteralPath $updater -PathType Leaf)) {
    throw 'Verified application update helper is missing.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
        $TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Rollback requires independent approval of the previous release hash and publisher.'
    }
    # The update helper verifies itself and its privileged dependencies too.
    # Sign the rollback entry point or refuse to delegate privileged execution.
    $signature = Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
            $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Rollback launcher is not signed by the independently approved publisher.'
    }
}
$updaterArgs = @{
    InstalledDir = $installed
    CandidateBundleDir = $previous
    BackupDir = $BackupDir
    ExternalBackupDir = $ExternalBackupDir
}
if ($UnsignedDisposableCIFixture) {
    $updaterArgs.UnsignedDisposableCIFixture = $true
}
if ($ApprovedManifestSHA256) {
    $updaterArgs.ApprovedManifestSHA256 = $ApprovedManifestSHA256
}
if ($TrustedPublisherThumbprint) {
    $updaterArgs.TrustedPublisherThumbprint = $TrustedPublisherThumbprint
}
if (-not $Apply) {
    & $updater @updaterArgs | Out-Null
    Write-Output 'PLAN: previous application bundle is eligible for app-files-only rollback.'
    Write-Output 'PLAN: a stopped service, identical SQL and verified recent off-machine backup are required.'
    Write-Output 'PLAN: no files or services changed. Database schema rollback is NOT supported here.'
    return
}
# The update helper refuses any change to SQL files, rechecks the signer and
# the off-machine backup, validates staging and installed bytes, demands admin
# and leaves the currently installed version in ANOTHER rollback directory.
& $updater @updaterArgs -Apply
Write-Output 'PASS: application-only rollback completed; keep the newer bundle for investigation.'
Write-Output 'Service remains stopped; verify the application and data before manually restarting.'
