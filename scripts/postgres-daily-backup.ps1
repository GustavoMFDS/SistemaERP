#requires -Version 5.1
<#
  Esterium ERP: backup PostgreSQL for Windows Task Scheduler.
  Set PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSFILE in the task's
  protected environment (or via approved credential manager).
  Never store a password in the task command line or this script.
#>
param(
    [Parameter(Mandatory = $true)][string]$BackupDir,
    [string]$PostgresBin = 'C:\Program Files\PostgreSQL\16\bin',
    [ValidateRange(7, 3650)][int]$RetentionDays = 14,
    [string]$ExternalBackupDir = '',
    [switch]$RequireExternalCopy
)
$ErrorActionPreference = 'Stop'
foreach ($key in @('PGHOST', 'PGUSER', 'PGDATABASE')) {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($key))) {
        throw "Missing PostgreSQL variable: $key"
    }
}
if (-not [IO.Path]::IsPathRooted($BackupDir)) { throw 'BackupDir must be absolute.' }
if ($RequireExternalCopy -and [string]::IsNullOrWhiteSpace($ExternalBackupDir)) {
    throw 'An off-machine external backup destination is required.'
}
if (-not [string]::IsNullOrWhiteSpace($ExternalBackupDir)) {
    # A network SMB share has a separate failure domain from the shop PC.
    # USB drives require a separately approved and tested procedure.
    if ($ExternalBackupDir -notmatch '^\\\\[^\\]+\\[^\\]+') {
        throw 'ExternalBackupDir must be an SMB UNC share (\\server\share).'
    }
}
$full = [IO.Path]::GetFullPath($BackupDir).TrimEnd('\', '/')
$root = [IO.Path]::GetPathRoot($full).TrimEnd('\', '/')
if ($full -eq $root -or $full -eq [IO.Path]::GetTempPath().TrimEnd('\', '/')) {
    throw 'Unsafe backup directory.'
}
foreach ($exe in @('pg_dump.exe', 'pg_restore.exe', 'psql.exe')) {
    if (-not (Test-Path (Join-Path $PostgresBin $exe))) { throw "Missing PostgreSQL utility: $exe" }
}
[IO.Directory]::CreateDirectory($full) | Out-Null
$lock = $null
$tmp = $null
$ok = $false
$externalVerified = $false
$status = Join-Path $full 'backup-status.json'
try {
    $lock = [IO.File]::Open((Join-Path $full '.esterium-backup.lock'),
        [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $psql = Join-Path $PostgresBin 'psql.exe'
    $pgDump = Join-Path $PostgresBin 'pg_dump.exe'
    $pgRestore = Join-Path $PostgresBin 'pg_restore.exe'

    $tables = & $psql -v ON_ERROR_STOP=1 -At -c @'
SELECT count(*) FROM information_schema.tables
WHERE table_schema='public' AND table_name IN
('companies','products','sales','sale_items','inventory_balances','invoices','audit_logs')
'@
    if ($LASTEXITCODE -ne 0 -or [string]$tables -ne '7') {
        throw 'Critical ERP tables are missing.'
    }

    $stamp = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ')
    $unique = [Guid]::NewGuid().ToString('N').Substring(0, 8)
    $name = "esterium-$stamp-$unique.dump"
    $tmp = Join-Path $full ".$unique.tmp"
    $final = Join-Path $full $name

    & $pgDump --format=custom --compress=6 --no-owner --no-privileges "--file=$tmp"
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $tmp) -or (Get-Item $tmp).Length -eq 0) {
        throw 'pg_dump failed or produced an empty file.'
    }
    & $pgRestore --list $tmp | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Archive directory validation failed.' }
    Move-Item -LiteralPath $tmp -Destination $final -ErrorAction Stop
    $tmp = $null

    $hash = (Get-FileHash -LiteralPath $final -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText("$final.sha256", "$hash  $name" + [Environment]::NewLine,
        [Text.UTF8Encoding]::new($false))
    $verify = (Get-FileHash -LiteralPath $final -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne $verify) { throw 'SHA256 verification failed.' }

    if (-not [string]::IsNullOrWhiteSpace($ExternalBackupDir)) {
        & (Join-Path $PSScriptRoot 'copy-verified-external-backup.ps1') -SourceDump $final -DestinationDir $ExternalBackupDir | Out-Null
        $externalVerified = $true
    }
    $ok = $true
    $cutoff = [DateTime]::UtcNow.AddDays(-$RetentionDays)
    foreach ($old in @(Get-ChildItem -LiteralPath $full -Filter 'esterium-*.dump' -File)) {
        if ($old.LastWriteTimeUtc -ge $cutoff -or -not (Test-Path "$($old.FullName).sha256")) { continue }
        Remove-Item -LiteralPath $old.FullName -Force
        Remove-Item -LiteralPath "$($old.FullName).sha256" -Force
    }
    Write-Output "PASS: backup verified: $name"
}
catch {
    Write-Error ('Esterium backup failed: ' + $_.Exception.Message) -ErrorAction Continue
    throw
}
finally {
    if ($tmp -and (Test-Path $tmp)) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
    try {
        $state = if ($ok) { 'ok' } else { 'failed' }
        $report = @{
            last_run_utc = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
            state = $state
            archive_verified = $ok
            external_copy_verified = $externalVerified
        } | ConvertTo-Json -Compress
        $tempStatus = Join-Path $full ('.esterium-status-' + [Guid]::NewGuid().ToString('N') + '.tmp')
        [IO.File]::WriteAllText($tempStatus, $report, [Text.UTF8Encoding]::new($false))
        Move-Item -LiteralPath $tempStatus -Destination $status -Force
    }
    catch { Write-Warning 'Could not persist sanitized backup status.' }
    if ($lock) { $lock.Dispose() }
}
