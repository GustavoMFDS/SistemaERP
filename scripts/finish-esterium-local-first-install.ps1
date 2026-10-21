#requires -Version 5.1
<#
  Signed first-install coordinator AFTER setup-esterium-local-dependencies.ps1.
  Prepares approved ERP source in Program Files, bootstraps the SQL schema,
  creates app secrets, registers ERP stopped/manual, runs first-run probe.
  No rollback of partial SQL, no automatic startup, no SEFAZ or selling.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$SourceBundleDir,
    [Parameter(Mandatory=$true)][string]$AllowedOrigin,
    [string]$StageDir=(Join-Path $env:ProgramFiles 'EsteriumApprovedSetup'),
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$PostgresBin=(Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$PostgresTlsPolicyDir=(Join-Path $env:ProgramData 'Esterium\PostgresTlsPolicy'),
    [string]$GarnetConfigDir=(Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [string]$BootstrapDir=(Join-Path $env:ProgramData 'Esterium\DependencyBootstrap'),
    [string]$DatabaseDestinationDir=(Join-Path $env:ProgramData 'Esterium\PostgresApplication'),
    [string]$ApplicationSecretsDir=(Join-Path $env:ProgramData 'Esterium\Secrets'),
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
if ($env:OS -ne 'Windows_NT') { throw 'Esterium initial coordinator requires Windows.' }
$origin=$null
if (-not [Uri]::TryCreate($AllowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
    $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or
    [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or
    $origin.Fragment -ne '' -or $origin.Host -in @('localhost','127.0.0.1','::1')) {
    throw 'Initial setup requires a separately provisioned valid HTTPS origin, not localhost.'
}
if ($PostgresPort -in @($GarnetPort,8080) -or $GarnetPort -eq 8080) {
    throw 'Database, cache and HTTP ports must remain distinct.'
}
if ($PostgresServiceName -eq $GarnetServiceName -or
    $PostgresServiceName -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,63}$' -or
    $GarnetServiceName -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,63}$' -or
    $PostgresServiceAccount -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,30}$') {
    throw 'Initial setup contains invalid local Windows service identities.'
}
if ($Apply) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[0-9a-fA-F]{40}$' -or
        $ApprovedManifestSHA256 -cnotmatch '^[0-9a-fA-F]{64}$') {
        throw 'Initial setup requires an independently approved manifest SHA-256 and publisher.'
    }
    # Authenticate delegated verification code BEFORE invoking it as admin.
    foreach ($file in @('check-esterium-signed-release.ps1','verify-esterium-bundle.ps1')) {
        $auth=Get-AuthenticodeSignature -LiteralPath (Join-Path $PSScriptRoot $file)
        if ($auth.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $auth.SignerCertificate -or
            $auth.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Initial setup approval helper is not signed by approved publisher.'
        }
    }
    $signature=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
          $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Initial setup coordinator is not signed by independently approved publisher.'
    }
}
if (-not [IO.Path]::IsPathRooted($SourceBundleDir) -or $SourceBundleDir.StartsWith('\\')) {
    throw 'Initial setup bundle must be stored on a local disk.'
}
$source=[IO.Path]::GetFullPath($SourceBundleDir).TrimEnd('\')
if (-not (Test-Path -LiteralPath $source -PathType Container)) { throw 'Approved ERP source bundle missing.' }
if ($Apply) {
    if (-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),
        (Join-Path $source 'tools'),[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Only a signed ERP bundle may start initial Windows setup.'
    }
    $approval=@{BundleDir=$source;ExpectedManifestSHA256=$ApprovedManifestSHA256;ExpectedPublisherThumbprint=$TrustedPublisherThumbprint}
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') @approval | Out-Null
} else {
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $source | Out-Null
}
foreach ($path in @($StageDir,$InstallDir,$DatabaseDestinationDir,$ApplicationSecretsDir,
                    (Join-Path $env:ProgramData 'Esterium\FirstRunReadiness'))) {
    if (Test-Path -LiteralPath $path) {
        throw 'An initial installation folder already exists; manual assessment required.'
    }
}
if (-not $Apply) {
    Write-Output 'PLAN: stage the signed ERP release under protected Program Files, no services touched.'
    Write-Output 'PLAN: bootstrap first SQL database/migrations with temporary PostgreSQL start and safe shutdown.'
    Write-Output 'PLAN: generate app credentials, register ERP stopped/manual, probe both dependencies and disable them again.'
    Write-Output 'NOT READY: EDB EXE installation, external restored backup, signed release pilot and automatic startup remain separate.'
    return
}
if (-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Initial Windows setup requires elevated administrator.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    $svc=@(Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop)
    if ($svc.Count -ne 1 -or $svc[0].State -cne 'Stopped' -or
        $svc[0].StartMode -cne 'Disabled' -or [int]$svc[0].ProcessId -ne 0) {
        throw 'SQL and Garnet services must already exist and be disabled/stopped.'
    }
}
if (@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop).Count -ne 0) {
    throw 'Existing ERP service must never be overwritten by initial setup.'
}
$stageArgs=@{SourceBundleDir=$source;StageDir=$StageDir;ApprovedManifestSHA256=$ApprovedManifestSHA256;
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
& (Join-Path $PSScriptRoot 'stage-esterium-approved-windows-release.ps1') @stageArgs | Out-Null
$stageTool=Join-Path $StageDir 'tools'
$sqlArgs=@{BundleDir=$StageDir;PostgresBin=$PostgresBin;PostgresDataDir=$PostgresDataDir;
    PostgresOptionsDir=$PostgresOptionsDir;PostgresTlsPolicyDir=$PostgresTlsPolicyDir;
    BootstrapDir=$BootstrapDir;DatabaseDestinationDir=$DatabaseDestinationDir;
    PostgresServiceName=$PostgresServiceName;PostgresServiceAccount=$PostgresServiceAccount;
    PostgresPort=$PostgresPort;ApprovedManifestSHA256=$ApprovedManifestSHA256;
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
& (Join-Path $stageTool 'bootstrap-esterium-local-schema.ps1') @sqlArgs | Out-Null
& (Join-Path $stageTool 'provision-esterium-app-secrets.ps1') -DestinationDir $ApplicationSecretsDir -TrustedPublisherThumbprint $TrustedPublisherThumbprint -Apply | Out-Null
$registration=@{
    BundleDir=$StageDir
    DatabaseURLFile=(Join-Path $DatabaseDestinationDir 'database-url.secret')
    RedisURLFile=(Join-Path $BootstrapDir 'redis-url.secret')
    RedisCACertFile=(Join-Path $BootstrapDir 'garnet-server.cer')
    JWTSecretFile=(Join-Path $ApplicationSecretsDir 'jwt.secret')
    MetricsBearerTokenFile=(Join-Path $ApplicationSecretsDir 'metrics.secret')
    AllowedOrigin=$AllowedOrigin
    ListenAddress='127.0.0.1:8080'
    InstallDir=$InstallDir
    ApprovedManifestSHA256=$ApprovedManifestSHA256
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
}
$probe=@{BundleDir=$InstallDir;PostgresBin=$PostgresBin;PostgresDataDir=$PostgresDataDir;
    PostgresOptionsDir=$PostgresOptionsDir;PostgresTlsPolicyDir=$PostgresTlsPolicyDir;
    GarnetConfigDir=$GarnetConfigDir;PostgresServiceName=$PostgresServiceName;
    PostgresServiceAccount=$PostgresServiceAccount;GarnetServiceName=$GarnetServiceName;
    PostgresPort=$PostgresPort;GarnetPort=$GarnetPort;
    ApprovedManifestSHA256=$ApprovedManifestSHA256;
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
# No registered but unvalidated application service may remain startable
# after a failed proof, including a failed final receipt write.
try {
    & (Join-Path $stageTool 'register-esterium-windows-service.ps1') @registration | Out-Null
    & (Join-Path $InstallDir 'tools\probe-esterium-local-first-run.ps1') @probe | Out-Null
} catch {
    try {
        $erp=Get-Service -Name 'EsteriumERP' -ErrorAction SilentlyContinue
        if ($null -ne $erp) {
            Set-Service -Name 'EsteriumERP' -StartupType Disabled -ErrorAction Stop
            if ((Get-Service -Name 'EsteriumERP').Status -ne 'Stopped') {
                Stop-Service -Name 'EsteriumERP' -Force -ErrorAction Stop
                (Get-Service -Name 'EsteriumERP').WaitForStatus(
                    [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
            }
        }
    } catch {
        throw 'First install failed and could not disable the partial ERP Windows service; manual intervention required.'
    }
    throw
}
Write-Output 'PASS: signed release, initial SQL, protected app credentials and stopped ERP service installed; database/cache probe passed.'
Write-Output 'NOT READY: ERP not started; external restore, Windows EXE EDB installation, HTTPS client and physical pilot gates remain.'
