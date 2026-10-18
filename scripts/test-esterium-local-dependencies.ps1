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
$global:ESTERIUM_TEST_REDIS_RUNNING = $true
$global:ESTERIUM_TEST_LAN_BIND = $false
$global:ESTERIUM_TEST_PG_MISSING = $false
function Get-CimInstance {
    [CmdletBinding()]
    param([string]$ClassName, [string]$Filter)
    if ($ClassName -ne 'Win32_Service') { throw 'Unexpected CIM request.' }
    if ($Filter -eq "Name='EsteriumCI-Postgres'") {
        if ($global:ESTERIUM_TEST_PG_MISSING) { return }
        return [PSCustomObject]@{
            Name='EsteriumCI-Postgres'
            State=$(if ($global:ESTERIUM_TEST_PG_RUNNING) { 'Running' } else { 'Stopped' })
        }
    }
    if ($Filter -eq "Name='EsteriumCI-Redis'") {
        return [PSCustomObject]@{
            Name='EsteriumCI-Redis'
            State=$(if ($global:ESTERIUM_TEST_REDIS_RUNNING) { 'Running' } else { 'Stopped' })
        }
    }
    throw 'Unexpected Windows service requested.'
}
function Get-NetTCPConnection {
    [CmdletBinding()]
    param([string]$State, [int]$LocalPort)
    if ($State -ne 'Listen' -or $LocalPort -notin @(5432, 6379)) {
        throw 'Unexpected TCP inventory request.'
    }
    if ($global:ESTERIUM_TEST_LAN_BIND -and $LocalPort -eq 6379) {
        return [PSCustomObject]@{ LocalAddress='0.0.0.0'; LocalPort=$LocalPort }
    }
    return [PSCustomObject]@{ LocalAddress='127.0.0.1'; LocalPort=$LocalPort }
}
try {
    [IO.Directory]::CreateDirectory($toolsDir) | Out-Null
    foreach ($binary in @('postgres.exe','pg_dump.exe','pg_restore.exe','psql.exe')) {
        [IO.File]::WriteAllText((Join-Path $toolsDir $binary), 'synthetic bytes not executed')
    }
    $args = @{
        PostgresServiceName='EsteriumCI-Postgres'
        RedisServiceName='EsteriumCI-Redis'
        PostgresBin=$toolsDir
    }
    $ok = @(& $checker @args) -join ' '
    if ($ok -notmatch 'PASS: declared PostgreSQL') {
        throw 'Synthetic healthy local services were not recognized.'
    }
    function Assert-Reject([string]$reason, [string]$message) {
        $rejected = $false
        $actualError = '(no exception)'
        try { & $checker @args | Out-Null }
        catch {
            $actualError = $_.Exception.Message
            $rejected = $actualError -match $message
        }
        if (-not $rejected) { throw "Inventory unexpectedly accepted $reason, or failed for another reason: $actualError" }
    }
    $global:ESTERIUM_TEST_LAN_BIND = $true
    Assert-Reject 'Redis bound on all interfaces' 'non-loopback address'
    $global:ESTERIUM_TEST_LAN_BIND = $false
    $global:ESTERIUM_TEST_PG_RUNNING = $false
    Assert-Reject 'a stopped PostgreSQL service' 'not running'
    $global:ESTERIUM_TEST_PG_RUNNING = $true
    $global:ESTERIUM_TEST_PG_MISSING = $true
    Assert-Reject 'a missing PostgreSQL service' 'missing or ambiguous'
    $global:ESTERIUM_TEST_PG_MISSING = $false
    $args.RedisServiceName = 'Invalid''Service'
    Assert-Reject 'a quote in Windows service name' 'names must contain'
    $args.RedisServiceName = 'EsteriumCI-Redis'
    $args.RedisPort = 5432
    Assert-Reject 'database and cache sharing a port' 'different ports'
    $args.RedisPort = 6379
    $testExe = Join-Path $toolsDir 'pg_restore.exe'
    Remove-Item -LiteralPath $testExe -Force
    Assert-Reject 'missing PostgreSQL recovery tooling' 'PostgreSQL tool is missing'
    Write-Output 'PASS: synthetic Windows dependency inventory rejects LAN exposure, invalid or missing services, and missing backup tooling.'
} finally {
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    foreach ($name in @('ESTERIUM_TEST_PG_RUNNING', 'ESTERIUM_TEST_REDIS_RUNNING', 'ESTERIUM_TEST_LAN_BIND', 'ESTERIUM_TEST_PG_MISSING')) {
        Remove-Variable -Name $name -Scope Global -ErrorAction SilentlyContinue
    }
}
