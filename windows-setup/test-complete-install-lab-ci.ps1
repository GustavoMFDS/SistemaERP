#requires -Version 7.2
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:ESTERIUM_FULL_INSTALL_DISPOSABLE_CI -ne '1' -or [string]::IsNullOrWhiteSpace($env:GITHUB_WORKSPACE) -or [string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
    throw 'Complete installation is allowed only on the explicitly enabled disposable GitHub Windows runner.'
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'The disposable Windows installation lab requires its hosted-runner administrator.' }
$repo = [IO.Path]::GetFullPath($env:GITHUB_WORKSPACE)
$tempRoot = [IO.Path]::GetFullPath((Join-Path $env:RUNNER_TEMP ('esterium-complete-install-' + [Guid]::NewGuid().ToString('N'))))
$bundle = Join-Path $tempRoot 'bundle'
$vendor = Join-Path $tempRoot 'vendor'
$evidence = Join-Path $env:RUNNER_TEMP 'esterium-full-install-evidence'
$labHost = 'esterium.ci.invalid'
$httpsPort = 0
$postgresPort = 0
$garnetPort = 0
$privateAddress = ''
$hostFile = Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
$hostOriginalLength = -1L
$hostAppendBytes = $null
$signer = $null
$httpsCertificate = $null
$signerThumbprint = ''
$httpsThumbprint = ''
$global:LabAdminEmail = 'ci-admin@esterium.invalid'
$global:LabAdminPassword = 'CIonly-' + [Guid]::NewGuid().ToString('N')
$global:LabPromptValues = @{
    'Razao social da empresa' = 'Esterium CI Laboratorio Ltda'
    'Nome fantasia (opcional)' = 'Esterium CI'
    'CNPJ (14 digitos)' = '00000000000191'
    'Nome do administrador' = 'Pessoa Administradora CI'
    'E-mail do administrador' = $global:LabAdminEmail
    'Crie uma senha forte (12 a 72 bytes UTF-8)' = $global:LabAdminPassword
    'Confirme a senha' = $global:LabAdminPassword
}
function global:Read-Host {
    param([string]$Prompt, [switch]$AsSecureString)
    if (-not $global:LabPromptValues.ContainsKey($Prompt)) { throw 'The installer requested an unexpected first-administrator prompt.' }
    $value = [string]$global:LabPromptValues[$Prompt]
    if ($AsSecureString) { return (ConvertTo-SecureString -String $value -AsPlainText -Force) }
    return $value
}
function Get-FreeTcpPort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Any, 0)
    try { $listener.Start(); return ([Net.IPEndPoint]$listener.LocalEndpoint).Port }
    finally { $listener.Stop() }
}
function Get-PrivateIPv4Address {
    $addresses = @(Get-NetIPAddress -AddressFamily IPv4 -Type Unicast -ErrorAction Stop | Where-Object {
        $_.AddressState -eq 'Preferred' -and -not $_.SkipAsSource -and
        ($_.IPAddress.StartsWith('10.') -or $_.IPAddress -match '^172\.(1[6-9]|2[0-9]|3[01])\.' -or $_.IPAddress -match '^192\.168\.')
    } | Sort-Object InterfaceIndex)
    if ($addresses.Count -eq 0) { throw 'The hosted Windows runner has no RFC1918 IPv4 address for a scoped LAN fixture.' }
    return [string]$addresses[0].IPAddress
}
function Invoke-PinnedDownload([string]$Uri, [string]$Path, [string]$ExpectedSHA256) {
    & curl.exe '-fLsS' '--retry' '2' '--connect-timeout' '25' '--max-time' '420' '-o' $Path $Uri
    if ($LASTEXITCODE -ne 0) { throw 'Pinned vendor download failed on the disposable runner.' }
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -cne $ExpectedSHA256.ToUpperInvariant()) { throw 'A downloaded vendor file differs from its independently pinned SHA-256.' }
}
function ConvertTo-Pem([string]$Label, [byte[]]$Bytes) {
    $encoded = [Convert]::ToBase64String($Bytes)
    $lines = [Collections.Generic.List[string]]::new()
    for ($offset = 0; $offset -lt $encoded.Length; $offset += 64) {
        $length = [Math]::Min(64, $encoded.Length - $offset)
        $lines.Add($encoded.Substring($offset, $length))
    }
    $newline = [Environment]::NewLine
    return ('-----BEGIN {0}-----{1}{2}{1}-----END {0}-----{1}' -f $Label, $newline, ($lines -join $newline))
}
function Wait-ERPReady([int]$Port, [int]$TimeoutSeconds) {
    $uri = 'https://{0}:{1}/health/ready' -f $labHost, $Port
    for ($attempt = 0; $attempt -lt $TimeoutSeconds; $attempt++) {
        $service = Get-Service -Name 'EsteriumERP' -ErrorAction SilentlyContinue
        if ($null -ne $service -and $service.Status -eq 'Running') {
            try { $response = Invoke-RestMethod -Uri $uri -Method Get -TimeoutSec 5; if ($response.status -eq 'ready') { return } }
            catch {}
        }
        Start-Sleep -Seconds 1
    }
    $service = Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction SilentlyContinue
    throw ('ERP HTTPS readiness failed; SCM state={0}.' -f $service.State)
}
function Assert-AdminLogin([string]$Origin) {
    $body = @{ email = $global:LabAdminEmail; password = $global:LabAdminPassword } | ConvertTo-Json -Compress
    $response = Invoke-RestMethod -Uri ($Origin + '/api/v1/auth/login') -Method Post -ContentType 'application/json' -Body $body -TimeoutSec 15
    if ([string]::IsNullOrWhiteSpace([string]$response.token.access_token)) { throw 'Synthetic first administrator could not sign in to the installed HTTPS ERP.' }
}
function Assert-FirewallRule([int]$Port, [string]$ExpectedRemoteAddress) {
    $rules = @(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction Stop)
    if ($rules.Count -ne 1 -or $rules[0].Enabled -ne 'True' -or $rules[0].Action -ne 'Allow' -or $rules[0].Direction -ne 'Inbound' -or [string]$rules[0].Profile -notmatch 'Domain' -or [string]$rules[0].Profile -notmatch 'Private' -or [string]$rules[0].Profile -match 'Public') {
        throw 'The installer did not create exactly one enabled Domain/Private inbound HTTPS firewall rule.'
    }
    $portFilter = $rules[0] | Get-NetFirewallPortFilter
    $addressFilter = $rules[0] | Get-NetFirewallAddressFilter
    $applicationFilter = $rules[0] | Get-NetFirewallApplicationFilter
    $remote = @($addressFilter.RemoteAddress | ForEach-Object { [string]$_ })
    if ([string]$portFilter.Protocol -ne 'TCP' -or [string]$portFilter.LocalPort -ne [string]$Port -or $remote.Count -ne 1 -or
        -not ([string]::Equals($remote[0], $ExpectedRemoteAddress, [StringComparison]::OrdinalIgnoreCase) -or [string]::Equals($remote[0], $privateAddress, [StringComparison]::OrdinalIgnoreCase)) -or
        -not [string]::Equals([string]$applicationFilter.Program, (Join-Path $env:ProgramFiles 'EsteriumERP\esterium-server.exe'), [StringComparison]::OrdinalIgnoreCase)) {
        throw 'The HTTPS firewall rule does not match the app, port, or single-host synthetic CI scope.'
    }
}
function Assert-InstalledServices {
    $expected = @(
        @{ Name = 'EsteriumPostgreSQL16'; AccountPattern = '(?i)esterium_pgsvc$' },
        @{ Name = 'EsteriumGarnet'; AccountPattern = '(?i)NetworkService$' },
        @{ Name = 'EsteriumERP'; AccountPattern = '(?i)LocalService$' }
    )
    foreach ($entry in $expected) {
        $service = Get-CimInstance Win32_Service -Filter ("Name='{0}'" -f $entry.Name) -ErrorAction Stop
        if ($null -eq $service -or $service.State -ne 'Running' -or $service.StartMode -ne 'Auto' -or $service.StartName -notmatch $entry.AccountPattern) {
            throw ('Installed service identity, automatic startup, or state is wrong: {0}.' -f $entry.Name)
        }
    }
}
function Register-LabCertificateTrust([string]$Thumbprint) {
    $source = 'Cert:\LocalMachine\My\' + $Thumbprint
    foreach ($store in @('Cert:\LocalMachine\Root', 'Cert:\LocalMachine\TrustedPublisher')) {
        if (-not (Test-Path -LiteralPath ($store + '\' + $Thumbprint))) { Copy-Item -LiteralPath $source -Destination $store -ErrorAction Stop | Out-Null }
    }
}

[IO.Directory]::CreateDirectory($tempRoot) | Out-Null
[IO.Directory]::CreateDirectory($vendor) | Out-Null
[IO.Directory]::CreateDirectory($evidence) | Out-Null
try {
    $forbiddenServices = @(Get-CimInstance Win32_Service -ErrorAction Stop | Where-Object { $_.Name -in @('EsteriumPostgreSQL16', 'EsteriumGarnet', 'EsteriumERP') })
    if ($forbiddenServices.Count -ne 0) { throw 'The disposable runner already has an Esterium service; refusing to overwrite it.' }
    if ((Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumERP')) -or (Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'PostgreSQL\16')) -or (Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumGarnet')) -or (Test-Path -LiteralPath (Join-Path $env:ProgramData 'Esterium'))) {
        throw 'The disposable runner has pre-existing Esterium files; refusing to overwrite them.'
    }
    $privateAddress = Get-PrivateIPv4Address
    $ports = [Collections.Generic.List[int]]::new()
    while ($ports.Count -lt 3) {
        $candidate = Get-FreeTcpPort
        if ($candidate -ne 8080 -and -not $ports.Contains($candidate)) { $ports.Add($candidate) }
    }
    $postgresPort = $ports[0]
    $garnetPort = $ports[1]
    $httpsPort = $ports[2]
    $origin = 'https://{0}:{1}' -f $labHost, $httpsPort
    $remoteScope = $privateAddress + '/32'

    $hostOriginalLength = (Get-Item -LiteralPath $hostFile -ErrorAction Stop).Length
    $hostAppendBytes = [Text.UTF8Encoding]::new($false).GetBytes([Environment]::NewLine + $privateAddress + ' ' + $labHost + [Environment]::NewLine)
    $hostStream = [IO.File]::Open($hostFile, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::Read)
    try { $hostStream.Seek(0, [IO.SeekOrigin]::End) | Out-Null; $hostStream.Write($hostAppendBytes, 0, $hostAppendBytes.Length); $hostStream.Flush($true) }
    finally { $hostStream.Dispose() }

    $signer = New-SelfSignedCertificate -Type CodeSigningCert -Subject ('CN=Esterium Disposable CI Signer ' + [Guid]::NewGuid().ToString('N')) -CertStoreLocation 'Cert:\LocalMachine\My' -KeyAlgorithm RSA -KeyLength 2048 -KeyExportPolicy Exportable -NotAfter (Get-Date).AddDays(14)
    $signerThumbprint = $signer.Thumbprint.ToUpperInvariant()
    Register-LabCertificateTrust $signerThumbprint
    $httpsCertificate = New-SelfSignedCertificate -Type SSLServerAuthentication -DnsName $labHost -FriendlyName 'Esterium disposable CI HTTPS only' -CertStoreLocation 'Cert:\LocalMachine\My' -KeyAlgorithm RSA -KeyLength 2048 -KeyExportPolicy Exportable -NotAfter (Get-Date).AddDays(14)
    $httpsThumbprint = $httpsCertificate.Thumbprint.ToUpperInvariant()
    Register-LabCertificateTrust $httpsThumbprint
    $httpsCertPath = Join-Path $tempRoot 'ci-https-cert.cer'
    Export-Certificate -Cert $httpsCertificate -FilePath $httpsCertPath -Force | Out-Null
    $httpsCertPem = Join-Path $tempRoot 'ci-https-chain.pem'
    $httpsKeyPem = Join-Path $tempRoot 'ci-https-key.pem'
    $certBytes = $httpsCertificate.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)
    [IO.File]::WriteAllText($httpsCertPem, (ConvertTo-Pem 'CERTIFICATE' $certBytes), [Text.UTF8Encoding]::new($false))
    $rsa = [Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPrivateKey($httpsCertificate)
    try {
        $keyBytes = $rsa.ExportPkcs8PrivateKey()
        [IO.File]::WriteAllText($httpsKeyPem, (ConvertTo-Pem 'PRIVATE KEY' $keyBytes), [Text.UTF8Encoding]::new($false))
        [Array]::Clear($keyBytes, 0, $keyBytes.Length)
    }
    finally { $rsa.Dispose() }
    [Array]::Clear($certBytes, 0, $certBytes.Length)

    $runtimeDotnet = Join-Path $bundle 'tools\dotnet\dotnet.exe'
    $runtimeDll = Join-Path $bundle 'tools\dotnet\shared\Microsoft.NETCore.App\10.0.14\System.Private.CoreLib.dll'
    $buildScript = Join-Path $repo 'scripts\build-esterium-local-windows.ps1'
    Write-Output 'Building the exact Windows ERP bundle in the disposable runner.'
    & npm.cmd install --global npm@11.6.1
    if ($LASTEXITCODE -ne 0) { throw 'Pinned npm toolchain setup failed.' }
    & $buildScript -OutputDir $bundle
    if (-not (Test-Path -LiteralPath (Join-Path $bundle 'bundle-manifest.json')) -or -not (Test-Path -LiteralPath $runtimeDotnet) -or -not (Test-Path -LiteralPath $runtimeDll)) {
        throw 'The Windows bundle or its pinned .NET runtime was not built completely.'
    }
    $runtimeHashBefore = (Get-FileHash -LiteralPath $runtimeDotnet -Algorithm SHA256).Hash
    if ((Get-AuthenticodeSignature -LiteralPath $runtimeDotnet).Status -ne [Management.Automation.SignatureStatus]::Valid) { throw 'The unchanged Microsoft .NET runtime must retain its vendor signature.' }

    Write-Output 'Creating a runner-local, non-release Authenticode identity for the signed path.'
    $signedTargets = @(Get-ChildItem -LiteralPath (Join-Path $bundle 'tools') -Filter '*.ps1' -File | ForEach-Object { $_.FullName })
    $signedTargets += @((Join-Path $bundle 'esterium-server.exe'), (Join-Path $bundle 'tools\esterium-migrate.exe'), (Join-Path $bundle 'tools\esterium-pg-tls-certgen.exe'), (Join-Path $bundle 'tools\esterium-bootstrap-admin.exe'))
    foreach ($path in $signedTargets) {
        $signature = Set-AuthenticodeSignature -LiteralPath $path -Certificate $signer -HashAlgorithm SHA256
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or $null -eq $signature.SignerCertificate -or $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $signerThumbprint) {
            throw 'A generated Esterium executable or installer script could not be signed by the disposable test identity.'
        }
    }
    if ((Get-FileHash -LiteralPath $runtimeDotnet -Algorithm SHA256).Hash -cne $runtimeHashBefore -or (Get-AuthenticodeSignature -LiteralPath $runtimeDotnet).Status -ne [Management.Automation.SignatureStatus]::Valid) {
        throw 'The Microsoft .NET runtime changed during synthetic package signing.'
    }
    $manifestPath = Join-Path $bundle 'bundle-manifest.json'
    Remove-Item -LiteralPath $manifestPath -Force -ErrorAction Stop
    & (Join-Path $repo 'scripts\write-esterium-bundle-manifest.ps1') -BundleDir $bundle
    $manifestSHA256 = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash
    $releaseCheck = Join-Path $bundle 'tools\check-esterium-signed-release.ps1'
    & $releaseCheck -BundleDir $bundle -ExpectedManifestSHA256 $manifestSHA256 -ExpectedPublisherThumbprint $signerThumbprint
    Write-Output 'PASS: bundle integrity verified; all Esterium code is signed; Microsoft runtime remains vendor-signed.'

    $edbHash = 'DE926FEFAD00E313E212CD438C0F04BF033E200099AD56C012724EFCEBED79F2'
    $edbSigner = '7BEDD1269FCCF7A5D95F18274750B79893C06C70'
    $garnetHash = '5CD130686AB415B13C59D36D048A6FADED81E707FFE6DA5953F98BEEB79F9F16'
    $nssmArchiveHash = '99F5045FFFBFFB745D67FE3A065A953C4A3D9C253B868892D9B685B0EE7D07B8'
    $nssmHash = 'EEE9C44C29C2BE011F1F1E43BB8C3FCA888CB81053022EC5A0060035DE16D848'
    $edbExe = Join-Path $vendor 'postgresql-16.15-1-windows-x64.exe'
    $garnetZip = Join-Path $vendor 'garnet-2.2.3-win-x64.zip'
    $nupkg = Join-Path $vendor 'nssm-2.24.101-g897c7ad.nupkg'
    $nssmZip = Join-Path $vendor 'nssm-2.24-101-g897c7ad.zip'
    $nssmExtract = Join-Path $vendor 'nssm-unpacked'
    $nssmExe = Join-Path $vendor 'nssm.exe'
    Write-Output 'Downloading and verifying the pinned EDB, Garnet, and NSSM vendor inputs.'
    Invoke-PinnedDownload 'https://get.enterprisedb.com/postgresql/postgresql-16.15-1-windows-x64.exe' $edbExe $edbHash
    Invoke-PinnedDownload 'https://github.com/microsoft/garnet/releases/download/v2.2.3/win-x64-based-readytorun.zip' $garnetZip $garnetHash
    & curl.exe '-fLsS' '--retry' '2' '--connect-timeout' '25' '--max-time' '180' '-o' $nupkg 'https://community.chocolatey.org/api/v2/package/NSSM/2.24.101-g897c7ad'
    if ($LASTEXITCODE -ne 0) { throw 'Pinned NSSM transport package download failed.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $packageArchive = [IO.Compression.ZipFile]::OpenRead($nupkg)
    try {
        $entries = @($packageArchive.Entries | Where-Object { $_.FullName -ceq 'tools/nssm-2.24-101-g897c7ad.zip' })
        if ($entries.Count -ne 1) { throw 'NSSM transport did not contain exactly one expected inner archive.' }
        $inputStream = $entries[0].Open()
        $outputStream = [IO.File]::Create($nssmZip)
        try { $inputStream.CopyTo($outputStream) }
        finally { $outputStream.Dispose(); $inputStream.Dispose() }
    }
    finally { $packageArchive.Dispose() }
    if ((Get-FileHash -LiteralPath $nssmZip -Algorithm SHA256).Hash -cne $nssmArchiveHash) { throw 'The NSSM archive differs from the independently pinned release.' }
    Expand-Archive -LiteralPath $nssmZip -DestinationPath $nssmExtract -Force
    $nssmSource = Join-Path $nssmExtract 'nssm-2.24-101-g897c7ad\win64\nssm.exe'
    Copy-Item -LiteralPath $nssmSource -Destination $nssmExe -ErrorAction Stop
    if ((Get-FileHash -LiteralPath $nssmExe -Algorithm SHA256).Hash -cne $nssmHash) { throw 'The x64 NSSM service wrapper differs from the independently pinned release.' }
    $edbSignature = Get-AuthenticodeSignature -LiteralPath $edbExe
    if ($edbSignature.Status -ne [Management.Automation.SignatureStatus]::Valid -or $null -eq $edbSignature.SignerCertificate -or $edbSignature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $edbSigner) {
        throw 'The pinned EDB installer does not have its approved vendor Authenticode identity.'
    }

    Write-Output 'Applying the real complete first-install coordinator on this disposable runner.'
    $completeInstaller = Join-Path $bundle 'tools\install-esterium-local-complete.ps1'
    $installArgs = @{
        SourceBundleDir = $bundle
        AllowedOrigin = $origin
        PostgresInstaller = $edbExe
        ExpectedPostgresSHA256 = $edbHash
        ExpectedPostgresPublisherThumbprint = $edbSigner
        GarnetPackage = $garnetZip
        ExpectedGarnetSHA256 = $garnetHash
        ServiceWrapperExe = $nssmExe
        ExpectedServiceWrapperSHA256 = $nssmHash
        ListenAddress = '0.0.0.0:' + $httpsPort
        TLSCertFile = $httpsCertPem
        TLSKeyFile = $httpsKeyPem
        TrustedRemoteAddress = $remoteScope
        ApprovedManifestSHA256 = $manifestSHA256
        TrustedPublisherThumbprint = $signerThumbprint
        PostgresPort = $postgresPort
        GarnetPort = $garnetPort
        Apply = $true
    }
    & $completeInstaller @installArgs
    Assert-InstalledServices
    Wait-ERPReady $httpsPort 120
    Assert-AdminLogin $origin
    Assert-FirewallRule $httpsPort $remoteScope
    $activationReceiptPath = Join-Path $env:ProgramData 'Esterium\Installation\installation-ready.json'
    $activationReceipt = Get-Content -LiteralPath $activationReceiptPath -Raw | ConvertFrom-Json
    if ($activationReceipt.fiscalEmissionApproved -ne $false -or [string]$activationReceipt.allowedOrigin -cne $origin -or [string]$activationReceipt.trustedRemoteAddress -cne $remoteScope) {
        throw 'Activation receipt must document synthetic non-fiscal mode and the exact HTTPS scope.'
    }
    $shortcut = Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)) 'Esterium ERP.url'
    if (-not (Test-Path -LiteralPath $shortcut) -or [IO.File]::ReadAllText($shortcut) -notmatch ('(?m)^URL=' + [Regex]::Escape($origin) + '\r?$')) {
        throw 'The first install did not create its expected all-users HTTPS shortcut.'
    }
    Write-Output 'PASS: signed coordinator installed EDB/PostgreSQL, protected DB secrets, Garnet TLS/ACL/AOF, schema, first admin, ERP, HTTPS, scoped firewall, and shortcut.'

    Write-Output 'Installing Playwright browsers and exercising real installed services over trusted HTTPS.'
    Push-Location (Join-Path $repo 'web')
    try {
        & npx.cmd playwright install chromium firefox
        if ($LASTEXITCODE -ne 0) { throw 'Playwright Chromium/Firefox installation failed.' }
        $env:E2E_BASE_URL = $origin
        $env:ESTERIUM_LAB_ADMIN_EMAIL = $global:LabAdminEmail
        $env:ESTERIUM_LAB_ADMIN_PASSWORD = $global:LabAdminPassword
        $env:ESTERIUM_INSTALLED_LAB = '1'
        $env:E2E_IGNORE_HTTPS_ERRORS = '0'
        $env:ESTERIUM_LAB_EVIDENCE_DIR = $evidence
        & npx.cmd playwright test --config playwright.installed-lab.config.ts
        if ($LASTEXITCODE -ne 0) { throw 'Installed-service Playwright acceptance failed.' }
    }
    finally {
        Remove-Item Env:E2E_BASE_URL -ErrorAction SilentlyContinue
        Remove-Item Env:ESTERIUM_LAB_ADMIN_EMAIL -ErrorAction SilentlyContinue
        Remove-Item Env:ESTERIUM_LAB_ADMIN_PASSWORD -ErrorAction SilentlyContinue
        Remove-Item Env:ESTERIUM_INSTALLED_LAB -ErrorAction SilentlyContinue
        Remove-Item Env:E2E_IGNORE_HTTPS_ERRORS -ErrorAction SilentlyContinue
        Remove-Item Env:ESTERIUM_LAB_EVIDENCE_DIR -ErrorAction SilentlyContinue
        Pop-Location
    }

    Write-Output 'Restarting real dependencies and ERP to verify persisted first-install readiness.'
    Stop-Service -Name 'EsteriumERP' -Force -ErrorAction Stop
    (Get-Service -Name 'EsteriumERP').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(45))
    Restart-Service -Name 'EsteriumPostgreSQL16' -Force -ErrorAction Stop
    (Get-Service -Name 'EsteriumPostgreSQL16').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(90))
    Restart-Service -Name 'EsteriumGarnet' -Force -ErrorAction Stop
    (Get-Service -Name 'EsteriumGarnet').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(90))
    Start-Service -Name 'EsteriumERP' -ErrorAction Stop
    (Get-Service -Name 'EsteriumERP').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(90))
    Wait-ERPReady $httpsPort 120
    Assert-AdminLogin $origin

    Write-Output 'Removing only the owned shortcut and firewall rule to exercise assisted repair.'
    Remove-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction Stop
    Remove-Item -LiteralPath $shortcut -Force -ErrorAction Stop
    $repair = Join-Path $env:ProgramFiles 'EsteriumERP\tools\repair-esterium-local-install.ps1'
    $repairArgs = @{ InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'); ApprovedManifestSHA256 = $manifestSHA256; TrustedPublisherThumbprint = $signerThumbprint; Apply = $true; ConfirmRepair = $true }
    & $repair @repairArgs
    Assert-FirewallRule $httpsPort $remoteScope
    if (-not (Test-Path -LiteralPath $shortcut)) { throw 'Assisted repair did not restore the installer-owned HTTPS shortcut.' }
    Wait-ERPReady $httpsPort 60
    Assert-AdminLogin $origin
    Write-Output 'PASS: signed assisted repair restored only the missing owned HTTPS rule and shortcut.'

    Write-Output 'Testing conservative app uninstall while retaining synthetic dependency data.'
    Stop-Service -Name 'EsteriumERP' -Force -ErrorAction Stop
    (Get-Service -Name 'EsteriumERP').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(45))
    $uninstall = Join-Path $env:ProgramFiles 'EsteriumERP\tools\uninstall-esterium-local-app.ps1'
    $uninstallArgs = @{ InstallDir = (Join-Path $env:ProgramFiles 'EsteriumERP'); ApprovedManifestSHA256 = $manifestSHA256; TrustedPublisherThumbprint = $signerThumbprint; Apply = $true; ConfirmUninstall = $true }
    & $uninstall @uninstallArgs
    if ((Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'EsteriumERP')) -or
        @(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction SilentlyContinue).Count -ne 0 -or
        @(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction SilentlyContinue).Count -ne 0 -or
        (Test-Path -LiteralPath $shortcut)) { throw 'Conservative uninstall left its owned ERP service, rule, shortcut, or bundle behind.' }
    foreach ($preservedPath in @(
        (Join-Path $env:ProgramFiles 'PostgreSQL\16'),
        (Join-Path $env:ProgramFiles 'EsteriumGarnet'),
        (Join-Path $env:ProgramData 'Esterium\PostgresData\PG_VERSION'),
        (Join-Path $env:ProgramData 'Esterium\PostgresApplication\database-url.secret'),
        (Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
        (Join-Path $env:ProgramData 'Esterium\Secrets')
    )) {
        if (-not (Test-Path -LiteralPath $preservedPath)) { throw ('Conservative uninstall removed preserved dependency/data material: {0}.' -f $preservedPath) }
    }
    foreach ($dependency in @('EsteriumPostgreSQL16', 'EsteriumGarnet')) {
        if ($null -eq (Get-Service -Name $dependency -ErrorAction SilentlyContinue)) { throw ('Conservative uninstall removed dependency service {0}.' -f $dependency) }
    }
    Write-Output 'PASS: uninstall removed the application and archived receipts while preserving PostgreSQL, Garnet, credentials, and SQL data.'
    Write-Output 'LIMIT: this disposable runner did not perform a Windows OS reboot, physical LAN pilot, external backup restore, vendor/peripheral acceptance, real SEFAZ transmission, or commercial signing.'
}
finally {
    Remove-Item Function:\global:Read-Host -Force -ErrorAction SilentlyContinue
    foreach ($serviceName in @('EsteriumERP', 'EsteriumGarnet', 'EsteriumPostgreSQL16')) {
        try {
            $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
            if ($service) {
                if ($service.Status -ne 'Stopped') { Stop-Service -Name $serviceName -Force -ErrorAction Stop }
                Set-Service -Name $serviceName -StartupType Disabled -ErrorAction SilentlyContinue
            }
        }
        catch { Write-Warning ('Could not stop disposable lab service ' + $serviceName + '; the hosted VM will be discarded.') }
    }
    if ($httpsCertificate) {
        $httpsThumbprint = $httpsCertificate.Thumbprint.ToUpperInvariant()
        foreach ($store in @('Cert:\LocalMachine\My', 'Cert:\LocalMachine\Root', 'Cert:\LocalMachine\TrustedPublisher')) {
            $entry = Join-Path $store $httpsThumbprint
            if (Test-Path -LiteralPath $entry) { Remove-Item -LiteralPath $entry -Force -ErrorAction SilentlyContinue }
        }
    }
    if ($signer) {
        $signerThumbprint = $signer.Thumbprint.ToUpperInvariant()
        foreach ($store in @('Cert:\LocalMachine\My', 'Cert:\LocalMachine\Root', 'Cert:\LocalMachine\TrustedPublisher')) {
            $entry = Join-Path $store $signerThumbprint
            if (Test-Path -LiteralPath $entry) { Remove-Item -LiteralPath $entry -Force -ErrorAction SilentlyContinue }
        }
    }
    if ($hostOriginalLength -ge 0 -and $hostAppendBytes) {
        try {
            $stream = [IO.File]::Open($hostFile, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::Read)
            try { if ($stream.Length -eq ($hostOriginalLength + $hostAppendBytes.Length)) { $stream.SetLength($hostOriginalLength); $stream.Flush($true) } }
            finally { $stream.Dispose() }
        }
        catch { Write-Warning 'Hosted runner cleanup left its disposable hosts entry; the VM will be discarded after this job.' }
    }
    if ($tempRoot -and $tempRoot.StartsWith(([IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\') + '\'), [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $tempRoot)) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
