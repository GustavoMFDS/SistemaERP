#requires -Version 5.1
<#
  Signed new-install SQL bootstrap. After offline TLS staging, starts only
  the new PostgreSQL service, creates app role and DB, applies immutable SQL,
  then ALWAYS returns PostgreSQL to Disabled/Stopped. Never touches ERP,
  Garnet, SEFAZ or a previously initialized shop database.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDir,
    [string]$PostgresBin=(Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$PostgresTlsPolicyDir=(Join-Path $env:ProgramData 'Esterium\PostgresTlsPolicy'),
    [string]$BootstrapDir=(Join-Path $env:ProgramData 'Esterium\DependencyBootstrap'),
    [string]$DatabaseDestinationDir=(Join-Path $env:ProgramData 'Esterium\PostgresApplication'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'SQL bootstrap requires Windows.' }
if ($PostgresPort -in @(6379,8080) -or $PostgresServiceName -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or
    $PostgresServiceAccount -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,30}$') {
    throw 'SQL bootstrap service identity or port is invalid.'
}
if ($Apply) {
    if ($ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
        $TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'SQL bootstrap requires independent release hash and publisher approval.'
    }
    $sig=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $sig.SignerCertificate -or
        $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'SQL bootstrap script is not signed by approved publisher.'
    }
}
function Assert-Local([string]$path,[bool]$directory) {
    if (-not [IO.Path]::IsPathRooted($path) -or $path.StartsWith('\\')) {
        throw 'SQL bootstrap requires absolute local disk paths.'
    }
    $full=[IO.Path]::GetFullPath($path).TrimEnd('\')
    $cur=$full
    while ($cur) {
        $item=Get-Item -LiteralPath $cur -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'SQL bootstrap forbids junction and symbolic-link paths.'
        }
        $parent=Split-Path -Parent $cur
        if ($parent -eq $cur) { break }
        $cur=$parent
    }
    if ((Get-Item -LiteralPath $full).PSIsContainer -ne $directory) {
        throw 'Unexpected file or folder in SQL bootstrap.'
    }
    return $full
}
$bundle=Assert-Local $BundleDir $true
$prefix=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')+'\'
if (-not $bundle.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)) {
    throw 'SQL bootstrap requires ERP release already installed in Program Files.'
}
if ($Apply) {
    if (-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),(Join-Path $bundle 'tools'),[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Only the approved installed SQL bootstrap is executable.'
    }
    $gate=@{BundleDir=$bundle;ExpectedManifestSHA256=$ApprovedManifestSHA256;ExpectedPublisherThumbprint=$TrustedPublisherThumbprint}
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') @gate | Out-Null
} else {
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
}
$bin=Assert-Local $PostgresBin $true
if (-not [string]::Equals($bin,([IO.Path]::GetFullPath((Join-Path $env:ProgramFiles 'PostgreSQL\16\bin')).TrimEnd('\')),[StringComparison]::OrdinalIgnoreCase)) {
    throw 'SQL bootstrap accepts only the installed PostgreSQL 16 command-line tools.'
}
$data=Assert-Local $PostgresDataDir $true
$options=Assert-Local $PostgresOptionsDir $true
$policy=Assert-Local $PostgresTlsPolicyDir $true
$bootstrap=Assert-Local $BootstrapDir $true
$base=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
foreach ($dir in @($data,$options,$policy,$bootstrap)) {
    if (-not $dir.StartsWith($base,[StringComparison]::OrdinalIgnoreCase)) {
        throw 'SQL bootstrap requires protected ProgramData Esterium directories.'
    }
}
$destination=[IO.Path]::GetFullPath($DatabaseDestinationDir).TrimEnd('\')
if (-not [IO.Path]::IsPathRooted($DatabaseDestinationDir) -or $DatabaseDestinationDir.StartsWith('\\') -or
    -not $destination.StartsWith($base,[StringComparison]::OrdinalIgnoreCase) -or
    (Test-Path -LiteralPath $destination)) {
    throw 'SQL application output must be a NEW dedicated ProgramData folder.'
}
$parent=Split-Path -Parent $destination
while ($parent) {
    if (Test-Path -LiteralPath $parent) {
        if (((Get-Item -LiteralPath $parent -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'SQL bootstrap output cannot have junction ancestors.'
        }
    }
    $next=Split-Path -Parent $parent
    if ($next -eq $parent) { break }
    $parent=$next
}
if ([IO.File]::ReadAllText((Assert-Local (Join-Path $data 'PG_VERSION') $false)).Trim() -cne '16') {
    throw 'SQL bootstrap requires PostgreSQL PG_VERSION 16.'
}
$proof=Assert-Local (Join-Path $options ('PostgresFresh-'+$PostgresServiceName+'.marker')) $false
$receipt=Assert-Local (Join-Path $options ('PostgresTlsApplied-'+$PostgresServiceName+'.receipt')) $false
$fields=[IO.File]::ReadAllText($proof).Trim() -split [char]10
if ($fields.Count -ne 3 -or $fields[0].Trim() -cne 'ESTERIUM_FRESH_POSTGRES_16' -or
    $fields[1].Trim() -cne $PostgresServiceName -or
    -not [string]::Equals($fields[2].Trim(),$data,[StringComparison]::OrdinalIgnoreCase) -or
    -not ([IO.File]::ReadAllText($receipt)).StartsWith('TLS_POLICY_STAGED_NEVER_STARTED')) {
    throw 'Missing matching first-install TLS proof.'
}
& (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf (Join-Path $data 'postgresql.conf') -PgHbaConf (Join-Path $data 'pg_hba.conf') | Out-Null
$psql=Assert-Local (Join-Path $bin 'psql.exe') $false
$ca=Assert-Local (Join-Path $policy 'certificates\root.crt') $false
$super=Assert-Local (Join-Path $bootstrap 'pg-super.secret') $false
$app=Assert-Local (Join-Path $bootstrap 'pg-app.secret') $false
$migrator=Assert-Local (Join-Path $bundle 'tools\esterium-migrate.exe') $false
$migrations=Assert-Local (Join-Path $bundle 'migrations') $true
$services=@(Get-CimInstance Win32_Service -Filter "Name='$PostgresServiceName'" -ErrorAction Stop)
if ($services.Count -ne 1 -or $services[0].State -cne 'Stopped' -or
    $services[0].StartMode -cne 'Disabled' -or [int]$services[0].ProcessId -ne 0 -or
    $services[0].StartName -notin @($PostgresServiceAccount,('.\'+$PostgresServiceAccount),
        ($env:COMPUTERNAME+'\'+$PostgresServiceAccount))) {
    throw 'SQL bootstrap requires new nonprivileged PostgreSQL service Disabled/Stopped.'
}
if (@(Get-NetTCPConnection -State Listen -LocalPort $PostgresPort -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'PostgreSQL port is in use before first SQL initialization.'
}
if (-not $Apply) {
    Write-Output 'PLAN: temporarily start PostgreSQL 16, create non-admin SQL role/database, and apply approved migrations.'
    Write-Output 'PLAN: disable/stop PostgreSQL on success or failure, without ERP/Garnet/SEFAZ startup.'
    Write-Output 'PLAN: never replay SQL installation if directory or schema already exists.'
    return
}
if (-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'SQL first install requires elevated administrator.'
}
$completed=$false
try {
    Set-Service -Name $PostgresServiceName -StartupType Manual -ErrorAction Stop
    Start-Service -Name $PostgresServiceName -ErrorAction Stop
    (Get-Service -Name $PostgresServiceName).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $init=@{PsqlExe=$psql;SuperPasswordFile=$super;AppPasswordFile=$app;
        RootCertificateFile=$ca;DestinationDir=$destination;PostgresPort=$PostgresPort;
        TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
    & (Join-Path $PSScriptRoot 'initialize-esterium-postgres-app-db.ps1') @init | Out-Null
    $schema=@{DatabaseURLFile=(Join-Path $destination 'database-url.secret');
        PsqlExe=$psql;MigrateExe=$migrator;MigrationsDir=$migrations;BundleDir=$bundle;
        ApprovedManifestSHA256=$ApprovedManifestSHA256;
        TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
    & (Join-Path $PSScriptRoot 'apply-esterium-initial-schema.ps1') @schema | Out-Null
    $completed=$true
} finally {
    # A partial/dirty schema is never automatically rolled back or retried.
    $cleanup=@()
    try {
        if ((Get-Service -Name $PostgresServiceName -ErrorAction Stop).Status -ne 'Stopped') {
            Stop-Service -Name $PostgresServiceName -Force -ErrorAction Stop
            (Get-Service -Name $PostgresServiceName).WaitForStatus(
                [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
        }
    } catch { $cleanup+= 'stop' }
    try { Set-Service -Name $PostgresServiceName -StartupType Disabled -ErrorAction Stop }
    catch { $cleanup+= 'disable' }
    try {
        $state=Get-CimInstance Win32_Service -Filter "Name='$PostgresServiceName'" -ErrorAction Stop
        if ($state.State -cne 'Stopped' -or $state.StartMode -cne 'Disabled' -or
            [int]$state.ProcessId -ne 0) { $cleanup+= 'state' }
    } catch { $cleanup+= 'verify' }
    if ($cleanup.Count -gt 0) {
        throw 'SQL bootstrap could not verify PostgreSQL stopped/disabled; manual repair required.'
    }
}
if (-not $completed) { throw 'Initial SQL bootstrap is incomplete and cannot be replayed automatically.' }
Write-Output 'PASS: initial SQL role, database and migrations created; PostgreSQL stopped and disabled.'
Write-Output 'NOT ENABLED: backup/restore, Garnet ERP preflight and release activation remain separate.'
