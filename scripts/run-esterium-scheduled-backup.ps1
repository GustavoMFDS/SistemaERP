#requires -Version 5.1
<#
  Daily backup task entrypoint for the Windows NetworkService identity.
  Config contains only non-secret settings and file paths, never passwords.
  -CheckOnly validates configuration without opening PostgreSQL/SMB/writing.
#>
param(
    [Parameter(Mandatory = $true)][string]$ConfigFile,
    [switch]$CheckOnly
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows backup task requires Windows.' }
if (-not [IO.Path]::IsPathRooted($ConfigFile) -or
    -not (Test-Path -LiteralPath $ConfigFile -PathType Leaf)) {
    throw 'Backup configuration must be an existing absolute file path.'
}
$absoluteConfig = [IO.Path]::GetFullPath($ConfigFile)
function Assert-NoSharedReaders([string]$path, [string]$label) {
    $item = Get-Item -LiteralPath $path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Backup $label must not be a symbolic link."
    }
    $acl = Get-Acl -LiteralPath $path
    $networkServiceCanRead = $false
    foreach ($ace in $acl.Access) {
        if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        $canRead = ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
        if (-not $canRead) { continue }
        try { $sid = $ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
        catch { throw "Cannot inspect all ACL readers for $label." }
        if ($sid -in @('S-1-1-0', 'S-1-5-11', 'S-1-5-32-545', 'S-1-5-7')) {
            throw "Backup $label is readable by a shared Windows user group."
        }
        if ($sid -eq 'S-1-5-20') { $networkServiceCanRead = $true }
    }
    if (-not $networkServiceCanRead) {
        throw "NetworkService needs an explicit file read ACL for backup $label."
    }
}
Assert-NoSharedReaders $absoluteConfig 'configuration'
$config = Get-Content -LiteralPath $absoluteConfig -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
if ($null -eq $config -or $config -isnot [PSCustomObject]) {
    throw 'Backup configuration must be a JSON object.'
}
$allowed = @('PGHOST', 'PGPORT', 'PGUSER', 'PGDATABASE', 'PGPASSFILE',
             'PGSSLROOTCERT', 'PostgresBin', 'BackupDir', 'ExternalBackupDir',
             'RetentionDays')
$fields = @($config.PSObject.Properties.Name)
foreach ($field in $fields) {
    if ($field -cnotin $allowed) {
        throw 'Backup configuration has an unsupported property (passwords are not allowed).'
    }
}
foreach ($field in $allowed) {
    if ($field -cnotin $fields -or $null -eq $config.$field -or
        [string]::IsNullOrWhiteSpace([string]$config.$field)) {
        throw "Backup configuration is missing a required field: $field"
    }
}
if ($config.PGHOST -cnotin @('localhost', '127.0.0.1', '::1') -or
    [string]$config.PGPORT -notmatch '^[0-9]{1,5}$' -or
    [int]$config.PGPORT -lt 1 -or [int]$config.PGPORT -gt 65535) {
    throw 'Backup PostgreSQL must be on loopback with a valid port.'
}
foreach ($field in @('PGUSER', 'PGDATABASE')) {
    if ([string]$config.$field -notmatch '^[a-zA-Z0-9_-]{1,64}$') {
        throw "Backup PostgreSQL field is invalid: $field"
    }
}
if ([string]$config.RetentionDays -notmatch '^[0-9]{1,4}$' -or
    [int]$config.RetentionDays -lt 7 -or [int]$config.RetentionDays -gt 3650) {
    throw 'Backup retention must be 7 to 3650 days.'
}
foreach ($field in @('PGPASSFILE', 'PGSSLROOTCERT', 'PostgresBin', 'BackupDir')) {
    if (-not [IO.Path]::IsPathRooted([string]$config.$field)) {
        throw "Backup path must be absolute: $field"
    }
}
if ([string]$config.ExternalBackupDir -notmatch '^\\\\[^\\]+\\[^\\]+(\\[^\\]+)*\\?$') {
    throw 'External backup destination must be a different machine on an SMB UNC share.'
}
if (-not (Test-Path -LiteralPath $config.PGPASSFILE -PathType Leaf) -or
    -not (Test-Path -LiteralPath $config.PGSSLROOTCERT -PathType Leaf)) {
    throw 'Backup requires existing password file and PostgreSQL TLS CA certificate.'
}
Assert-NoSharedReaders ([IO.Path]::GetFullPath([string]$config.PGPASSFILE)) 'PostgreSQL password file'
if ((Get-Item -LiteralPath $config.PGSSLROOTCERT).Length -eq 0 -or
    (Get-Item -LiteralPath $config.PGPASSFILE).Length -eq 0) {
    throw 'Backup password file or TLS CA certificate is empty.'
}
if (-not (Test-Path -LiteralPath $config.PostgresBin -PathType Container)) {
    throw 'PostgreSQL tools folder is unavailable.'
}
foreach ($exe in @('pg_dump.exe', 'pg_restore.exe', 'psql.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $config.PostgresBin $exe) -PathType Leaf)) {
        throw "PostgreSQL backup utility is missing: $exe"
    }
}
$local = [IO.Path]::GetFullPath([string]$config.BackupDir).TrimEnd('\', '/')
$root = [IO.Path]::GetPathRoot($local).TrimEnd('\', '/')
if ($local -eq $root -or $local -eq [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\', '/')) {
    throw 'Backup destination is an unsafe root or temporary directory.'
}
if ($CheckOnly) {
    Write-Output 'PASS: protected backup task configuration validated; no database, share or scheduler accessed.'
    return
}
$env:PGHOST = [string]$config.PGHOST
$env:PGPORT = [string]$config.PGPORT
$env:PGUSER = [string]$config.PGUSER
$env:PGDATABASE = [string]$config.PGDATABASE
$env:PGPASSFILE = [string]$config.PGPASSFILE
$env:PGSSLROOTCERT = [string]$config.PGSSLROOTCERT
$env:PGSSLMODE = 'verify-full'
$backupScript = Join-Path $PSScriptRoot 'postgres-daily-backup.ps1'
$checker = Join-Path $PSScriptRoot 'check-esterium-backup-integrity.ps1'
& $backupScript -BackupDir $local -ExternalBackupDir ([string]$config.ExternalBackupDir) -RequireExternalCopy -PostgresBin ([string]$config.PostgresBin) -RetentionDays ([int]$config.RetentionDays)
& $checker -BackupDir $local -ExternalBackupDir ([string]$config.ExternalBackupDir)
Write-Output 'PASS: scheduled local and off-machine backup integrity verified.'
