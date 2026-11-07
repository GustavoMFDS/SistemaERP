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
    [ValidateRange(0, 60)][int]$ListenerReadyTimeoutSeconds = 0,
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
$serviceOwnerSids = @{}
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
        if ($name -eq $PostgresServiceName) {
            $serviceProcess = @(Get-CimInstance -ClassName Win32_Process -Filter "ProcessId=$pidOfService" -ErrorAction Stop)
            if ($serviceProcess.Count -ne 1) {
                throw 'PostgreSQL service process cannot be verified for strict port ownership.'
            }
            $serviceOwner = Invoke-CimMethod -InputObject $serviceProcess[0] -MethodName GetOwnerSid -ErrorAction Stop
            if ($serviceOwner.ReturnValue -ne 0 -or [string]::IsNullOrWhiteSpace($serviceOwner.Sid)) {
                throw 'PostgreSQL service account cannot be verified for strict port ownership.'
            }
            $serviceOwnerSids[$name] = [string]$serviceOwner.Sid
        }
    }
    # Windows process command-line details might embed credentials: NEVER echo PathName.
}
# A service wrapper (NSSM) holds the SCM PID, while the wrapped Garnet
# listener is a child. Permit only a bounded process ancestry under NETWORK
# SERVICE; do not weaken PostgreSQL's direct owner check.
function Assert-GarnetServiceDescendant([int]$listenerPid, [int]$servicePid) {
    $currentPid = $listenerPid
    $visited = @{}
    for ($depth = 0; $depth -lt 6; $depth++) {
        if ($currentPid -eq $servicePid) { return }
        if ($currentPid -le 0 -or $visited.ContainsKey($currentPid)) {
            throw 'Garnet listener process ancestry is invalid.'
        }
        $visited[$currentPid] = $true
        $processes = @(Get-CimInstance -ClassName Win32_Process -Filter "ProcessId=$currentPid" -ErrorAction Stop)
        if ($processes.Count -ne 1) { throw 'Garnet listener process is missing.' }
        $owner = Invoke-CimMethod -InputObject $processes[0] -MethodName GetOwnerSid -ErrorAction Stop
        if ($owner.ReturnValue -ne 0 -or $owner.Sid -ne 'S-1-5-20') {
            throw 'Garnet listener must run under NETWORK SERVICE.'
        }
        $currentPid = [int]$processes[0].ParentProcessId
    }
    throw 'Garnet listener is not descended from its Windows service.'
}
function Assert-PostgresServiceDescendant([int]$listenerPid, [int]$servicePid) {
    $expectedSid = [string]$serviceOwnerSids[$PostgresServiceName]
    if ([string]::IsNullOrWhiteSpace($expectedSid)) {
        throw 'PostgreSQL service account is unavailable for strict port ownership.'
    }
    $currentPid = $listenerPid
    $visited = @{}
    for ($depth = 0; $depth -lt 6; $depth++) {
        if ($currentPid -eq $servicePid) { return }
        if ($currentPid -le 0 -or $visited.ContainsKey($currentPid)) {
            throw 'PostgreSQL listener process ancestry is invalid.'
        }
        $visited[$currentPid] = $true
        $processes = @(Get-CimInstance -ClassName Win32_Process -Filter "ProcessId=$currentPid" -ErrorAction Stop)
        if ($processes.Count -ne 1) { throw 'PostgreSQL listener process is missing.' }
        if ($currentPid -eq $listenerPid -and $processes[0].Name -cne 'postgres.exe') {
            throw 'PostgreSQL port is not owned by postgres.exe.'
        }
        $owner = Invoke-CimMethod -InputObject $processes[0] -MethodName GetOwnerSid -ErrorAction Stop
        if ($owner.ReturnValue -ne 0 -or $owner.Sid -cne $expectedSid) {
            throw 'PostgreSQL listener must run under the declared service account.'
        }
        $currentPid = [int]$processes[0].ParentProcessId
    }
    throw 'PostgreSQL listener is not descended from its Windows service.'
}
function Assert-LoopbackListener([int]$port, [int]$expectedPid) {
    # A service can report Running just before binding its socket. Only an
    # absent listener is retried; an exposed or incorrectly owned listener
    # still fails immediately below.
    $deadline = [DateTime]::UtcNow.AddSeconds($ListenerReadyTimeoutSeconds)
    do {
        $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction Stop)
        if ($listeners.Count -gt 0) { break }
        if ([DateTime]::UtcNow -ge $deadline) {
            throw "No listener is bound to required local dependency port $port."
        }
        Start-Sleep -Milliseconds 500
    } while ($true)
    foreach ($listener in $listeners) {
        if ([string]$listener.LocalAddress -notin @('127.0.0.1', '::1')) {
            throw "Local dependency port $port is exposed on a non-loopback address."
        }
        if ($RequireServiceOwnership -and [int]$listener.OwningProcess -ne $expectedPid) {
            if ($port -eq $RedisPort) {
                Assert-GarnetServiceDescendant ([int]$listener.OwningProcess) $expectedPid
            } elseif ($port -eq $PostgresPort) {
                # EDB can keep pg_ctl.exe as SCM owner while postgres.exe
                # child process owns the socket. Require bounded ancestry and
                # the exact service account for every process in that chain.
                Assert-PostgresServiceDescendant ([int]$listener.OwningProcess) $expectedPid
            } else {
                throw 'Dependency listener cannot be tied to its declared service.'
            }
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
