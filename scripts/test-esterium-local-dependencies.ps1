#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Synthetic Windows dependency inventory test requires disposable hosted CI.'
}
$root = Join-Path $env:RUNNER_TEMP ('esterium-deps-' + [Guid]::NewGuid().ToString('N'))
$toolsDir = Join-Path $root 'pg-bin'
$checker = Join-Path $PSScriptRoot 'check-esterium-local-dependencies.ps1'
# Override only read-only Windows inventory commands in this script session.
# No real Windows service is installed, started or contacted in this test.
$global:ESTERIUM_TEST_PG_RUNNING = $true
$global:ESTERIUM_TEST_PG_WRAPPED = $false
$global:ESTERIUM_TEST_PG_WRONG_OWNER = $false
$global:ESTERIUM_TEST_PG_WRONG_PARENT = $false
$global:ESTERIUM_TEST_REDIS_RUNNING = $true
$global:ESTERIUM_TEST_LAN_BIND = $false
$global:ESTERIUM_TEST_PG_MISSING = $false
$global:ESTERIUM_TEST_PORT_OWNER_MISMATCH = $false
$global:ESTERIUM_TEST_PG_PID_MISSING = $false
$global:ESTERIUM_TEST_GARNET_WRAPPED = $false
$global:ESTERIUM_TEST_GARNET_WRONG_OWNER = $false
$global:ESTERIUM_TEST_GARNET_WRONG_PARENT = $false
$global:ESTERIUM_TEST_LISTENER_DELAY_CALLS = 0
$global:ESTERIUM_TEST_LISTENER_NEVER_PORT = 0
$global:ESTERIUM_TEST_LISTENER_READS = @{}
function Get-CimInstance {
    [CmdletBinding()]
    param([string]$ClassName, [string]$Filter)
    if ($ClassName -eq 'Win32_Process') {
        switch ($Filter) {
            'ProcessId=28101' { return [PSCustomObject]@{ ProcessId=28101; Name='pg_ctl.exe'; ParentProcessId=4 } }
            'ProcessId=28104' {
                return [PSCustomObject]@{
                    ProcessId=28104
                    Name='postgres.exe'
                    ParentProcessId=$(if ($global:ESTERIUM_TEST_PG_WRONG_PARENT) { 28199 } else { 28101 })
                }
            }
            'ProcessId=28103' {
                return [PSCustomObject]@{
                    ProcessId=28103
                    Name='Garnet.exe'
                    ParentProcessId=$(if ($global:ESTERIUM_TEST_GARNET_WRONG_PARENT) { 28199 } else { 28102 })
                }
            }
            default {
                if ($global:ESTERIUM_TEST_PG_WRONG_PARENT -and $Filter -match '^ProcessId=(2819[4-9])$') {
                    $processId=[int]($Filter -replace '^ProcessId=','')
                    return [PSCustomObject]@{ ProcessId=$processId; Name='postgres.exe'; ParentProcessId=($processId-1) }
                }
                return
            }
        }
    }
    if ($ClassName -ne 'Win32_Service') { throw 'Unexpected CIM request.' }
    if ($Filter -eq "Name='EsteriumCI-Postgres'") {
        if ($global:ESTERIUM_TEST_PG_MISSING) { return }
        return [PSCustomObject]@{
            Name='EsteriumCI-Postgres'
            State=$(if ($global:ESTERIUM_TEST_PG_RUNNING) { 'Running' } else { 'Stopped' })
            ProcessId=$(if ($global:ESTERIUM_TEST_PG_PID_MISSING) { 0 } else { 28101 })
        }
    }
    if ($Filter -eq "Name='EsteriumCI-Redis'") {
        return [PSCustomObject]@{
            Name='EsteriumCI-Redis'
            State=$(if ($global:ESTERIUM_TEST_REDIS_RUNNING) { 'Running' } else { 'Stopped' })
            ProcessId=28102
        }
    }
    throw 'Unexpected Windows service requested.'
}
function Invoke-CimMethod {
    [CmdletBinding()]
    param($InputObject, [string]$MethodName)
    if ($MethodName -ne 'GetOwnerSid') { throw 'Unexpected process owner query.' }
    $sid='S-1-5-20'
    if ($InputObject.Name -in @('pg_ctl.exe','postgres.exe')) {
        $sid=if ($global:ESTERIUM_TEST_PG_WRONG_OWNER -and $InputObject.Name -eq 'postgres.exe') {
            'S-1-5-21-999'
        } else { 'S-1-5-21-42' }
    } elseif ($global:ESTERIUM_TEST_GARNET_WRONG_OWNER) {
        $sid='S-1-5-21-123'
    }
    return [PSCustomObject]@{ ReturnValue=0; Sid=$sid }
}
function Get-NetTCPConnection {
    [CmdletBinding()]
    param([string]$State, [int]$LocalPort)
    if ($State -ne 'Listen' -or $LocalPort -notin @(5432, 6379)) {
        throw 'Unexpected TCP inventory request.'
    }
    if ($global:ESTERIUM_TEST_LISTENER_NEVER_PORT -eq $LocalPort) { return }
    if ($global:ESTERIUM_TEST_LISTENER_DELAY_CALLS -gt 0) {
        $key=[string]$LocalPort
        if (-not $global:ESTERIUM_TEST_LISTENER_READS.ContainsKey($key)) {
            $global:ESTERIUM_TEST_LISTENER_READS[$key]=0
        }
        $global:ESTERIUM_TEST_LISTENER_READS[$key]++
        if ($global:ESTERIUM_TEST_LISTENER_READS[$key] -le $global:ESTERIUM_TEST_LISTENER_DELAY_CALLS) { return }
    }
    $pidValue = if ($LocalPort -eq 5432) {
        if ($global:ESTERIUM_TEST_PG_WRAPPED) { 28104 } else { 28101 }
    } else { 28102 }
    if ($global:ESTERIUM_TEST_PORT_OWNER_MISMATCH -and $LocalPort -eq 6379) { $pidValue = 28199 }
    if ($global:ESTERIUM_TEST_GARNET_WRAPPED -and $LocalPort -eq 6379) { $pidValue = 28103 }
    if ($global:ESTERIUM_TEST_LAN_BIND -and $LocalPort -eq 6379) {
        return [PSCustomObject]@{ LocalAddress='0.0.0.0'; LocalPort=$LocalPort; OwningProcess=$pidValue }
    }
    return [PSCustomObject]@{ LocalAddress='127.0.0.1'; LocalPort=$LocalPort; OwningProcess=$pidValue }
}
try {
    [IO.Directory]::CreateDirectory($toolsDir) | Out-Null
    foreach ($binary in @('postgres.exe','pg_dump.exe','pg_restore.exe','psql.exe')) {
        [IO.File]::WriteAllText((Join-Path $toolsDir $binary), 'synthetic bytes not executed')
    }
    $inventoryParameters = @{
        PostgresServiceName='EsteriumCI-Postgres'
        RedisServiceName='EsteriumCI-Redis'
        PostgresBin=$toolsDir
    }
    $ok = @(& $checker @inventoryParameters) -join ' '
    if ($ok -notmatch 'PASS: declared PostgreSQL') {
        throw 'Synthetic healthy local services were not recognized.'
    }
    function Assert-Reject([string]$reason, [string]$message) {
        $rejected = $false
        $actualError = '(no exception)'
        try { & $checker @inventoryParameters | Out-Null }
        catch {
            $actualError = $_.Exception.Message
            $rejected = $actualError -match $message
        }
        if (-not $rejected) { throw "Inventory unexpectedly accepted $reason, or failed for another reason: $actualError" }
    }
    $inventoryParameters.RequireServiceOwnership = $true
    $inventoryParameters.ListenerReadyTimeoutSeconds = 2
    $global:ESTERIUM_TEST_LISTENER_DELAY_CALLS = 1
    $global:ESTERIUM_TEST_LISTENER_READS = @{}
    $strictResult = @(& $checker @inventoryParameters) -join ' '
    if ($strictResult -notmatch 'strict ownership mode' -or
        $global:ESTERIUM_TEST_LISTENER_READS['5432'] -lt 2 -or
        $global:ESTERIUM_TEST_LISTENER_READS['6379'] -lt 2) {
        throw 'Synthetic strict dependency listener ownership was not checked.'
    }
    $global:ESTERIUM_TEST_LISTENER_DELAY_CALLS = 0
    $global:ESTERIUM_TEST_LISTENER_READS = @{}
    $inventoryParameters.Remove('ListenerReadyTimeoutSeconds')
    $global:ESTERIUM_TEST_PG_WRAPPED = $true
    $wrappedPostgres = @(& $checker @inventoryParameters) -join ' '
    if ($wrappedPostgres -notmatch 'strict ownership mode') {
        throw 'PostgreSQL listener child under the declared service account was rejected.'
    }
    $global:ESTERIUM_TEST_PG_WRONG_OWNER = $true
    Assert-Reject 'PostgreSQL listener under a different Windows account' 'declared service account'
    $global:ESTERIUM_TEST_PG_WRONG_OWNER = $false
    $global:ESTERIUM_TEST_PG_WRONG_PARENT = $true
    Assert-Reject 'PostgreSQL listener outside the Windows service ancestry' 'not descended'
    $global:ESTERIUM_TEST_PG_WRONG_PARENT = $false
    $global:ESTERIUM_TEST_PG_WRAPPED = $false
    $global:ESTERIUM_TEST_GARNET_WRAPPED = $true
    $wrapped = @(& $checker @inventoryParameters) -join ' '
    if ($wrapped -notmatch 'strict ownership mode') {
        throw 'Restricted Garnet process child was not accepted.'
    }
    $global:ESTERIUM_TEST_GARNET_WRONG_OWNER = $true
    Assert-Reject 'Garnet child in an unrelated OS identity' 'NETWORK SERVICE'
    $global:ESTERIUM_TEST_GARNET_WRONG_OWNER = $false
    $global:ESTERIUM_TEST_GARNET_WRONG_PARENT = $true
    Assert-Reject 'Garnet child outside the service ancestry' 'Garnet listener process is missing'
    $global:ESTERIUM_TEST_GARNET_WRONG_PARENT = $false
    $global:ESTERIUM_TEST_GARNET_WRAPPED = $false
    $global:ESTERIUM_TEST_PORT_OWNER_MISMATCH = $true
    Assert-Reject 'a different executable listening on the Redis port' 'Garnet listener process is missing'
    $global:ESTERIUM_TEST_PORT_OWNER_MISMATCH = $false
    $inventoryParameters.ListenerReadyTimeoutSeconds = 1
    $global:ESTERIUM_TEST_LISTENER_NEVER_PORT = 5432
    Assert-Reject 'a service that never opens its database listener' 'No listener is bound to required local dependency port 5432'
    $global:ESTERIUM_TEST_LISTENER_NEVER_PORT = 0
    $inventoryParameters.Remove('ListenerReadyTimeoutSeconds')
    $global:ESTERIUM_TEST_PG_PID_MISSING = $true
    Assert-Reject 'a service without a live owning process' 'does not expose a running process'
    $global:ESTERIUM_TEST_PG_PID_MISSING = $false
    $global:ESTERIUM_TEST_LAN_BIND = $true
    Assert-Reject 'Redis bound on all interfaces' 'non-loopback address'
    $global:ESTERIUM_TEST_LAN_BIND = $false
    $global:ESTERIUM_TEST_PG_RUNNING = $false
    Assert-Reject 'a stopped PostgreSQL service' 'not running'
    $global:ESTERIUM_TEST_PG_RUNNING = $true
    $global:ESTERIUM_TEST_PG_MISSING = $true
    Assert-Reject 'a missing PostgreSQL service' 'missing or ambiguous'
    $global:ESTERIUM_TEST_PG_MISSING = $false
    $inventoryParameters.RedisServiceName = 'Invalid''Service'
    Assert-Reject 'a quote in Windows service name' 'names must contain'
    $inventoryParameters.RedisServiceName = 'EsteriumCI-Redis'
    $inventoryParameters.RedisPort = 5432
    Assert-Reject 'database and cache sharing a port' 'different ports'
    $inventoryParameters.RedisPort = 6379
    $testExe = Join-Path $toolsDir 'pg_restore.exe'
    Remove-Item -LiteralPath $testExe -Force
    Assert-Reject 'missing PostgreSQL recovery tooling' 'PostgreSQL tool is missing'
    Write-Output 'PASS: synthetic Windows dependency inventory rejects LAN exposure, invalid or missing services, and missing backup tooling.'
} finally {
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    foreach ($name in @('ESTERIUM_TEST_PG_RUNNING', 'ESTERIUM_TEST_PG_WRAPPED', 'ESTERIUM_TEST_PG_WRONG_OWNER', 'ESTERIUM_TEST_PG_WRONG_PARENT', 'ESTERIUM_TEST_REDIS_RUNNING', 'ESTERIUM_TEST_LAN_BIND', 'ESTERIUM_TEST_PG_MISSING', 'ESTERIUM_TEST_PORT_OWNER_MISMATCH', 'ESTERIUM_TEST_PG_PID_MISSING', 'ESTERIUM_TEST_GARNET_WRAPPED', 'ESTERIUM_TEST_GARNET_WRONG_OWNER', 'ESTERIUM_TEST_GARNET_WRONG_PARENT', 'ESTERIUM_TEST_LISTENER_DELAY_CALLS', 'ESTERIUM_TEST_LISTENER_NEVER_PORT', 'ESTERIUM_TEST_LISTENER_READS')) {
        Remove-Variable -Name $name -Scope Global -ErrorAction SilentlyContinue
    }
}
