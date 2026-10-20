#requires -Version 5.1
<#
  Read-only Windows inventory gate for existing local PostgreSQL and
  Redis-compatible (e.g. Garnet) services. No TCP connections are opened.
  This is NOT a TLS, schema, Redis-atomicity or vendor/version certification.
#>
param(
    [Parameter(Mandatory = $true)][string]$PostgresServiceName,
    [Parameter(Mandatory = $true)][string]$RedisServiceName,
    [Parameter(Mandatory = $true)][string]$PostgresBin,
    [ValidateRange(1, 65535)][int]$PostgresPort = 5432,
    [ValidateRange(1, 65535)][int]$RedisPort = 6379,
    [switch]$RequireServiceOwnership
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'The local dependency inventory requires Windows.' }
foreach ($name in @($PostgresServiceName, $RedisServiceName)) {
    if ($name -cnotmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$') {
        throw 'Dependency Windows service names must contain only letters, digits, dot, underscore or hyphen.'
    }
    if ($name -eq 'EsteriumERP') {
        throw 'The application service cannot be used as a database dependency.'
    }
}
if ($PostgresServiceName -eq $RedisServiceName) {
    throw 'PostgreSQL and Redis-compatible backends must use distinct Windows services.'
}
if ($PostgresPort -eq $RedisPort) {
    throw 'PostgreSQL and Redis-compatible backends must use different ports.'
}
if (-not [IO.Path]::IsPathRooted($PostgresBin)) {
    throw 'PostgresBin must be an absolute directory.'
}
$toolsDir = [IO.Path]::GetFullPath($PostgresBin)
if (-not (Test-Path -LiteralPath $toolsDir -PathType Container)) {
    throw 'PostgreSQL tools directory is missing.'
}
$folder = Get-Item -LiteralPath $toolsDir -Force
if (($folder.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw 'PostgreSQL tools directory must not be a junction or symbolic link.'
}
foreach ($exe in @('postgres.exe', 'pg_dump.exe', 'pg_restore.exe', 'psql.exe')) {
    $path = Join-Path $toolsDir $exe
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "PostgreSQL tool is missing: $exe"
    }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.Length -le 0 -or
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "PostgreSQL tool is empty or a symbolic link: $exe"
    }
}
$servicePids = @{}
function Assert-RunningService([string]$name) {
    $service = @(Get-CimInstance -ClassName Win32_Service -Filter "Name='$name'" -ErrorAction Stop)
    if ($service.Count -ne 1) {
        throw 'A declared local dependency Windows service is missing or ambiguous.'
    }
    if ($service[0].State -ne 'Running') {
        throw 'A declared local dependency Windows service is not running.'
    }
    if ($RequireServiceOwnership) {
        $pidOfService = [int]$service[0].ProcessId
        if ($pidOfService -le 0) {
            throw 'Dependency service does not expose a running process for strict port ownership.'
        }
        $servicePids[$name] = $pidOfService
    }
    # Windows process command-line details might embed credentials: NEVER echo PathName.
}
function Assert-LoopbackListener([int]$port, [int]$expectedPid) {
    $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction Stop)
    if ($listeners.Count -eq 0) {
        throw "No listener is bound to required local dependency port $port."
    }
    foreach ($listener in $listeners) {
        if ([string]$listener.LocalAddress -notin @('127.0.0.1', '::1')) {
            throw "Local dependency port $port is exposed on a non-loopback address."
        }
        if ($RequireServiceOwnership -and [int]$listener.OwningProcess -ne $expectedPid) {
            throw "Dependency listener process does not belong to its declared Windows service."
        }
    }
}
Assert-RunningService $PostgresServiceName
Assert-RunningService $RedisServiceName
Assert-LoopbackListener $PostgresPort $(if ($RequireServiceOwnership) { $servicePids[$PostgresServiceName] } else { 0 })
Assert-LoopbackListener $RedisPort $(if ($RequireServiceOwnership) { $servicePids[$RedisServiceName] } else { 0 })
Write-Output 'PASS: declared PostgreSQL and Redis-compatible services are running with loopback-only TCP listeners.'
if ($RequireServiceOwnership) {
    Write-Output 'PASS: each listening port belongs to the matching Windows service process (strict ownership mode).'
}
Write-Output 'NOT CHECKED: PostgreSQL 16/version, data location, service ACLs, TLS trust, credentials, schema migrations, Redis atomic primitives or backups.'
Write-Output 'NEXT: run esterium-server.exe --check-config and --check-dependencies with protected production configuration.'
