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
        # Even write-only rights could alter backups or secrets before the
        # scheduled job reads them, so inspect every Allow ACE, not just reads.
        try { $sid = $ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
        catch { throw "Cannot inspect all ACL readers for $label." }
        # A blocklist of Everyone/Users/Authenticated Users is insufficient:
        # a domain group or another local account may also read or replace
        # protected backup credentials. Fail closed for ANY unrelated SID.
        if ($sid -notin @('S-1-5-18', 'S-1-5-20', 'S-1-5-32-544')) {
            throw "Backup $label grants rights to an unapproved Windows identity."
        }
        if ($sid -eq 'S-1-5-20') {
            # This service must be able to read the configuration/password
            # but not rewrite its own connection authority or trust material.
            $unsafeWrites = [Security.AccessControl.FileSystemRights]::WriteData -bor
                [Security.AccessControl.FileSystemRights]::AppendData -bor
                [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
                [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
                [Security.AccessControl.FileSystemRights]::Delete -bor
                [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
                [Security.AccessControl.FileSystemRights]::TakeOwnership
            if (($ace.FileSystemRights -band $unsafeWrites) -ne 0) {
                throw "NetworkService must have read-only ACL access to backup $label."
            }
            if ($canRead) { $networkServiceCanRead = $true }
        }
    }
    if (-not $networkServiceCanRead) {
        throw "NetworkService needs an explicit file read ACL for backup $label."
    }
}
function Assert-ProtectedTLSRoot([string]$path) {
    # The PostgreSQL root CA is public information, so shared READ access
    # is acceptable; unauthorized WRITE/DELETE/ACL changes are not. Otherwise
    # verify-full could accept a replaced trust anchor.
    $file = Get-Item -LiteralPath $path -Force
    if (($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'PostgreSQL root CA must not be a symbolic link.'
    }
    $acl = Get-Acl -LiteralPath $path
    $writes = [Security.AccessControl.FileSystemRights]::WriteData -bor
        [Security.AccessControl.FileSystemRights]::AppendData -bor
        [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
        [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
        [Security.AccessControl.FileSystemRights]::Delete -bor
        [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
        [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach ($ace in $acl.Access) {
        if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        try { $sid = $ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value }
        catch { throw 'Cannot inspect PostgreSQL root CA ACL identity.' }
        if ($sid -notin @('S-1-5-18', 'S-1-5-32-544') -and
            ($ace.FileSystemRights -band $writes) -ne 0) {
            throw 'PostgreSQL root CA may not be modified by unapproved Windows identities.'
        }
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
# The installed local ERP has a fixed database and dedicated backup identity.
# Never allow a valid-looking config to select the application or admin role.
if ([string]$config.PGUSER -cne 'esterium_backup' -or
    [string]$config.PGDATABASE -cne 'esterium') {
    throw 'Backup requires the dedicated esterium_backup identity and esterium database.'
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
# A syntactically valid UNC path can still refer to the same PC.
$shareHost = [regex]::Match([string]$config.ExternalBackupDir, '^\\\\([^\\]+)\\').Groups[1].Value
if ($shareHost -in @('.', 'localhost', '127.0.0.1', '::1', $env:COMPUTERNAME)) {
    throw 'External backup share cannot point to the local computer.'
}
if (-not (Test-Path -LiteralPath $config.PGPASSFILE -PathType Leaf) -or
    -not (Test-Path -LiteralPath $config.PGSSLROOTCERT -PathType Leaf)) {
    throw 'Backup requires existing password file and PostgreSQL TLS CA certificate.'
}
Assert-NoSharedReaders ([IO.Path]::GetFullPath([string]$config.PGPASSFILE)) 'PostgreSQL password file'
Assert-ProtectedTLSRoot ([IO.Path]::GetFullPath([string]$config.PGSSLROOTCERT))
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
$programDir = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if ($local.StartsWith($programDir + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Backups cannot be stored inside Program Files or the application directory.'
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
# Libpq environment settings inherited from a user/service must not override
# the approved password file, hostname, or session role.
foreach ($override in @('PGPASSWORD','PGSERVICE','PGSERVICEFILE','PGHOSTADDR','PGOPTIONS','PGSSLCERT','PGSSLKEY','PGSSLCERTMODE','PGGSSENCMODE','PGREQUIREAUTH','PGSSLSNI','PGSSLNEGOTIATION','PGCHANNELBINDING','PGTARGETSESSIONATTRS')) {
    Remove-Item -LiteralPath ('Env:'+$override) -ErrorAction SilentlyContinue
}
$env:PGCONNECT_TIMEOUT = '10'
$psql = Join-Path ([string]$config.PostgresBin) 'psql.exe'
$identityCheck = & $psql -X -w -v ON_ERROR_STOP=1 -At -c @'
SELECT CASE WHEN
    current_database() = 'esterium'
    AND current_user = 'esterium_backup'
    AND session_user = current_user
    AND current_setting('default_transaction_read_only') = 'on'
    AND EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = current_user
          AND rolcanlogin AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit)
    )
    AND NOT EXISTS (
        SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member
        WHERE r.rolname=current_user
    )
    AND EXISTS (SELECT 1 FROM pg_stat_ssl WHERE pid=pg_backend_pid() AND ssl)
    AND NOT has_database_privilege(current_user,current_database(),'CREATE')
    AND NOT has_schema_privilege(current_user,'public','CREATE')
    AND NOT EXISTS (
        SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','f')
          AND (c.relrowsecurity OR NOT has_table_privilege(current_user,c.oid,'SELECT')
               OR has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
               OR has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES'))
    )
    AND NOT EXISTS (
        SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='public' AND c.relkind='S'
          AND (NOT has_sequence_privilege(current_user,c.oid,'SELECT')
               OR has_sequence_privilege(current_user,c.oid,'USAGE,UPDATE'))
    )
    AND NOT EXISTS (
        SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
        WHERE n.nspname='public' AND p.prosecdef
          AND has_function_privilege(current_user,p.oid,'EXECUTE')
    )
THEN 'PASS' ELSE 'REFUSED' END;
'@
if ($LASTEXITCODE -ne 0 -or [string]$identityCheck -cne 'PASS') {
    throw 'Backup identity, TLS or effective privileges refused; no dump attempted.'
}
$backupScript = Join-Path $PSScriptRoot 'postgres-daily-backup.ps1'
$checker = Join-Path $PSScriptRoot 'check-esterium-backup-integrity.ps1'
& $backupScript -BackupDir $local -ExternalBackupDir ([string]$config.ExternalBackupDir) -RequireExternalCopy -PostgresBin ([string]$config.PostgresBin) -RetentionDays ([int]$config.RetentionDays)
& $checker -BackupDir $local -ExternalBackupDir ([string]$config.ExternalBackupDir)
Write-Output 'PASS: scheduled local and off-machine backup integrity verified.'
