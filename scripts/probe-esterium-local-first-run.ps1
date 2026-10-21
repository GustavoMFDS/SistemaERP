#requires -Version 5.1
<#
  Signed opt-in probe of a fully installed but STOPPED Esterium Windows host.
  It temporarily starts PostgreSQL and Garnet, runs esterium-server.exe
  --check-dependencies, and always disables/stops both afterward.
  Never enables ERP, runs migrations, touches fiscal provider or serves HTTP.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDir,
    [string]$PostgresBin=(Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresTlsPolicyDir=(Join-Path $env:ProgramData 'Esterium\PostgresTlsPolicy'),
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$GarnetConfigDir=(Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [string]$ReadinessReceiptDir=(Join-Path $env:ProgramData 'Esterium\FirstRunReadiness'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [string]$GarnetServiceName='EsteriumGarnet',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'First-run readiness requires Windows.' }
if ($PostgresPort -in @($GarnetPort,8080) -or $GarnetPort -eq 8080) {
    throw 'Database and cache ports must be distinct from ERP.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($name -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or $name -eq 'EsteriumERP') {
        throw 'Unexpected dependency service identity.'
    }
}
if ($PostgresServiceName -eq $GarnetServiceName -or
    $PostgresServiceAccount -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,30}$') {
    throw 'Unsafe or overlapping Windows service accounts.'
}
if ($Apply) {
    if ($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
        $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$') {
        throw 'First-run probe requires independently approved signature and release digest.'
    }
    $sig=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $sig.SignerCertificate -or
        $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
          $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'First-run probe executable is not signed by approved publisher.'
    }
}
function Assert-Local([string]$path,[bool]$directory) {
    if (-not [IO.Path]::IsPathRooted($path) -or $path.StartsWith('\\')) {
        throw 'First-run probe accepts only local absolute paths.'
    }
    $full=[IO.Path]::GetFullPath($path)
    $cursor=$full
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'First-run probe forbids junctions or symbolic links.'
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    if ((Get-Item -LiteralPath $full).PSIsContainer -ne $directory) {
        throw 'First-run probe received a file of the wrong type.'
    }
    return $full
}
$bundle=Assert-Local $BundleDir $true
$prefix=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')+'\'
if (-not $bundle.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Only ERP bundles already installed under Program Files can be probed.'
}
if ($Apply) {
    if (-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),
        (Join-Path $bundle 'tools'),[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Run only the approved installed script copy.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $bundle -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
} else {
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
}
$bin=Assert-Local $PostgresBin $true
$data=Assert-Local $PostgresDataDir $true
$policy=Assert-Local $PostgresTlsPolicyDir $true
$options=Assert-Local $PostgresOptionsDir $true
$garnet=Assert-Local $GarnetConfigDir $true
$receiptTarget=[IO.Path]::GetFullPath($ReadinessReceiptDir).TrimEnd('\')
if (-not [IO.Path]::IsPathRooted($ReadinessReceiptDir) -or
    $ReadinessReceiptDir.StartsWith('\') -or
    -not $receiptTarget.StartsWith([IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase) -or
    (Test-Path -LiteralPath $receiptTarget)) {
    throw 'First-run probe receipt destination must be new and protected under ProgramData.'
}
$cursor=Split-Path -Parent $receiptTarget
while ($cursor) {
    if (Test-Path -LiteralPath $cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'First-run readiness receipt cannot use symbolic-link or junction ancestors.'
        }
    }
    $parent=Split-Path -Parent $cursor
    if ($parent -eq $cursor) { break }
    $cursor=$parent
}
$base=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
foreach ($path in @($data,$policy,$options,$garnet)) {
    if (-not $path.StartsWith($base,[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Database and cache material must be located under ProgramData\Esterium.'
    }
}
if ([IO.File]::ReadAllText((Assert-Local (Join-Path $data 'PG_VERSION') $false)).Trim() -cne '16') {
    throw 'First-run probe requires PostgreSQL 16.'
}
$receipt=Assert-Local (Join-Path $options ('PostgresTlsApplied-'+$PostgresServiceName+'.receipt')) $false
$marker=Assert-Local (Join-Path $options ('PostgresFresh-'+$PostgresServiceName+'.marker')) $false
$proof=[IO.File]::ReadAllText($marker).Trim() -split [char]10
if ($proof.Count -ne 3 -or $proof[0].Trim() -cne 'ESTERIUM_FRESH_POSTGRES_16' -or
    $proof[1].Trim() -cne $PostgresServiceName -or
    -not [string]::Equals($proof[2].Trim(),$data,[StringComparison]::OrdinalIgnoreCase) -or
    -not ([IO.File]::ReadAllText($receipt)).StartsWith('TLS_POLICY_STAGED_NEVER_STARTED')) {
    throw 'Staged PostgreSQL TLS first-install proof is missing or inconsistent.'
}
foreach ($name in @('OriginalPostgresql-'+$PostgresServiceName+'.conf',
                    'OriginalPgHba-'+$PostgresServiceName+'.conf')) {
    $null=Assert-Local (Join-Path $options $name) $false
}
& (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf (Join-Path $data 'postgresql.conf') -PgHbaConf (Join-Path $data 'pg_hba.conf') | Out-Null
$null=Assert-Local (Join-Path $policy 'certificates\root.crt') $false
foreach ($name in @('postgres.exe','psql.exe','pg_dump.exe','pg_restore.exe')) {
    $null=Assert-Local (Join-Path $bin $name) $false
}
$garnetConf=Assert-Local (Join-Path $garnet 'garnet.conf') $false
$configuration=Get-Content -LiteralPath $garnetConf -Raw | ConvertFrom-Json -ErrorAction Stop
if ($configuration.Recover -cne $true -or $configuration.FailOnRecoveryError -cne $true -or
    $configuration.EnableAOF -cne $true -or $configuration.WaitForCommit -cne $true -or
    $configuration.EnableTLS -cne $true -or $configuration.AuthenticationMode -cne 'ACL' -or
    $configuration.Address -cne '127.0.0.1' -or [int]$configuration.Port -ne $GarnetPort) {
    throw 'Durable Garnet local TLS configuration is not approved.'
}
$services=@{}
foreach ($name in @($PostgresServiceName,$GarnetServiceName,'EsteriumERP')) {
    $list=@(Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop)
    if ($list.Count -ne 1 -or $list[0].State -cne 'Stopped' -or
        [int]$list[0].ProcessId -ne 0) {
        throw 'First-run probe requires all three Windows services stopped.'
    }
    $services[$name]=$list[0]
}
if ($services[$PostgresServiceName].StartMode -cne 'Disabled' -or
    $services[$GarnetServiceName].StartMode -cne 'Disabled' -or
    $services['EsteriumERP'].StartMode -cne 'Manual' -or
    $services['EsteriumERP'].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or
    $services[$GarnetServiceName].StartName -notin @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE') -or
    $services[$PostgresServiceName].StartName -notin @($PostgresServiceAccount,
        ('.\'+$PostgresServiceAccount),($env:COMPUTERNAME+'\'+$PostgresServiceAccount))) {
    throw 'Service identity and startup policy do not match approved local installation.'
}
foreach ($port in @($PostgresPort,$GarnetPort)) {
    if (@(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -gt 0) {
        throw 'Dependency port is already in use before the probe.'
    }
}
$registry='HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP'
$serviceEnv=@{}
foreach ($entry in @((Get-ItemProperty -LiteralPath $registry -Name Environment -ErrorAction Stop).Environment)) {
    $i=$entry.IndexOf('=')
    if ($i -le 0) { throw 'Malformed service environment entry.' }
    $key=$entry.Substring(0,$i)
    if ($key -cnotmatch '^[A-Z][A-Z0-9_]{1,70}$' -or $serviceEnv.ContainsKey($key)) {
        throw 'Duplicate or invalid service environment entry.'
    }
    $serviceEnv[$key]=$entry.Substring($i+1)
}
foreach ($key in @('APP_ENV','FISCAL_PROVIDER','NFCE_SEFAZ_PRODUCTION_ENABLED',
    'NFCE_SEFAZ_HOMOLOGATION_ENABLED','DATABASE_URL_FILE','REDIS_URL_FILE',
    'REDIS_CA_CERT_FILE','JWT_SECRET_FILE','METRICS_BEARER_TOKEN_FILE',
    'ESTERIUM_MIGRATIONS_DIR','ESTERIUM_WEB_DIR','HTTP_ADDR','APP_PUBLIC_URL')) {
    if ([string]::IsNullOrWhiteSpace($serviceEnv[$key])) { throw 'ERP runtime setting missing.' }
}
if ($serviceEnv['APP_ENV'] -cne 'prod' -or
    $serviceEnv['FISCAL_PROVIDER'] -cne 'disabled' -or
    $serviceEnv['NFCE_SEFAZ_PRODUCTION_ENABLED'] -cne 'false' -or
    $serviceEnv['NFCE_SEFAZ_HOMOLOGATION_ENABLED'] -cne 'false' -or
    $serviceEnv['HTTP_ADDR'] -cne '127.0.0.1:8080') {
    throw 'ERP must remain loopback-only with fiscal transmission disabled.'
}
if (-not [string]::Equals([IO.Path]::GetFullPath($serviceEnv['ESTERIUM_WEB_DIR']),
    (Join-Path $bundle 'web'),[StringComparison]::OrdinalIgnoreCase) -or
    -not [string]::Equals([IO.Path]::GetFullPath($serviceEnv['ESTERIUM_MIGRATIONS_DIR']),
    (Join-Path $bundle 'migrations'),[StringComparison]::OrdinalIgnoreCase)) {
    throw 'ERP must use frontend and migrations from installed approved release.'
}
foreach ($key in @('DATABASE_URL_FILE','REDIS_URL_FILE','REDIS_CA_CERT_FILE',
    'JWT_SECRET_FILE','METRICS_BEARER_TOKEN_FILE')) {
    $file=Assert-Local $serviceEnv[$key] $false
    $hasLocalService=$false
    foreach ($ace in (Get-Acl -LiteralPath $file).Access) {
        if ($ace.AccessControlType -ne 'Allow' -or
            ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -eq 0) {
            continue
        }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if ($sid -notin @('S-1-5-18','S-1-5-32-544','S-1-5-19')) {
            throw 'ERP credential has an unapproved Windows reader.'
        }
        if ($sid -eq 'S-1-5-19') { $hasLocalService=$true }
    }
    if (-not $hasLocalService) { throw 'ERP service must read its own protected file.' }
}
if (-not $Apply) {
    Write-Output 'PLAN: installed TLS/SQL, Garnet AOF, SCM identities and protected ERP secret paths were checked.'
    Write-Output 'PLAN: signed Apply will temporarily start PostgreSQL/Garnet and run --check-dependencies without serving HTTP.'
    Write-Output 'PLAN: PostgreSQL and Garnet will be stopped/disabled after the probe, including on failure.'
    Write-Output 'NOT ENABLED: no automatic startup, customer data or fiscal authorization.'
    return
}
if (-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Elevated Windows administrator is required.'
}
$success=$false
try {
    # Disallow an external SCM trigger from starting ERP while dependencies
    # are temporarily alive. On a failed probe ERP remains Disabled.
    Set-Service -Name 'EsteriumERP' -StartupType Disabled -ErrorAction Stop
    foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
        Set-Service -Name $name -StartupType Manual -ErrorAction Stop
        Start-Service -Name $name -ErrorAction Stop
        (Get-Service -Name $name).WaitForStatus(
            [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    }
    & (Join-Path $PSScriptRoot 'check-esterium-local-dependencies.ps1') -PostgresServiceName $PostgresServiceName -RedisServiceName $GarnetServiceName -PostgresBin $bin -PostgresPort $PostgresPort -RedisPort $GarnetPort -RequireServiceOwnership | Out-Null
    $process=[Diagnostics.ProcessStartInfo]::new()
    $process.FileName=Assert-Local (Join-Path $bundle 'esterium-server.exe') $false
    $process.Arguments='--check-dependencies'
    $process.UseShellExecute=$false
    $process.CreateNoWindow=$true
    $process.RedirectStandardOutput=$true
    $process.RedirectStandardError=$true
    $process.EnvironmentVariables.Clear()
    foreach ($name in @('SystemRoot','WINDIR','PATH','TEMP','TMP','USERPROFILE')) {
        $value=[Environment]::GetEnvironmentVariable($name)
        if ($value) { $process.EnvironmentVariables[$name]=$value }
    }
    foreach ($key in $serviceEnv.Keys) { $process.EnvironmentVariables[$key]=$serviceEnv[$key] }
    $client=[Diagnostics.Process]::Start($process)
    if ($null -eq $client) { throw 'ERP dependency preflight could not launch.' }
    try {
        $stdout=$client.StandardOutput.ReadToEndAsync()
        $stderr=$client.StandardError.ReadToEndAsync()
        if (-not $client.WaitForExit(60000)) {
            Stop-Process -Id $client.Id -Force -ErrorAction SilentlyContinue
            throw 'ERP dependency preflight exceeded one minute.'
        }
        $null=$stdout.GetAwaiter().GetResult()
        $null=$stderr.GetAwaiter().GetResult()
        if ($client.ExitCode -ne 0) {
            throw 'ERP rejected PostgreSQL schema or Garnet TLS/atomic security requirements.'
        }
    } finally { $client.Dispose() }
    $success=$true
} finally {
    # Always attempt BOTH services, even when the first stop command fails.
    # Retain a hard failure if any process is still running or auto-start
    # policy was not disabled. No success receipt is ever created on failure.
    $cleanupFailures=@()
    foreach ($name in @($GarnetServiceName,$PostgresServiceName)) {
        try {
            if ((Get-Service -Name $name -ErrorAction Stop).Status -ne 'Stopped') {
                Stop-Service -Name $name -Force -ErrorAction Stop
                (Get-Service -Name $name).WaitForStatus(
                    [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
            }
        } catch { $cleanupFailures+=('stop:'+ $name) }
        try { Set-Service -Name $name -StartupType Disabled -ErrorAction Stop }
        catch { $cleanupFailures+=('disable:'+ $name) }
    }
    foreach ($name in @($GarnetServiceName,$PostgresServiceName)) {
        try {
            $actual=Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop
            if ($actual.StartMode -cne 'Disabled' -or $actual.State -cne 'Stopped' -or
                [int]$actual.ProcessId -ne 0) { $cleanupFailures+=('unsafe-state:'+ $name) }
        } catch { $cleanupFailures+=('unverifiable-state:'+ $name) }
    }
    if ($cleanupFailures.Count -gt 0) {
        throw 'Probe cleanup could not confirm BOTH dependencies stopped and disabled; manual intervention required.'
    }
}
if (-not $success) { throw 'Runtime probe did not pass.' }
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    $state=Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop
    if ($state.StartMode -cne 'Disabled' -or $state.State -cne 'Stopped' -or [int]$state.ProcessId -ne 0) {
        throw 'Runtime probe could not verify both dependencies stopped and disabled.'
    }
}
# Only a completed, successful probe produces a one-time audit receipt.
# It deliberately does NOT authorize automatic ERP startup or fiscal traffic.
$receiptParent=Split-Path -Parent $receiptTarget
[IO.Directory]::CreateDirectory($receiptParent) | Out-Null
$staging=Join-Path $receiptParent ('.esterium-first-run-'+[Guid]::NewGuid().ToString('N'))
$committed=$false
try {
    [IO.Directory]::CreateDirectory($staging) | Out-Null
    & icacls.exe $staging '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'First-run verification receipt folder ACL rejected.' }
    $meta=[ordered]@{
        kind='ESTERIUM_LOCAL_DEPENDENCY_PROBE'
        schemaVersion=1
        stage='dependencies-validated-services-disabled'
        checkedUtc=[DateTime]::UtcNow.ToString('o')
        releaseManifestSHA256=(Get-FileHash -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Algorithm SHA256).Hash
        postgresqlConfigSHA256=(Get-FileHash -LiteralPath $config -Algorithm SHA256).Hash
        garnetConfigSHA256=(Get-FileHash -LiteralPath $garnetConf -Algorithm SHA256).Hash
        postgresService=$PostgresServiceName
        garnetService=$GarnetServiceName
        automaticStartupApproved=$false
        backupRestoreApproved=$false
        fiscalEmissionApproved=$false
    }
    $out=Join-Path $staging 'probe-result.json'
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $meta -Depth 3)+[Environment]::NewLine)
    try {
        $stream=[IO.File]::Open($out,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $stream.Write($bytes,0,$bytes.Length);$stream.Flush($true) }
        finally { $stream.Dispose() }
    } finally { [Array]::Clear($bytes,0,$bytes.Length) }
    & icacls.exe $out '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'First-run verification receipt could not be protected.' }
    if (Test-Path -LiteralPath $receiptTarget) {
        throw 'Another attempt already created a first-run readiness receipt.'
    }
    Move-Item -LiteralPath $staging -Destination $receiptTarget -ErrorAction Stop
    $committed=$true
} finally {
    if (-not $committed -and (Test-Path -LiteralPath $staging)) {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
}
Set-Service -Name 'EsteriumERP' -StartupType Manual -ErrorAction Stop
Write-Output 'PASS: PostgreSQL schema and Garnet TLS/ACL/Lua verified by real ERP preflight.'
Write-Output 'NOT ENABLED: dependencies stopped/disabled, ERP remains stopped/manual; backup restoration and release approval are still required.'
