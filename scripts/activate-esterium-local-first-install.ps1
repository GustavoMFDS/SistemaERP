#requires -Version 5.1
<#
  Finalize a fresh Esterium installation only after the signed first-run probe.
  The application and dependency services become automatic only after HTTPS
  readiness succeeds. On failure, only resources created by this invocation
  are rolled back; database files, credentials and backups are preserved.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDir,
    [Parameter(Mandatory=$true)][string]$AllowedOrigin,
    [Parameter(Mandatory=$true)][string]$ListenAddress,
    [Parameter(Mandatory=$true)][string]$TLSCertFile,
    [Parameter(Mandatory=$true)][string]$TLSKeyFile,
    [string]$TrustedRemoteAddress='',
    [string]$ReadinessReceiptDir=(Join-Path $env:ProgramData 'Esterium\FirstRunReadiness'),
    [string]$ActivationReceiptDir=(Join-Path $env:ProgramData 'Esterium\Installation'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$GarnetConfigDir=(Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$GarnetServiceName='EsteriumGarnet',
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest

if($env:OS -ne 'Windows_NT'){throw 'Esterium installation activation requires Windows.'}
if($UnsignedDisposableCIFixture -and (
   $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
   $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1' -or
   $env:ESTERIUM_FINAL_ACTIVATION_DISPOSABLE_CI -ne '1')){
    throw 'Unsigned activation is restricted to disposable GitHub-hosted Windows CI.'
}
foreach($name in @($PostgresServiceName,$GarnetServiceName)){
    if($name -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or $name -eq 'EsteriumERP'){
        throw 'Invalid dependency service name.'
    }
}
if($PostgresServiceName -ceq $GarnetServiceName){throw 'PostgreSQL and Garnet services must be distinct.'}

$origin=$null
if(-not [Uri]::TryCreate($AllowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
   $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or
   $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or $origin.Fragment -ne ''){
    throw 'AllowedOrigin must be a root HTTPS origin.'
}
$listenerParts=$ListenAddress -split ':',2
if($listenerParts.Count -ne 2 -or $listenerParts[1] -notmatch '^\d{1,5}$'){
    throw 'ListenAddress must be 127.0.0.1:port or 0.0.0.0:port.'
}
$listenIP=$null
if(-not [Net.IPAddress]::TryParse($listenerParts[0],[ref]$listenIP) -or
   $listenIP.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork -or
   $listenerParts[0] -notin @('127.0.0.1','0.0.0.0')){
    throw 'ListenAddress must bind IPv4 loopback or all IPv4 interfaces.'
}
$port=[int]$listenerParts[1]
if($port -lt 1 -or $port -gt 65535){throw 'HTTPS listener port is invalid.'}
$originPort=if($origin.IsDefaultPort){443}else{$origin.Port}
if($originPort -ne $port){throw 'AllowedOrigin port must match the ERP HTTPS listener.'}
$isLan=$listenerParts[0] -eq '0.0.0.0'
if($isLan -and [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)){
    throw 'LAN activation requires an explicit private IPv4 CIDR for the firewall.'
}
if(-not $isLan -and -not [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)){
    throw 'A loopback-only listener must not request a LAN firewall rule.'
}

function Test-FirewallRemoteScope([string[]]$Actual,[string]$Expected){
    if($Actual.Count -ne 1){return $false}
    $observed=[string]$Actual[0]
    if([string]::Equals($observed,$Expected,[StringComparison]::OrdinalIgnoreCase)){return $true}
    # Windows Firewall may normalize a single-host /32 to the bare IPv4
    # address when returning its filter. Treat only that exact host as equal.
    $slash=$Expected.LastIndexOf('/')
    if($slash -gt 0 -and $Expected.Substring($slash+1) -ceq '32'){
        return [string]::Equals($observed,$Expected.Substring(0,$slash),[StringComparison]::OrdinalIgnoreCase)
    }
    return $false
}

function ConvertTo-TrustedIPv4Network([string]$value,[bool]$allowLoopback){
    if($value -notmatch '^([^/]+)/([0-9]{1,2})$'){throw 'Firewall scope must be one canonical IPv4 CIDR.'}
    $address=$null
    if(-not [Net.IPAddress]::TryParse($Matches[1],[ref]$address) -or
       $address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork){
        throw 'Firewall scope must be IPv4.'
    }
    $prefix=[int]$Matches[2]
    if($prefix -lt 1 -or $prefix -gt 32){throw 'Firewall CIDR prefix is invalid.'}
    $bytes=$address.GetAddressBytes()
    $isPrivate=($bytes[0] -eq 10) -or
       ($bytes[0] -eq 172 -and $bytes[1] -ge 16 -and $bytes[1] -le 31) -or
       ($bytes[0] -eq 192 -and $bytes[1] -eq 168)
    $isLoop=$bytes[0] -eq 127
    if(-not $isPrivate -and -not ($allowLoopback -and $isLoop)){
        throw 'Firewall CIDR must use RFC1918 space.'
    }
    if($isPrivate -and $prefix -lt 16){throw 'Firewall CIDR must be /16 or narrower.'}
    if($isLoop -and ($prefix -lt 8 -or $prefix -gt 32)){
        throw 'Loopback firewall fixture must be /8 or narrower.'
    }
    for($bit=$prefix;$bit -lt 32;$bit++){
        $byte=[int][Math]::Floor($bit/8)
        $mask=1 -shl (7-($bit%8))
        if(($bytes[$byte] -band $mask) -ne 0){throw 'Firewall CIDR must name a canonical network address.'}
    }
    return ($address.ToString()+'/'+$prefix)
}
$canonicalRemote=''
if($isLan){$canonicalRemote=ConvertTo-TrustedIPv4Network $TrustedRemoteAddress $UnsignedDisposableCIFixture}

$resolvedBundle=[IO.Path]::GetFullPath((Resolve-Path -LiteralPath $BundleDir -ErrorAction Stop).Path).TrimEnd('\')
$programFiles=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
if(-not $resolvedBundle.StartsWith($programFiles+'\',[StringComparison]::OrdinalIgnoreCase)){
    throw 'Only an installed bundle under Program Files can be activated.'
}
$manifestFile=Join-Path $resolvedBundle 'bundle-manifest.json'
if(-not (Test-Path -LiteralPath $manifestFile -PathType Leaf)){throw 'Installed bundle manifest is missing.'}
$manifestHash=(Get-FileHash -LiteralPath $manifestFile -Algorithm SHA256).Hash
if($ApprovedManifestSHA256 -and $manifestHash -cne $ApprovedManifestSHA256.ToUpperInvariant()){
    throw 'Installed bundle does not match its approved manifest digest.'
}
$expectedBinary=Join-Path $resolvedBundle 'esterium-server.exe'
$server=@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
if($server.Count -ne 1 -or $server[0].State -cne 'Stopped' -or
   $server[0].StartMode -cne 'Manual' -or
   $server[0].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or
   -not [string]::Equals(([string]$server[0].PathName).Trim().Trim('"'),$expectedBinary,[StringComparison]::OrdinalIgnoreCase)){
    throw 'ERP service must be the exact stopped LocalService instance created by this fresh install.'
}

function Assert-PhysicalFile([string]$path,[string]$label){
    if(-not [IO.Path]::IsPathRooted($path)){throw "$label path must be absolute."}
    $full=[IO.Path]::GetFullPath($path)
    $item=Get-Item -LiteralPath $full -Force -ErrorAction Stop
    if($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.Length -le 0){
        throw "$label must be a non-empty physical file."
    }
    return $full
}
function Assert-LocalServiceReadOnly([string]$path,[string]$label){
    $acl=Get-Acl -LiteralPath $path
    $found=$false
    $writeMask=[Security.AccessControl.FileSystemRights]::WriteData -bor
       [Security.AccessControl.FileSystemRights]::AppendData -bor
       [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
       [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
       [Security.AccessControl.FileSystemRights]::Delete -bor
       [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
       [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach($entry in $acl.Access){
        try{$sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value}
        catch{throw "Could not verify ACL identity for $label."}
        if($entry.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow){continue}
        $canRead=($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
        if(-not $canRead){continue}
        if($sid -notin @('S-1-5-19','S-1-5-18','S-1-5-32-544')){throw "$label is readable by an unapproved identity."}
        if($sid -eq 'S-1-5-19'){
            if(($entry.FileSystemRights -band $writeMask) -ne 0){throw "LocalService can modify $label."}
            $found=$true
        }
    }
    if(-not $found){throw "LocalService needs explicit read access to $label."}
}
$certPath=Assert-PhysicalFile $TLSCertFile 'TLS certificate'
$keyPath=Assert-PhysicalFile $TLSKeyFile 'TLS private key'
Assert-LocalServiceReadOnly $certPath 'TLS certificate'
Assert-LocalServiceReadOnly $keyPath 'TLS private key'

$programDataEsterium=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
$readinessRoot=[IO.Path]::GetFullPath($ReadinessReceiptDir).TrimEnd('\')
foreach($path in @($certPath,$keyPath)){
    if(-not $path.StartsWith($programDataEsterium,[StringComparison]::OrdinalIgnoreCase)){
        throw 'TLS certificate and private key must stay under protected ProgramData\Esterium.'
    }
}
if(-not $readinessRoot.StartsWith($programDataEsterium,[StringComparison]::OrdinalIgnoreCase)){
    throw 'First-run probe receipt must remain under ProgramData\Esterium.'
}
$receiptFile=Join-Path $readinessRoot 'probe-result.json'
if(-not (Test-Path -LiteralPath $receiptFile -PathType Leaf)){throw 'Successful first-run dependency probe receipt is required.'}
$receiptInfo=Get-Item -LiteralPath $receiptFile -Force
if(($receiptInfo.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Probe receipt cannot be a symbolic link.'}
$receipt=Get-Content -LiteralPath $receiptFile -Raw | ConvertFrom-Json -ErrorAction Stop
$receiptKeys=@($receipt.PSObject.Properties.Name | Sort-Object)
$expectedReceiptKeys=@('automaticStartupApproved','backupRestoreApproved','checkedUtc','fiscalEmissionApproved',
    'garnetConfigSHA256','garnetService','kind','postgresService','postgresqlConfigSHA256',
    'releaseManifestSHA256','schemaVersion','stage') | Sort-Object
$receiptKeyDiff=@(Compare-Object $expectedReceiptKeys $receiptKeys)
if($receiptKeyDiff.Count -ne 0 -or
   $receipt.kind -cne 'ESTERIUM_LOCAL_DEPENDENCY_PROBE' -or [int]$receipt.schemaVersion -ne 1 -or
   $receipt.stage -cne 'dependencies-validated-services-disabled' -or
   $receipt.releaseManifestSHA256 -cne $manifestHash -or
   $receipt.postgresService -cne $PostgresServiceName -or $receipt.garnetService -cne $GarnetServiceName -or
   $receipt.automaticStartupApproved -cne $false -or $receipt.fiscalEmissionApproved -cne $false -or
   $receipt.postgresqlConfigSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
   $receipt.garnetConfigSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$'){
    throw 'Probe receipt is not a valid fresh-install dependency proof.'
}
$checkedUtc=[DateTime]::MinValue
if(-not [DateTime]::TryParse([string]$receipt.checkedUtc,[Globalization.CultureInfo]::InvariantCulture,
   [Globalization.DateTimeStyles]::AssumeUniversal,[ref]$checkedUtc) -or
   [DateTime]::UtcNow.Subtract($checkedUtc.ToUniversalTime()).TotalMinutes -gt 60 -or
   $checkedUtc.ToUniversalTime() -gt [DateTime]::UtcNow.AddMinutes(5)){
    throw 'First-run dependency proof is too old or has an invalid timestamp.'
}
if(-not $UnsignedDisposableCIFixture){
    $pgConfig=Join-Path $PostgresDataDir 'postgresql.conf'
    $garnetConfig=Join-Path $GarnetConfigDir 'garnet.conf'
    foreach($check in @(@($pgConfig,$receipt.postgresqlConfigSHA256),@($garnetConfig,$receipt.garnetConfigSHA256))){
        if(-not (Test-Path -LiteralPath $check[0] -PathType Leaf) -or
           (Get-FileHash -LiteralPath $check[0] -Algorithm SHA256).Hash -cne $check[1]){
            throw 'Database/cache configuration changed after the successful dependency probe.'
        }
    }
}

$serviceEnv=@{}
$registry="HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP"
foreach($entry in @((Get-ItemProperty -LiteralPath $registry -Name Environment -ErrorAction Stop).Environment)){
    $separator=$entry.IndexOf('=')
    if($separator -le 0){throw 'ERP Windows service environment is malformed.'}
    $name=$entry.Substring(0,$separator)
    if($serviceEnv.ContainsKey($name)){throw 'ERP Windows service environment contains duplicate names.'}
    $serviceEnv[$name]=$entry.Substring($separator+1)
}
$expectedOrigin=$origin.GetLeftPart([UriPartial]::Authority)
if($serviceEnv['HTTP_ADDR'] -cne $ListenAddress -or
   $serviceEnv['APP_PUBLIC_URL'] -cne $expectedOrigin -or
   $serviceEnv['CORS_ALLOWED_ORIGINS'] -cne $expectedOrigin -or
   [IO.Path]::GetFullPath($serviceEnv['ESTERIUM_TLS_CERT_FILE']) -cne $certPath -or
   [IO.Path]::GetFullPath($serviceEnv['ESTERIUM_TLS_KEY_FILE']) -cne $keyPath -or
   $serviceEnv['FISCAL_PROVIDER'] -cne 'disabled' -or
   $serviceEnv['NFCE_SEFAZ_PRODUCTION_ENABLED'] -cne 'false' -or
   $serviceEnv['NFCE_SEFAZ_HOMOLOGATION_ENABLED'] -cne 'false'){
    throw 'ERP HTTPS, origin or fiscal-disabled configuration differs from the approved install inputs.'
}

if(-not $Apply){
    Write-Output 'PLAN: verify fresh-install probe, TLS files, HTTPS origin and scoped firewall policy.'
    Write-Output 'PLAN: set PostgreSQL, Garnet and ERP to automatic startup in dependency order.'
    Write-Output 'PLAN: start ERP, require trusted HTTPS readiness, then create the Start Menu shortcut.'
    Write-Output 'PLAN: no services, firewall rules, shortcuts or data changed.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'Administrator elevation is required to activate the local ERP services.'
}
if(-not $UnsignedDisposableCIFixture){
    if($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
       $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$'){
        throw 'Activation requires independent release digest and publisher approval.'
    }
    foreach($scriptPath in @($PSCommandPath,(Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'),
        (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'))){
        $signature=Get-AuthenticodeSignature -LiteralPath $scriptPath
        if($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
           $null -eq $signature.SignerCertificate -or
           $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()){
            throw 'Activation and verification helpers must carry the approved Authenticode signature.'
        }
    }
    if(-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),
       (Join-Path $resolvedBundle 'tools'),[StringComparison]::OrdinalIgnoreCase)){
        throw 'Only the signed helper in the installed release may activate the services.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $resolvedBundle `
       -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
}
if(-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),
   (Join-Path $resolvedBundle 'tools'),[StringComparison]::OrdinalIgnoreCase) -and -not $UnsignedDisposableCIFixture){
    throw 'Activation helper must run from the installed signed bundle.'
}

$dependencyNames=@($PostgresServiceName,$GarnetServiceName)
$oldStates=@{}
foreach($name in @($dependencyNames+'EsteriumERP')){
    $svc=@(Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop)
    if($svc.Count -ne 1){throw "Required service $name is missing or ambiguous."}
    if($name -eq 'EsteriumERP' -and $svc[0].StartMode -cne 'Manual'){
        throw 'ERP must be stopped and manual before first activation.'
    }
    if($name -ne 'EsteriumERP' -and ($svc[0].StartMode -notin @('Disabled','Manual','Auto') -or
       $svc[0].State -notin @('Stopped','Running'))){
        throw 'Dependency service has an unsupported state or startup mode.'
    }
    $oldStates[$name]=@{StartMode=[string]$svc[0].StartMode;State=[string]$svc[0].State}
}
$serviceRegistry="HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP"
$serviceRegistryItem=Get-Item -LiteralPath $serviceRegistry -ErrorAction Stop
$existingDependencies=@()
if($serviceRegistryItem.GetValueNames() -contains 'DependOnService'){
    $existingDependencies=@($serviceRegistryItem.GetValue('DependOnService'))
}
if($existingDependencies.Count -gt 0){throw 'Fresh ERP service must not have pre-existing SCM dependencies.'}
$firewallName='EsteriumERP-HTTPS'
$firewallBefore=@(Get-NetFirewallRule -Name $firewallName -ErrorAction SilentlyContinue)
if($isLan -and $firewallBefore.Count -ne 0){throw 'A firewall rule with the Esterium ERP managed name already exists.'}
$shortcutRoot=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)
if([string]::IsNullOrWhiteSpace($shortcutRoot) -or -not (Test-Path -LiteralPath $shortcutRoot -PathType Container)){
    throw 'The all-users Start Menu folder is unavailable.'
}
$shortcut=Join-Path $shortcutRoot 'Esterium ERP.url'
if(Test-Path -LiteralPath $shortcut){throw 'The Esterium Start Menu shortcut already exists; refusing to overwrite it.'}
$activationPath=[IO.Path]::GetFullPath($ActivationReceiptDir).TrimEnd('\')
if(-not $activationPath.StartsWith($programDataEsterium,[StringComparison]::OrdinalIgnoreCase) -or
   (Test-Path -LiteralPath $activationPath)){
    throw 'Activation receipt directory must be new under ProgramData\Esterium.'
}
$profiles=@(Get-NetFirewallProfile -PolicyStore ActiveStore -Profile Domain,Private,Public)
if($isLan -and ((Get-Service -Name 'MpsSvc' -ErrorAction Stop).Status -ne 'Running' -or
   $profiles.Count -ne 3 -or @($profiles | Where-Object { -not $_.Enabled }).Count -gt 0)){
    throw 'Windows Firewall service and all profiles must be enabled for LAN activation.'
}

$ruleCreated=$false
$shortcutCreated=$false
$receiptCreated=$false
$success=$false
$staging=''
$sc=Join-Path $env:SystemRoot 'System32\sc.exe'
try{
    foreach($name in $dependencyNames){Set-Service -Name $name -StartupType Automatic -ErrorAction Stop}
    $depResult=& $sc config EsteriumERP 'depend=' (($PostgresServiceName+'/'+$GarnetServiceName)) 2>&1
    if($LASTEXITCODE -ne 0){throw "Could not configure ERP service dependencies: $depResult"}
    Set-Service -Name 'EsteriumERP' -StartupType Automatic -ErrorAction Stop
    if($isLan){
        New-NetFirewallRule -Name $firewallName -DisplayName 'Esterium ERP HTTPS' `
            -Description 'Esterium install; private LAN CIDR only.' -Group 'Esterium ERP' `
            -Direction Inbound -Action Allow -Enabled True -Profile Domain,Private `
            -Protocol TCP -LocalPort $port -RemoteAddress $canonicalRemote -Program $expectedBinary | Out-Null
        $ruleCreated=$true
    }
    foreach($name in $dependencyNames){
        $current=Get-Service -Name $name -ErrorAction Stop
        if($current.Status -ne 'Running'){
            Start-Service -Name $name -ErrorAction Stop
            $current.WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(90))
        }
    }
    Start-Service -Name 'EsteriumERP' -ErrorAction Stop
    (Get-Service -Name 'EsteriumERP' -ErrorAction Stop).WaitForStatus(
        [ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $healthUri=$expectedOrigin.TrimEnd('/')+'/health/ready'
    $ready=$false
    for($attempt=0;$attempt -lt 90;$attempt++){
        try{
            $health=Invoke-RestMethod -Uri $healthUri -Method Get -UseBasicParsing -TimeoutSec 5
            if($health.status -ceq 'ready'){$ready=$true;break}
        }catch{}
        Start-Sleep -Seconds 1
    }
    if(-not $ready){throw 'ERP did not become ready over trusted HTTPS within 90 seconds.'}
    if($isLan){
        $rule=@(Get-NetFirewallRule -Name $firewallName -PolicyStore ActiveStore -ErrorAction Stop)
        if($rule.Count -ne 1 -or $rule[0].Enabled -ne 'True' -or $rule[0].Action -ne 'Allow' -or
           $rule[0].Direction -ne 'Inbound' -or [string]$rule[0].Profile -notmatch 'Domain' -or
           [string]$rule[0].Profile -notmatch 'Private'){
            throw 'Windows Firewall did not retain the restricted inbound HTTPS rule.'
        }
        $portFilter=$rule[0] | Get-NetFirewallPortFilter
        $addressFilter=$rule[0] | Get-NetFirewallAddressFilter
        $applicationFilter=$rule[0] | Get-NetFirewallApplicationFilter
        $protocolMatches=[string]$portFilter.Protocol -in @('TCP','6')
        $portValues=@($portFilter.LocalPort)
        $portMatches=$portValues.Count -eq 1 -and [string]$portValues[0] -eq [string]$port
        $scopeMatches=Test-FirewallRemoteScope -Actual @($addressFilter.RemoteAddress) -Expected $canonicalRemote
        $programMatches=[string]::Equals([string]$applicationFilter.Program,$expectedBinary,[StringComparison]::OrdinalIgnoreCase)
        if(-not $protocolMatches -or -not $portMatches -or -not $scopeMatches -or -not $programMatches){
            throw ('Windows Firewall rule verification failed (TCP={0}; port={1}; CIDR={2}; executable={3}).' -f
                $protocolMatches,$portMatches,$scopeMatches,$programMatches)
        }
    }
    $urlContent="[InternetShortcut]`r`nURL=$expectedOrigin`r`n"
    [IO.File]::WriteAllText($shortcut,$urlContent,[Text.UTF8Encoding]::new($false))
    $shortcutCreated=$true
    $receiptParent=Split-Path -Parent $activationPath
    [IO.Directory]::CreateDirectory($receiptParent) | Out-Null
    $staging=Join-Path $receiptParent ('.esterium-activation-'+[Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($staging) | Out-Null
    & icacls.exe $staging '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not protect the activation receipt directory.'}
    $meta=[ordered]@{
        kind='ESTERIUM_LOCAL_INSTALL_ACTIVATION'
        schemaVersion=1
        checkedUtc=[DateTime]::UtcNow.ToString('o')
        releaseManifestSHA256=$manifestHash
        allowedOrigin=$expectedOrigin
        listenAddress=$ListenAddress
        trustedRemoteAddress=$canonicalRemote
        httpsPort=$port
        firewallRule=if($isLan){$firewallName}else{''}
        shortcut=$shortcut
        postgresService=$PostgresServiceName
        garnetService=$GarnetServiceName
        erpService='EsteriumERP'
        automaticStartupApproved=$true
        backupRestoreApproved=$false
        fiscalEmissionApproved=$false
    }
    $receiptPath=Join-Path $staging 'installation-ready.json'
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $meta -Depth 3)+[Environment]::NewLine)
    try{
        $stream=[IO.File]::Open($receiptPath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try{$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true)}finally{$stream.Dispose()}
    }finally{[Array]::Clear($bytes,0,$bytes.Length)}
    & icacls.exe $receiptPath '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if($LASTEXITCODE -ne 0){throw 'Could not protect the installation activation receipt.'}
    Move-Item -LiteralPath $staging -Destination $activationPath -ErrorAction Stop
    $receiptCreated=$true
    $success=$true
    Write-Output 'PASS: PostgreSQL, Garnet and ERP are automatic; ERP readiness passed over trusted HTTPS.'
    if($isLan){Write-Output "PASS: HTTPS firewall access is limited to $canonicalRemote on Domain/Private profiles."}
    Write-Output 'PASS: all-users Start Menu shortcut and protected installation receipt created.'
    Write-Output 'FISCAL REMAINS DISABLED; no backup-restore or SEFAZ approval was recorded.'
}finally{
    if(-not $success){
        $rollbackFailures=@()
        try{
            $current=Get-Service -Name 'EsteriumERP' -ErrorAction SilentlyContinue
            if($null -ne $current -and $current.Status -ne 'Stopped'){
                Stop-Service -Name 'EsteriumERP' -Force -ErrorAction Stop
                (Get-Service -Name 'EsteriumERP').WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
            }
            Set-Service -Name 'EsteriumERP' -StartupType Manual -ErrorAction Stop
            $currentRegistry=Get-Item -LiteralPath $serviceRegistry -ErrorAction Stop
            if($currentRegistry.GetValueNames() -contains 'DependOnService'){
                Remove-ItemProperty -LiteralPath $serviceRegistry -Name DependOnService -ErrorAction Stop
            }
        }catch{$rollbackFailures+='ERP service'}
        foreach($name in $dependencyNames){
            try{
                if($oldStates[$name].State -eq 'Stopped'){
                    $current=Get-Service -Name $name -ErrorAction Stop
                    if($current.Status -ne 'Stopped'){
                        Stop-Service -Name $name -Force -ErrorAction Stop
                        (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
                    }
                }
                $type=switch($oldStates[$name].StartMode){'Auto'{'Automatic'}'Manual'{'Manual'}default{'Disabled'}}
                Set-Service -Name $name -StartupType $type -ErrorAction Stop
                if($oldStates[$name].State -eq 'Running' -and (Get-Service -Name $name).Status -ne 'Running'){
                    Start-Service -Name $name -ErrorAction Stop
                    (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(90))
                }
            }catch{$rollbackFailures+=$name}
        }
        if($ruleCreated){try{Remove-NetFirewallRule -Name $firewallName -ErrorAction Stop}catch{$rollbackFailures+='firewall rule'}}
        if($shortcutCreated){try{Remove-Item -LiteralPath $shortcut -Force -ErrorAction Stop}catch{$rollbackFailures+='Start Menu shortcut'}}
        if($receiptCreated -and (Test-Path -LiteralPath $activationPath)){
            try{Remove-Item -LiteralPath $activationPath -Recurse -Force -ErrorAction Stop}catch{$rollbackFailures+='activation receipt'}
        }
        if($staging -and (Test-Path -LiteralPath $staging)){
            try{Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction Stop}catch{$rollbackFailures+='temporary activation receipt'}
        }
        if($rollbackFailures.Count -gt 0){throw ('Activation failed and rollback needs manual review: '+($rollbackFailures -join ', '))}
    }
}

