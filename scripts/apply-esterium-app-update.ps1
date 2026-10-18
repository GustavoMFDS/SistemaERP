#requires -Version 5.1
<#
  Fail-closed APP-FILES-ONLY Windows update, NEVER a SQL migration.
  Default is a read-only plan; -Apply requires admin and a signed candidate.
  A synthetic unsigned fixture is permitted ONLY in GitHub-hosted Windows CI.
  Never touches PostgreSQL/Redis, backup files, customer data or SEFAZ.
#>
param(
    [Parameter(Mandatory = $true)][string]$InstalledDir,
    [Parameter(Mandatory = $true)][string]$CandidateBundleDir,
    [Parameter(Mandatory = $true)][string]$BackupDir,
    [Parameter(Mandatory = $true)][string]$ExternalBackupDir,
    [switch]$Apply,
    [string]$ApprovedManifestSHA256 = '',
    [string]$TrustedPublisherThumbprint = '',
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows is required for an Esterium app update.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned update fixture is restricted to disposable GitHub-hosted Windows SCM CI.'
}
$oldPath = (Resolve-Path -LiteralPath $InstalledDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$newPath = (Resolve-Path -LiteralPath $CandidateBundleDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$plan = Join-Path $PSScriptRoot 'plan-esterium-windows-upgrade.ps1'
$planArgs = @{
    InstalledDir=$oldPath
    CandidateBundleDir=$newPath
    BackupDir=$BackupDir
    ExternalBackupDir=$ExternalBackupDir
}
if ($UnsignedDisposableCIFixture) {
    $planArgs.TechnicalUnsignedPlan = $true
    $planArgs.SyntheticCIFixture = $true
}
& $plan @planArgs | Out-Null

# This updater has no privilege to apply SQL. Even *adding* a migration must
# cause a refusal; those upgrades need a separate reviewed DB recovery plan.
$oldSQL = @(Get-ChildItem -LiteralPath (Join-Path $oldPath 'migrations') -File -Filter '*.sql')
$newSQL = @(Get-ChildItem -LiteralPath (Join-Path $newPath 'migrations') -File -Filter '*.sql')
if ($oldSQL.Count -ne $newSQL.Count) {
    throw 'Application-only updater refuses added or removed database migrations.'
}
$oldByName = @{}
foreach ($file in $oldSQL) {
    $oldByName[$file.Name] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
}
foreach ($file in $newSQL) {
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
    if (-not $oldByName.ContainsKey($file.Name) -or $oldByName[$file.Name] -ne $hash) {
        throw 'Application-only updater refuses rewritten SQL migrations.'
    }
}
if (-not $Apply) {
    Write-Output 'PLAN: same SQL schema; signed package, stopped service and recent off-machine backup checked.'
    Write-Output 'PLAN: application folder could be replaced with rollback folder preserved; no changes made.'
    Write-Output 'PLAN: this tool does not migrate PostgreSQL, start any Windows service or transmit SEFAZ.'
    return
}
if (-not $UnsignedDisposableCIFixture) {
    if ([string]::IsNullOrWhiteSpace($ApprovedManifestSHA256) -or
        [string]::IsNullOrWhiteSpace($TrustedPublisherThumbprint)) {
        throw 'Real update requires a separately approved manifest SHA-256 and trusted publisher thumbprint.'
    }
    $approvalArgs = @{
        BundleDir = $newPath
        ExpectedManifestSHA256 = $ApprovedManifestSHA256
        ExpectedPublisherThumbprint = $TrustedPublisherThumbprint
    }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') @approvalArgs | Out-Null
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator elevation required for an application-only package update.'
}
$parent = Split-Path -Parent $oldPath
$guid = [Guid]::NewGuid().ToString('N')
$staging = Join-Path $parent ('.esterium-update-stage-' + $guid)
$rollback = Join-Path $parent ('.esterium-update-rollback-' + $guid)
$oldMoved = $false
$newMoved = $false
try {
    if (Test-Path -LiteralPath $staging) { throw 'Update staging path already exists.' }
    if (Test-Path -LiteralPath $rollback) { throw 'Rollback path already exists.' }
    Copy-Item -LiteralPath $newPath -Destination $staging -Recurse -ErrorAction Stop
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $staging | Out-Null
    # Re-check all critical preconditions after copying and immediately before
    # the directory swap; never trust an earlier backup/service inspection.
    & $plan @planArgs | Out-Null
    $currentService = @(Get-CimInstance -ClassName Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
    if ($currentService.Count -ne 1 -or $currentService[0].State -ne 'Stopped') {
        throw 'The shop application service must remain stopped during replacement.'
    }
    Move-Item -LiteralPath $oldPath -Destination $rollback -ErrorAction Stop
    $oldMoved = $true
    Move-Item -LiteralPath $staging -Destination $oldPath -ErrorAction Stop
    $newMoved = $true
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $oldPath | Out-Null
    Write-Output 'PASS: application-only files updated with the prior full bundle kept intact in a rollback folder.'
    Write-Output "ROLLBACK FOLDER: $rollback"
    Write-Output 'Service remains STOPPED, backups untouched; do not start until the independent technical acceptance check.'
} catch {
    if ($oldMoved) {
        if ($newMoved -and (Test-Path -LiteralPath $oldPath)) {
            $failedNew = Join-Path $parent ('.esterium-update-failed-' + $guid)
            try { Move-Item -LiteralPath $oldPath -Destination $failedNew -ErrorAction Stop }
            catch { Write-Warning 'Could not isolate failed new application; manual recovery required.' }
        }
        if (-not (Test-Path -LiteralPath $oldPath) -and
            (Test-Path -LiteralPath $rollback)) {
            try { Move-Item -LiteralPath $rollback -Destination $oldPath -ErrorAction Stop }
            catch { Write-Warning 'Automatic application rollback failed; preserve old bundle and inspect manually.' }
        }
    }
    throw
} finally {
    if (-not $oldMoved -and (Test-Path -LiteralPath $staging)) {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
}
