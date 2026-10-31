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
    [string]$ListenAddress='0.0.0.0:8443',
    [string]$TLSCertFile='',
    [string]$TLSKeyFile='',
    [string]$TrustedRemoteAddress='',
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
$listenerParts=$ListenAddress -split ':',2
if ($listenerParts.Count -ne 2 -or $listenerParts[1] -notmatch '^\d{1,5}$' -or
    $listenerParts[0] -notin @('0.0.0.0','127.0.0.1')) {
    throw 'ListenAddress must be 0.0.0.0:port for LAN or 127.0.0.1:port for local-only access.'
}
$origin=$null
if (-not [Uri]::TryCreate($AllowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
    $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or
    [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or
    $origin.Fragment -ne '') {
    throw 'Initial setup requires a separately provisioned valid root HTTPS origin.'
}
$originIsLoopback=$origin.Host -in @('localhost','127.0.0.1','::1')
if ($originIsLoopback -and $listenerParts[0] -ne '127.0.0.1') {
    throw 'AllowedOrigin localhost may only be used with a loopback listener.'
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
    foreach ($file in @('check-esterium-signed-release.ps1','verify-esterium-bundle.ps1','bootstrap-esterium-first-admin.ps1','activate-esterium-local-first-install.ps1')) {
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
    Write-Output 'PLAN: require trusted HTTPS certificate/key, create first admin, probe real dependencies, then activate automatic startup.'
    Write-Output 'PLAN: LAN access requires an explicit private IPv4 CIDR; firewall stays limited to Domain/Private profiles.'
    Write-Output 'NOT READY: this plan does not install PostgreSQL vendor files or perform the independent backup restore/pilot.'
    return
}
if ([string]::IsNullOrWhiteSpace($TLSCertFile) -or [string]::IsNullOrWhiteSpace($TLSKeyFile)) {
    throw 'Apply requires an HTTPS certificate chain and private key provisioned under protected ProgramData\Esterium.'
}
function Assert-TLSPEMSource([string]$path,[bool]$isCertificate) {
    if (-not [IO.Path]::IsPathRooted($path) -or $path.StartsWith('\\')) {
        throw 'HTTPS certificate inputs must be local absolute file paths.'
    }
    $full=[IO.Path]::GetFullPath($path)
    $cursor=$full
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'HTTPS certificate inputs cannot use junctions or symbolic links.'
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    $item=Get-Item -LiteralPath $full -Force -ErrorAction Stop
    if ($item.PSIsContainer -or $item.Length -le 0) { throw 'HTTPS certificate input must be a non-empty file.' }
    $text=[IO.File]::ReadAllText($full)
    try {
        if ($isCertificate -and $text -notmatch '-----BEGIN CERTIFICATE-----') {
            throw 'HTTPS certificate chain must use PEM format.'
        }
        if (-not $isCertificate -and $text -notmatch '-----BEGIN (?:RSA |EC )?PRIVATE KEY-----') {
            throw 'HTTPS private key must be an unencrypted PEM private key.'
        }
    } finally { $text=$null }
}
Assert-TLSPEMSource $TLSCertFile $true
Assert-TLSPEMSource $TLSKeyFile $false
$httpsPort=[int]$listenerParts[1]
if ($httpsPort -lt 1 -or $httpsPort -gt 65535 -or $httpsPort -in @($PostgresPort,$GarnetPort)) {
    throw 'HTTPS port must be valid and distinct from PostgreSQL and Garnet.'
}
$originPort=if ($origin.IsDefaultPort) { 443 } else { $origin.Port }
if ($originPort -ne $httpsPort) { throw 'AllowedOrigin port must match the ERP HTTPS listener.' }
if ($listenerParts[0] -eq '0.0.0.0' -and [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)) {
    throw 'LAN setup requires an explicit trusted private IPv4 CIDR.'
}
if ($listenerParts[0] -eq '127.0.0.1' -and -not [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)) {
    throw 'Loopback-only setup must not open a LAN firewall rule.'
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
function Copy-ApprovedTLSFile([string]$sourcePath,[string]$targetPath,[string]$label) {
    if (-not [IO.Path]::IsPathRooted($sourcePath) -or $sourcePath.StartsWith('\\')) {
        throw "$label source must be a local absolute file path."
    }
    $sourceFull=[IO.Path]::GetFullPath($sourcePath)
    $cursor=$sourceFull
    while ($cursor) {
        $entry=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "$label source cannot use a junction or symbolic link."
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    $sourceItem=Get-Item -LiteralPath $sourceFull -Force -ErrorAction Stop
    if ($sourceItem.PSIsContainer -or $sourceItem.Length -le 0 -or (Test-Path -LiteralPath $targetPath)) {
        throw "$label source must be non-empty and the protected destination must be new."
    }
    $sourceHash=(Get-FileHash -LiteralPath $sourceFull -Algorithm SHA256).Hash
    Copy-Item -LiteralPath $sourceFull -Destination $targetPath -ErrorAction Stop
    if ((Get-FileHash -LiteralPath $targetPath -Algorithm SHA256).Hash -cne $sourceHash) {
        throw "$label changed during protected staging."
    }
    & icacls.exe $targetPath '/inheritance:r' '/grant:r' '*S-1-5-19:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not protect staged $label." }
    return [IO.Path]::GetFullPath($targetPath)
}
$TLSCertFile=Copy-ApprovedTLSFile $TLSCertFile (Join-Path $ApplicationSecretsDir 'https-server-chain.pem') 'HTTPS certificate chain'
$TLSKeyFile=Copy-ApprovedTLSFile $TLSKeyFile (Join-Path $ApplicationSecretsDir 'https-server-key.pem') 'HTTPS private key'
$registration=@{
    BundleDir=$StageDir
    DatabaseURLFile=(Join-Path $DatabaseDestinationDir 'database-url.secret')
    RedisURLFile=(Join-Path $BootstrapDir 'redis-url.secret')
    RedisCACertFile=(Join-Path $BootstrapDir 'garnet-server.cer')
    JWTSecretFile=(Join-Path $ApplicationSecretsDir 'jwt.secret')
    MetricsBearerTokenFile=(Join-Path $ApplicationSecretsDir 'metrics.secret')
    AllowedOrigin=$AllowedOrigin
    ListenAddress=$ListenAddress
    TLSCertFile=$TLSCertFile
    TLSKeyFile=$TLSKeyFile
    TrustedRemoteAddress=$TrustedRemoteAddress
    InstallDir=$InstallDir
    ApprovedManifestSHA256=$ApprovedManifestSHA256
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
}
$probe=@{BundleDir=$InstallDir;PostgresBin=$PostgresBin;PostgresDataDir=$PostgresDataDir;
    PostgresOptionsDir=$PostgresOptionsDir;PostgresTlsPolicyDir=$PostgresTlsPolicyDir;
    GarnetConfigDir=$GarnetConfigDir;AllowedOrigin=$AllowedOrigin;ListenAddress=$ListenAddress;
    TLSCertFile=$TLSCertFile;TLSKeyFile=$TLSKeyFile;TrustedRemoteAddress=$TrustedRemoteAddress;
    PostgresServiceName=$PostgresServiceName;
    PostgresServiceAccount=$PostgresServiceAccount;GarnetServiceName=$GarnetServiceName;
    PostgresPort=$PostgresPort;GarnetPort=$GarnetPort;
    ApprovedManifestSHA256=$ApprovedManifestSHA256;
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint;Apply=$true}
# No registered but unvalidated application service may remain startable
# after a failed proof, including a failed final receipt write.
try {
    & (Join-Path $stageTool 'register-esterium-windows-service.ps1') @registration | Out-Null
    $adminBootstrap=@{
        DatabaseURLFile=(Join-Path $DatabaseDestinationDir 'database-url.secret')
        BootstrapAdminExe=(Join-Path $stageTool 'esterium-bootstrap-admin.exe')
        TrustedPublisherThumbprint=$TrustedPublisherThumbprint
        Apply=$true
    }
    # SQL bootstrap deliberately leaves PostgreSQL disabled/stopped. Start it only
    # while creating the one-time administrator, then restore the fail-closed
    # state required by the dependency probe before it temporarily starts services.
    try {
        Set-Service -Name $PostgresServiceName -StartupType Manual -ErrorAction Stop
        Start-Service -Name $PostgresServiceName -ErrorAction Stop
        (Get-Service -Name $PostgresServiceName -ErrorAction Stop).WaitForStatus(
            [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(90))
        & (Join-Path $stageTool 'bootstrap-esterium-first-admin.ps1') @adminBootstrap | Out-Null
    } finally {
        try {
            $postgres=Get-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue
            if ($null -ne $postgres -and $postgres.Status -ne 'Stopped') {
                Stop-Service -Name $PostgresServiceName -Force -ErrorAction Stop
                (Get-Service -Name $PostgresServiceName -ErrorAction Stop).WaitForStatus(
                    [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
            }
            Set-Service -Name $PostgresServiceName -StartupType Disabled -ErrorAction Stop
            $postgresState=Get-CimInstance Win32_Service -Filter ("Name='{0}'" -f $PostgresServiceName) -ErrorAction Stop
            if ($null -eq $postgresState -or $postgresState.State -cne 'Stopped' -or
                $postgresState.StartMode -cne 'Disabled' -or [int]$postgresState.ProcessId -ne 0) {
                throw 'PostgreSQL did not return to its required stopped/disabled state.'
            }
        } catch {
            throw 'First-admin bootstrap could not verify PostgreSQL stopped and disabled; manual intervention required.'
        }
    }
    & (Join-Path $InstallDir 'tools\probe-esterium-local-first-run.ps1') @probe | Out-Null
    $activation=@{
        BundleDir=$InstallDir;AllowedOrigin=$AllowedOrigin;ListenAddress=$ListenAddress
        TLSCertFile=$TLSCertFile;TLSKeyFile=$TLSKeyFile;TrustedRemoteAddress=$TrustedRemoteAddress
        ReadinessReceiptDir=(Join-Path $env:ProgramData 'Esterium\FirstRunReadiness')
        ActivationReceiptDir=(Join-Path $env:ProgramData 'Esterium\Installation')
        PostgresDataDir=$PostgresDataDir;GarnetConfigDir=$GarnetConfigDir
        PostgresServiceName=$PostgresServiceName;GarnetServiceName=$GarnetServiceName
        ApprovedManifestSHA256=$ApprovedManifestSHA256;TrustedPublisherThumbprint=$TrustedPublisherThumbprint
        Apply=$true
    }
    & (Join-Path $InstallDir 'tools\activate-esterium-local-first-install.ps1') @activation | Out-Null
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
Write-Output 'PASS: signed bundle, schema/RBAC, first admin, protected credentials and HTTPS health check completed.'
Write-Output 'PASS: PostgreSQL, Garnet and ERP startup, HTTPS shortcut and activation receipt are configured.'
Write-Output 'FISCAL TRANSMISSION REMAINS DISABLED; external backup restore and physical pilot remain separate gates.'
