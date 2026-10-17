#requires -Version 5.1
<#
  Register a daily Windows backup task, DISABLED until physical pilot signoff.
  -Apply requires elevation; default is an observation-only plan.
  No passwords or database URLs are written to the task action/registry.
#>
param(
    [Parameter(Mandatory = $true)][string]$ConfigFile,
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$DailyAt = '02:00',
    [switch]$Apply
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Scheduled backup registration requires Windows.' }
if ($DailyAt -notmatch '^([01][0-9]|2[0-3]):[0-5][0-9]$') {
    throw 'DailyAt must use a 24-hour HH:mm value.'
}
if (-not [IO.Path]::IsPathRooted($ConfigFile) -or
    -not (Test-Path -LiteralPath $ConfigFile -PathType Leaf)) {
    throw 'Backup configuration must be an existing absolute file.'
}
$configPath = [IO.Path]::GetFullPath($ConfigFile)
$runnerForCheck = Join-Path $PSScriptRoot 'run-esterium-scheduled-backup.ps1'
& $runnerForCheck -ConfigFile $configPath -CheckOnly | Out-Null
$installPath = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\', '/')
$programFilesRoot = [IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\', '/')
if (-not $installPath.StartsWith($programFilesRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'InstallDir must be a dedicated subdirectory of Program Files.'
}
if ($configPath.StartsWith($installPath + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Backup configuration must be outside the application installation.'
}
$taskName = 'EsteriumERP-Backup'
$task = @(Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)
if ($task.Count -gt 0) { throw 'Esterium backup task already exists; refusing to replace it.' }
if (-not $Apply) {
    Write-Output 'PLAN: a daily disabled backup task can be registered after setup approval.'
    Write-Output 'PLAN: task principal NetworkService; external SMB share must authorize the PC machine account.'
    Write-Output 'PLAN: only protected config file path is passed; no passwords or database URLs in scheduler.'
    Write-Output 'PLAN: no task, data folder, network connection, database update or backup was created.'
    return
}
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($current)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrative elevation is required to register the backup task.'
}
if (-not (Test-Path -LiteralPath $installPath -PathType Container)) {
    throw 'Esterium installation is missing.'
}
# Validate the whole installed bundle, including the protected task scripts.
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $installPath | Out-Null
$installedRunner = Join-Path $installPath 'tools\run-esterium-scheduled-backup.ps1'
if (-not (Test-Path -LiteralPath $installedRunner -PathType Leaf)) {
    throw 'Installed backup runner is missing.'
}
$minutes = [int]$DailyAt.Substring(0,2) * 60 + [int]$DailyAt.Substring(3,2)
$at = [DateTime]::Today.AddMinutes($minutes)
$executable = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
# Windows filenames cannot contain double quotes; both paths are absolute.
$arguments = '-NoProfile -NonInteractive -ExecutionPolicy RemoteSigned -File "' +
    $installedRunner + '" -ConfigFile "' + $configPath + '"'
$action = New-ScheduledTaskAction -Execute $executable -Argument $arguments
$trigger = New-ScheduledTaskTrigger -Daily -At $at
$servicePrincipal = New-ScheduledTaskPrincipal -UserId 'NT AUTHORITY\NETWORK SERVICE' -LogonType ServiceAccount -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -Disable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Hours 3) -StartWhenAvailable
$definition = New-ScheduledTask -Action $action -Trigger $trigger -Principal $servicePrincipal -Settings $settings
Register-ScheduledTask -TaskName $taskName -InputObject $definition -ErrorAction Stop | Out-Null
$verified = Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
if ($verified.State -ne 'Disabled') {
    Disable-ScheduledTask -TaskName $taskName -ErrorAction Stop | Out-Null
    throw 'Backup task unexpectedly started enabled; disabled immediately.'
}
Write-Output 'PASS: backup task registered DISABLED as NetworkService with config path only.'
Write-Output 'A technician must test the remote share and a restore on another PC before manually enabling it.'
