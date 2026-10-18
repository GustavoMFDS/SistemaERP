#requires -Version 5.1
<#
  Conservative service-only removal for the technical Windows pilot.
  Never deletes installation binaries, PostgreSQL, Redis, secrets, tax
  materials, fiscal documents, backups or scheduled tasks.
  Default mode is read-only; -Apply is explicit and requires administrator.
#>
param(
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [switch]$Apply
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'The Esterium service removal tool requires Windows.' }
if (-not [IO.Path]::IsPathRooted($InstallDir)) { throw 'InstallDir must be absolute.' }
$installPath = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\', '/')
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if (-not $installPath.StartsWith($programFiles + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'InstallDir must be under a dedicated Program Files subdirectory.'
}
$expectedBinary = Join-Path $installPath 'esterium-server.exe'
$service = Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop
if ($null -eq $service) {
    throw 'EsteriumERP service does not exist; there is nothing to unregister.'
}
# Never delete an unrelated service just because it has a similar display name.
# Registered command has no extra CLI arguments; reject any changed ImagePath.
$actualBinary = ([string]$service.PathName).Trim().Trim('"')
if (-not [string]::Equals($actualBinary, $expectedBinary,
        [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Service executable differs from this exact Esterium installation. Refusing deletion.'
}
if ([string]$service.StartName -notmatch '(?i)(^|\\)Local ?Service$') {
    throw 'Service identity is not LocalService; refusing automated deletion.'
}
if ([string]$service.State -ne 'Stopped') {
    throw 'Stop sales, reconcile the shop and stop the service manually before unregistering.'
}
$backupTask = Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction SilentlyContinue
if ($null -ne $backupTask -and $backupTask.State -ne 'Disabled') {
    throw 'Backup task is still active; disable and review it before service removal.'
}
if (-not $Apply) {
    Write-Output 'PLAN: eligible stopped EsteriumERP service found; no changes made.'
    Write-Output 'PLAN: -Apply removes ONLY its SCM registration after administrator approval.'
    Write-Output 'PLAN: binaries, migrations, PostgreSQL/Redis, task, backups and secrets remain untouched.'
    return
}
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($current)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrative elevation is required to unregister the service.'
}
$sc = Join-Path $env:SystemRoot 'System32\sc.exe'
$result = & $sc delete EsteriumERP 2>&1
if ($LASTEXITCODE -ne 0) {
    throw "SCM failed to unregister EsteriumERP: $result"
}
$global:LASTEXITCODE = 0
Write-Output 'PASS: SCM accepted service deregistration only; deletion may complete asynchronously.'
Write-Output 'Data, backups, SQL, configuration, backup task and program files were not deleted.'
