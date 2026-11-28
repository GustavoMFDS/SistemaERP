#requires -Version 5.1
<#
  Stage a new protected backup configuration using an existing approved pgpass.
  PLAN is read-only. Apply requires elevation and independently approved signed
  installed tools. Never reads/creates passwords, contacts SMB or enables tasks.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PasswordFile,
    [Parameter(Mandatory=$true)][string]$RootCertificate,
    [Parameter(Mandatory=$true)][string]$ExternalBackupDir,
    [string]$PostgresBin=(Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [string]$BackupDir=(Join-Path $env:ProgramData 'Esterium\Backup\archives'),
    [ValidateRange(7,3650)][int]$RetentionDays=14,
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows backup configuration requires Windows.' }
$base=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target=Join-Path $base 'BackupConfig'
$planner=Join-Path $PSScriptRoot 'plan-esterium-backup-config.ps1'
$runner=Join-Path $PSScriptRoot 'run-esterium-scheduled-backup.ps1'
$gate=Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
function Assert-PhysicalPath([string]$Path) {
    $cursor=$Path
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Backup configuration path must not traverse a link or junction.'
        }
        $next=Split-Path -Parent $cursor
        if (-not $next -or $next -eq $cursor) { break }
        $cursor=$next
    }
}
function Assert-RestrictedParent([string]$Path) {
    $acl=Get-Acl -LiteralPath $Path
    $owner=([Security.Principal.NTAccount]::new($acl.Owner)).Translate([Security.Principal.SecurityIdentifier]).Value
    if ($owner -notin @('S-1-5-18','S-1-5-32-544')) {
        throw 'Backup configuration parent must be owned by SYSTEM or Administrators.'
    }
    $writeMask=[Security.AccessControl.FileSystemRights]::Write -bor
        [Security.AccessControl.FileSystemRights]::Delete -bor
        [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
        [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
        [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach ($ace in $acl.Access) {
        if ($ace.AccessControlType -ne 'Allow') { continue }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if ($sid -notin @('S-1-5-18','S-1-5-32-544') -and
            ($ace.FileSystemRights -band $writeMask) -ne 0) {
            throw 'Unapproved identity can modify the backup configuration parent.'
        }
    }
}
if ($Apply) {
    if ($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
        $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$') {
        throw 'Apply requires independent release and publisher approval.'
    }
    $identity=[Security.Principal.WindowsIdentity]::GetCurrent()
    $principal=[Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Administrative elevation required.'
    }
    $installedTools=Join-Path ([IO.Path]::GetFullPath($InstallDir)) 'tools'
    if (-not [string]::Equals($PSScriptRoot,$installedTools,[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Apply must run from the installed tools directory.'
    }
    foreach ($script in @($PSCommandPath,$planner,$runner,$gate,(Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'))) {
        $sig=Get-AuthenticodeSignature -LiteralPath $script
        if ($sig.Status -ne 'Valid' -or $null -eq $sig.SignerCertificate -or
            $sig.SignerCertificate.Thumbprint -ne $TrustedPublisherThumbprint) {
            throw 'Apply requires all configuration helpers signed by the approved publisher.'
        }
    }
    & $gate -BundleDir $InstallDir -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
}
$proposal=& $planner -PasswordFile $PasswordFile -RootCertificate $RootCertificate -ExternalBackupDir $ExternalBackupDir -PostgresBin $PostgresBin -BackupDir $BackupDir -RetentionDays $RetentionDays
$config=$proposal | ConvertFrom-Json
if (Test-Path -LiteralPath $target) { throw 'Backup configuration directory already exists; refusing replacement.' }
foreach ($path in @($config.PGPASSFILE,$config.PGSSLROOTCERT,$config.BackupDir)) {
    if ($path -eq $target -or $path.StartsWith($target+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Backup inputs and archives must be outside the new configuration directory.'
    }
}
if (-not $Apply) {
    Write-Output 'PLAN: stage protected backup configuration using existing credential and CA paths.'
    Write-Output 'PLAN: no files, credentials, database, SMB or scheduled task changed.'
    return
}
Assert-PhysicalPath $base
Assert-RestrictedParent $base
Assert-PhysicalPath $config.PGPASSFILE
Assert-PhysicalPath $config.PGSSLROOTCERT
Assert-PhysicalPath $config.PostgresBin
$staging=Join-Path $base ('.esterium-backup-config-'+[Guid]::NewGuid().ToString('N'))
$published=$false
try {
    New-Item -ItemType Directory -Path $staging -ErrorAction Stop | Out-Null
    & icacls.exe $staging '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not protect backup configuration staging.' }
    $file=Join-Path $staging 'backup-config.json'
    $stream=[IO.File]::Open($file,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try {
        $bytes=[Text.UTF8Encoding]::new($false).GetBytes($proposal+[Environment]::NewLine)
        $stream.Write($bytes,0,$bytes.Length)
        $stream.Flush($true)
    } finally { $stream.Dispose() }
    & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-20:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not protect backup configuration file.' }
    & $runner -ConfigFile $file -CheckOnly | Out-Null
    [IO.Directory]::Move($staging,$target)
    $published=$true
    Write-Output 'PASS: protected backup configuration staged; database, credentials and task unchanged.'
} finally {
    if (-not $published -and (Test-Path -LiteralPath $staging)) {
        Remove-Item -LiteralPath $staging -Recurse -Force
    }
}
