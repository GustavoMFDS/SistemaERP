#requires -Version 5.1
<#
  Builds a non-secret backup configuration proposal only. No files, credentials,
  permissions, services, scheduled tasks or network connections are changed.
  Persist only through a separately approved protected-file provisioning flow.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PasswordFile,
    [Parameter(Mandatory=$true)][string]$RootCertificate,
    [Parameter(Mandatory=$true)][string]$ExternalBackupDir,
    [string]$PostgresBin = (Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [string]$BackupDir = (Join-Path $env:ProgramData 'Esterium\Backup\archives'),
    [ValidateRange(7,3650)][int]$RetentionDays = 14
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows backup configuration requires Windows.' }
function Get-LocalAbsolutePath([string]$Value) {
    if ($Value -notmatch '^[A-Za-z]:\\' -or $Value.Substring(2).Contains(':') -or
        $Value -match '[\x00-\x1F*?]' -or $Value -match '(?:^|\\)\.\.?(?:\\|$)') {
        throw 'A canonical absolute local drive path is required.'
    }
    $full = [IO.Path]::GetFullPath($Value)
    if ($full.TrimEnd('\') -eq [IO.Path]::GetPathRoot($full).TrimEnd('\')) {
        throw 'A drive root is not a valid backup input or destination.'
    }
    return $full
}
$pass = Get-LocalAbsolutePath $PasswordFile
$ca = Get-LocalAbsolutePath $RootCertificate
$bin = Get-LocalAbsolutePath $PostgresBin
$archive = Get-LocalAbsolutePath $BackupDir
if ($pass -eq $ca -or $pass.StartsWith($archive.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase) -or
    $ca.StartsWith($archive.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase) -or
    $pass -eq $archive -or $ca -eq $archive) {
    throw 'Credential and trust paths must be distinct and outside the archive directory.'
}
$program = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
if ($archive -eq $program -or $archive.StartsWith($program+'\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Archives cannot be stored under Program Files.'
}
if ($ExternalBackupDir -notmatch '^\\\\([A-Za-z0-9][A-Za-z0-9.-]*)\\([^\\/:*?"<>|\x00-\x1F]+)(\\[^\\/:*?"<>|\x00-\x1F]+)*\\?$') {
    throw 'An external SMB UNC path is required.'
}
$server = $Matches[1]
if ($server -in @('localhost','127.0.0.1','0.0.0.0',$env:COMPUTERNAME) -or
    $server.StartsWith('127.') -or $ExternalBackupDir -match '(?:^|\\)\.\.?(?:\\|$)') {
    throw 'External backup destination cannot be local or traverse directories.'
}
[ordered]@{
    PGHOST = 'localhost'
    PGPORT = '5432'
    PGUSER = 'esterium_backup'
    PGDATABASE = 'esterium'
    PGPASSFILE = $pass
    PGSSLROOTCERT = $ca
    PostgresBin = $bin
    BackupDir = $archive
    ExternalBackupDir = $ExternalBackupDir
    RetentionDays = $RetentionDays
} | ConvertTo-Json -Compress
