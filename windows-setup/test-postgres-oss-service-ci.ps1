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
    [Parameter(Mandatory=$true)][pscredential]$ServiceCredential,
    [switch]$ExerciseLoopback
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
$started=$false
$port=0
$oldPgPassword=$env:PGPASSWORD
if ($ExerciseLoopback) {
    # Select an ephemeral loopback-only port; PostgreSQL config is confined to
    # a verified CI-only test cluster, never the store's data directory.
    $portProbe=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
    try {
        $portProbe.Start()
        $port=[int]$portProbe.LocalEndpoint.Port
    } finally { $portProbe.Stop() }
    if ($port -lt 1024) {throw 'Unexpected PostgreSQL test port.'}
    $conf=Join-Path $data 'postgresql.conf'
    if (-not (Test-Path -LiteralPath $conf -PathType Leaf)) {throw 'Missing PostgreSQL config.'}
    Add-Content -LiteralPath $conf -Value ("listen_addresses = '127.0.0.1'"+[Environment]::NewLine+"port = "+$port+[Environment]::NewLine) -Encoding Ascii
}
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
    if ($ExerciseLoopback) {
        Set-Service -Name $name -StartupType Manual -ErrorAction Stop
        Start-Service -Name $name -ErrorAction Stop
        $started=$true
        $svc=Get-Service -Name $name -ErrorAction Stop
        $svc.WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(50))
        $health=@(& (Join-Path $pgDir 'bin\pg_isready.exe') '-h' '127.0.0.1' '-p' ([string]$port) '-d' 'postgres' 2>&1)
        if ($LASTEXITCODE -ne 0) {throw 'Windows PostgreSQL service failed loopback readiness probe.'}
        # The same CSPRNG secret is supplied to initdb's postgres superuser
        # through a restricted pwfile. Supply it to psql via environment only.
        # Neither command-line arguments nor CI logs contain the secret.
        $env:PGPASSWORD=$ServiceCredential.GetNetworkCredential().Password
        $sql=@(& (Join-Path $pgDir 'bin\psql.exe') '-X' '-w' '-A' '-t' '-h' '127.0.0.1' '-p' ([string]$port) '-U' 'postgres' '-d' 'postgres' '-c' 'SELECT 1' 2>&1)
        if ($LASTEXITCODE -ne 0 -or ($sql -join '').Trim() -cne '1') {
            throw 'Authenticated local SQL query failed in disposable PostgreSQL service.'
        }
        $env:PGPASSWORD=$oldPgPassword
        Write-Output 'PASS: isolated PostgreSQL service started on 127.0.0.1 and answered authenticated SQL SELECT 1.'
        Stop-Service -Name $name -ErrorAction Stop
        (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(50))
        $started=$false
        Set-Service -Name $name -StartupType Disabled -ErrorAction Stop
        if (@(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -ne 0) {
            throw 'PostgreSQL loopback listener remained after controlled stop.'
        }
        Write-Output 'PASS: PostgreSQL service safely stopped/disabled; loopback port no longer listening.'
    }
    Write-Output 'PASS: PostgreSQL 16 Windows service registered for isolated NON-ADMIN CI identity.'
    Write-Output 'PASS: SCM binary from verified ZIP, service disabled/stopped, no process started.'
} finally {
    $env:PGPASSWORD=$oldPgPassword
    if ($created) {
        try {
            $existing=Get-Service -Name $name -ErrorAction SilentlyContinue
            if ($null -ne $existing -and $existing.Status -ne 'Stopped') {
                Stop-Service -Name $name -Force -ErrorAction Stop
                (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(50))
            }
            Set-Service -Name $name -StartupType Disabled -ErrorAction Stop
        } catch {
            throw 'Disposable PostgreSQL service could not be stopped/disabled; do not reuse runner.'
        }
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
