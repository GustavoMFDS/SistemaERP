#requires -Version 5.1
<#
  Register a real initialized PostgreSQL ZIP-based Windows service, DISABLED
  AND STOPPED, as ephemeral non-admin account. No passwords on command line.
  GitHub-hosted disposable Windows CI ONLY. Never start the service.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$VerifiedPgRoot,
    [Parameter(Mandatory=$true)][string]$DataDir,
    [Parameter(Mandatory=$true)][pscredential]$ServiceCredential
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1') {
    throw 'Service registration test requires disposable GitHub-hosted Windows CI.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'CI registration requires Administrator.'
}
$temp=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\')+'\'
$pgDir=[IO.Path]::GetFullPath($VerifiedPgRoot).TrimEnd('\')
$data=[IO.Path]::GetFullPath($DataDir).TrimEnd('\')
if (-not $pgDir.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase) -or
    -not $data.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase) -or
    $pgDir.Contains('"') -or $data.Contains('"') -or
    -not (Test-Path -LiteralPath (Join-Path $pgDir 'bin\pg_ctl.exe') -PathType Leaf) -or
    -not (Test-Path -LiteralPath (Join-Path $data 'PG_VERSION') -PathType Leaf) -or
    ([IO.File]::ReadAllText((Join-Path $data 'PG_VERSION'))).Trim() -cne '16') {
    throw 'Registration requires pinned PG16 ZIP and fresh initialized cluster.'
}
if ($ServiceCredential.UserName -notmatch '^[A-Za-z0-9_-]+\\epg[0-9a-f]{12}$') {
    throw 'Unexpected account; refuse non-ephemeral service identity.'
}
$name='EsteriumCIPG'+[Guid]::NewGuid().ToString('N').Substring(0,12)
if ($null -ne (Get-Service -Name $name -ErrorAction SilentlyContinue)) {
    throw 'Selected CI service name already exists.'
}
$binary=Join-Path $pgDir 'bin\pg_ctl.exe'
$command='"'+$binary+'" runservice -N "'+$name+'" -D "'+$data+'" -w'
$created=$false
try {
    New-Service -Name $name -BinaryPathName $command -Credential $ServiceCredential -StartupType Disabled -DisplayName ('Esterium disposable PG16 '+$name) -ErrorAction Stop | Out-Null
    $created=$true
    $entry=@(Get-CimInstance Win32_Service -Filter ("Name='"+$name+"'") -ErrorAction Stop)
    if ($entry.Count -ne 1) { throw 'PostgreSQL service missing from SCM.' }
    $identity=$ServiceCredential.UserName
    $expectedLocal='.\'+($identity.Split('\')[-1])
    if ($entry[0].StartMode -cne 'Disabled' -or $entry[0].State -cne 'Stopped' -or
        [int]$entry[0].ProcessId -ne 0 -or
        $entry[0].StartName -notin @($identity,$expectedLocal) -or
        $entry[0].PathName -cne $command) {
        throw 'SCM registration has unexpected identity, executable or state.'
    }
    Write-Output 'PASS: PostgreSQL 16 Windows service registered for isolated NON-ADMIN CI identity.'
    Write-Output 'PASS: SCM binary from verified ZIP, service disabled/stopped, no process started.'
} finally {
    if ($created) {
        & sc.exe delete $name | Out-Null
        if ($LASTEXITCODE -ne 0) {throw 'Ephemeral CI service cannot be deregistered.'}
        for ($attempt=0;$attempt -lt 10;$attempt++) {
            if ($null -eq (Get-Service -Name $name -ErrorAction SilentlyContinue)) {break}
            Start-Sleep -Milliseconds 500
        }
        if ($null -ne (Get-Service -Name $name -ErrorAction SilentlyContinue)) {
            throw 'Ephemeral CI service persisted after cleanup.'
        }
        Write-Output 'PASS: CI test service deregistered; account cleanup remains with initdb test.'
    }
}
