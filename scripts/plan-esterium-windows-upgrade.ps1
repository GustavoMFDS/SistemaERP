#requires -Version 5.1
<#
  Technical ONLY: read-only pre-upgrade gate. Does not install or migrate SQL.
  Both packages must pass their own SHA-256 inventory checks; bundle SHA
  checks are not publisher signatures. Never opens PostgreSQL/Redis.
#>
param(
    [Parameter(Mandatory = $true)][string]$InstalledDir,
    [Parameter(Mandatory = $true)][string]$CandidateBundleDir,
    [Parameter(Mandatory = $true)][string]$BackupDir,
    [Parameter(Mandatory = $true)][string]$ExternalBackupDir,
    [switch]$TechnicalUnsignedPlan,
    [switch]$SyntheticCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows x64 is required.' }
if ($SyntheticCIFixture -and
    ($env:GITHUB_ACTIONS -ne 'true' -or
     $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
     $env:OS -ne 'Windows_NT')) {
    throw 'Synthetic fixture mode is restricted to disposable GitHub-hosted Windows CI.'
}
foreach ($path in @($InstalledDir, $CandidateBundleDir, $BackupDir)) {
    if (-not [IO.Path]::IsPathRooted($path)) { throw 'Every local upgrade path must be absolute.' }
}
$installed = (Resolve-Path -LiteralPath $InstalledDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$candidate = (Resolve-Path -LiteralPath $CandidateBundleDir -ErrorAction Stop).Path.TrimEnd('\', '/')
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if (-not $installed.StartsWith($programFiles + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'The existing installation must be under a dedicated Program Files directory.'
}
if ($installed -eq $candidate -or
    $installed.StartsWith($candidate + '\', [StringComparison]::OrdinalIgnoreCase) -or
    $candidate.StartsWith($installed + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Candidate bundle must be separate from the current installation.'
}
foreach ($path in @($installed, $candidate)) {
    $item = Get-Item -LiteralPath $path -Force
    if (-not $item.PSIsContainer -or
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Upgrade package paths must be physical directories.'
    }
    # Use only the verifier bundled with the currently reviewed tool, never
    # execute PowerShell obtained from a candidate or older installation.
    $verifier = Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'
    & $verifier -BundleDir $path | Out-Null
}
if (-not $TechnicalUnsignedPlan) {
    $signature = Get-AuthenticodeSignature -LiteralPath (Join-Path $candidate 'esterium-server.exe')
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid) {
        throw 'Unsigned candidate: a production update requires a valid executable signature.'
    }
} else {
    Write-Warning 'UNSIGNED TECHNICAL PLAN ONLY. SHA-256 is not a publisher authenticity proof.'
}
# Do not make a backup or connect to the DB here; require an already verified
# recent archive on another machine, with a matching SHA-256 sidecar.
$checker = Join-Path $PSScriptRoot 'check-esterium-backup-integrity.ps1'
$backupArgs = @{
    BackupDir=$BackupDir; ExternalBackupDir=$ExternalBackupDir
}
if ($SyntheticCIFixture) { $backupArgs.SyntheticCIFixture = $true }
& $checker @backupArgs | Out-Null

$svc = @(Get-CimInstance -ClassName Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
if ($svc.Count -ne 1) { throw 'EsteriumERP service must exist before upgrade.' }
$expected = Join-Path $installed 'esterium-server.exe'
$serviceExe = ([string]$svc[0].PathName).Trim().Trim('"')
if (-not [string]::Equals($serviceExe, $expected, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Windows service is not associated with this exact installed bundle.'
}
if ([string]$svc[0].State -ne 'Stopped') {
    throw 'Stop and reconcile all POS terminals before proposing an update.'
}
$backupTask = Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction SilentlyContinue
if ($null -ne $backupTask -and $backupTask.State -ne 'Disabled') {
    throw 'Disable and review the automatic backup task before a package update.'
}
function Get-MigrationHashMap([string]$root) {
    $records = @{}
    foreach ($file in @(Get-ChildItem -LiteralPath (Join-Path $root 'migrations') -File -Filter '*.sql')) {
        $records[$file.Name] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
    }
    return ,$records
}
$oldMigrations = Get-MigrationHashMap $installed
$newMigrations = Get-MigrationHashMap $candidate
if ($oldMigrations.Count -eq 0 -or $newMigrations.Count -lt $oldMigrations.Count) {
    throw 'A candidate cannot remove or downgrade installed SQL migrations.'
}
foreach ($name in $oldMigrations.Keys) {
    if (-not $newMigrations.ContainsKey($name) -or $newMigrations[$name] -ne $oldMigrations[$name]) {
        throw 'Candidate altered an existing versioned SQL migration.'
    }
}
$oldServer = (Get-FileHash -LiteralPath (Join-Path $installed 'esterium-server.exe') -Algorithm SHA256).Hash
$newServer = (Get-FileHash -LiteralPath (Join-Path $candidate 'esterium-server.exe') -Algorithm SHA256).Hash
if ($oldServer -eq $newServer -and $oldMigrations.Count -eq $newMigrations.Count) {
    throw 'Candidate server and migration inventory are identical; no software update identified.'
}
Write-Output 'PASS: read-only technical upgrade gate: verified archives, stopped service, disabled task and append-only SQL migration lineage.'
Write-Output 'NOT EXECUTED: database schema checks, SQL migrations, file replacement, service restart, backup restore, sales or SEFAZ.'
Write-Output 'A release update needs a signed package, a reversible deployment procedure and a fresh physical restore rehearsal.'
