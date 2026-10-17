#requires -Version 5.1
<#
  Read-only verification of the most recent verified local and external
  PostgreSQL backup. Never restores, deletes or opens PostgreSQL.
  Non-UNC external directory is permitted ONLY for synthetic CI fixtures.
#>
param(
    [Parameter(Mandatory = $true)][string]$BackupDir,
    [Parameter(Mandatory = $true)][string]$ExternalBackupDir,
    [ValidateRange(1, 168)][int]$MaxAgeHours = 30,
    [switch]$SyntheticCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($SyntheticCIFixture) {
    if ($env:GITHUB_ACTIONS -ne 'true' -or
        $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
        $env:OS -ne 'Windows_NT') {
        throw 'Synthetic fixture mode is restricted to disposable GitHub-hosted Windows CI.'
    }
}
elseif ($ExternalBackupDir -notmatch '^\\[^\\]+\\[^\\]+') {
    throw 'ExternalBackupDir must point to another host over an SMB UNC share.'
}

$local = (Resolve-Path -LiteralPath $BackupDir -ErrorAction Stop).Path
$external = (Resolve-Path -LiteralPath $ExternalBackupDir -ErrorAction Stop).Path
if ($local.TrimEnd('\') -eq $external.TrimEnd('\')) {
    throw 'Local and external backup directories must be different.'
}
$statusPath = Join-Path $local 'backup-status.json'
if (-not (Test-Path -LiteralPath $statusPath -PathType Leaf)) {
    throw 'Backup status is missing.'
}
$status = Get-Content -LiteralPath $statusPath -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
if ($status.state -ne 'ok' -or $status.archive_verified -ne $true -or
    $status.external_copy_verified -ne $true) {
    throw 'A completed local and external backup has not been verified.'
}

$name = [string]$status.archive_name
$expected = [string]$status.archive_sha256
if ($name -notmatch '^esterium-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}\.dump$' -or
    $expected -notmatch '^[a-f0-9]{64}$') {
    throw 'Backup status has an invalid archive name or checksum.'
}

try {
    $when = [DateTimeOffset]::Parse([string]$status.last_run_utc,
        [Globalization.CultureInfo]::InvariantCulture,
        [Globalization.DateTimeStyles]::AssumeUniversal)
}
catch {
    throw 'Backup status has an invalid timestamp.'
}
$age = ([DateTimeOffset]::UtcNow - $when.ToUniversalTime()).TotalHours
if ($age -lt (-5.0 / 60.0) -or $age -gt $MaxAgeHours) {
    throw 'Verified external backup is too old or has an invalid future timestamp.'
}

$sidecarText = "$expected  $name"
foreach ($dir in @($local, $external)) {
    $archive = Join-Path $dir $name
    $checksumPath = "$archive.sha256"
    foreach ($item in @($archive, $checksumPath)) {
        if (-not (Test-Path -LiteralPath $item -PathType Leaf)) {
            throw 'One of the verified backup files is missing.'
        }
        $info = Get-Item -LiteralPath $item -Force
        if (($info.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'A backup file is an unexpected symbolic link.'
        }
    }
    if ((Get-Item -LiteralPath $archive).Length -le 0) {
        throw 'A backup archive is empty.'
    }
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw 'Local or external backup content does not match the recorded SHA-256.'
    }
    $sidecar = (Get-Content -LiteralPath $checksumPath -Raw).Trim()
    if ($sidecar -cne $sidecarText) {
        throw 'Local or external checksum file does not match the verified archive.'
    }
}
Write-Output 'PASS: recent local and off-machine PostgreSQL backup archives and checksums match.'
