#requires -Version 5.1
<#
  Assisted repair for an existing, signed local installation.
  Repairs only SCM startup/dependency policy, the owned HTTPS firewall rule,
  and the all-users shortcut. Never changes application, SQL or secret files.
#>
[CmdletBinding()]
param(
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$ActivationReceiptDir=(Join-Path $env:ProgramData 'Esterium\Installation'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$GarnetServiceName='EsteriumGarnet',
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$ConfirmRepair,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
function Test-ExactFirewallRemoteScope([string[]]$Actual,[string]$Expected){
    if($Actual.Count -ne 1){return $false}
    $observed=[string]$Actual[0]
    if([string]::Equals($observed,$Expected,[StringComparison]::OrdinalIgnoreCase)){return $true}
    $slash=$Expected.LastIndexOf('/')
    if($slash -gt 0 -and $Expected.Substring($slash+1) -ceq '32'){
        return [string]::Equals($observed,$Expected.Substring(0,$slash),[StringComparison]::OrdinalIgnoreCase)
    }
    return $false
}
if($env:OS -ne 'Windows_NT'){throw 'Esterium repair requires Windows.'}
if($UnsignedDisposableCIFixture -and (
   $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
   $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1' -or $env:ESTERIUM_REPAIR_DISPOSABLE_CI -ne '1')){
    throw 'Unsigned repair is restricted to the disposable GitHub-hosted Windows lab.'
}
if($Apply -and -not $ConfirmRepair){throw 'Explicit -ConfirmRepair is required before service repair.'}
if($PostgresServiceName -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or
   $GarnetServiceName -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or
   $PostgresServiceName -ceq $GarnetServiceName){throw 'Invalid dependency service identities.'}
if(-not [IO.Path]::IsPathRooted($InstallDir) -or -not [IO.Path]::IsPathRooted($ActivationReceiptDir)){
    throw 'Install and receipt paths must be absolute.'
}
$install=[IO.Path]::GetFullPath((Resolve-Path -LiteralPath $InstallDir -ErrorAction Stop).Path).TrimEnd('\')
$programFiles=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
if(-not $install.StartsWith($programFiles+'\',[StringComparison]::OrdinalIgnoreCase)){
    throw 'Repair target must be a dedicated Program Files installation.'
}
$installInfo=Get-Item -LiteralPath $install -Force -ErrorAction Stop
if(-not $installInfo.PSIsContainer -or ($installInfo.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
    throw 'Repair target must be a physical directory.'
}
$manifest=Join-Path $install 'bundle-manifest.json'
$manifestHash=(Get-FileHash -LiteralPath $manifest -Algorithm SHA256 -ErrorAction Stop).Hash
if($ApprovedManifestSHA256 -and $manifestHash -cne $ApprovedManifestSHA256.ToUpperInvariant()){
    throw 'Installed release manifest differs from the independently approved digest.'
}
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $install | Out-Null
$bundleRoot=[IO.Path]::GetFullPath($ActivationReceiptDir).TrimEnd('\')
$programDataRoot=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
if(-not $bundleRoot.StartsWith($programDataRoot,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Activation receipt must remain under ProgramData\Esterium.'
}
$receiptPath=Join-Path $bundleRoot 'installation-ready.json'
$receiptInfo=Get-Item -LiteralPath $receiptPath -Force -ErrorAction Stop
if($receiptInfo.PSIsContainer -or ($receiptInfo.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
    throw 'Activation receipt must be a physical file.'
}
foreach($metadataPath in @($bundleRoot,$receiptPath)){
    $writeMask=[Security.AccessControl.FileSystemRights]::WriteData -bor
        [Security.AccessControl.FileSystemRights]::AppendData -bor
        [Security.AccessControl.FileSystemRights]::WriteAttributes -bor
        [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes -bor
        [Security.AccessControl.FileSystemRights]::Delete -bor
        [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
        [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach($ace in (Get-Acl -LiteralPath $metadataPath).Access){
        if($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow){continue}
        try{$sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value}
        catch{throw 'Could not verify installation receipt ACL.'}
        if($sid -notin @('S-1-5-18','S-1-5-32-544')){throw 'Installation receipt is accessible to an unapproved identity.'}
    }
}
$receipt=Get-Content -LiteralPath $receiptPath -Raw | ConvertFrom-Json -ErrorAction Stop
$requiredReceipt=@('automaticStartupApproved','allowedOrigin','backupRestoreApproved','checkedUtc',
    'erpService','fiscalEmissionApproved','firewallRule','garnetService','httpsPort','kind',
    'listenAddress','postgresService','releaseManifestSHA256','schemaVersion','shortcut','trustedRemoteAddress') | Sort-Object
$actualReceipt=@($receipt.PSObject.Properties.Name | Sort-Object)
if(@(Compare-Object $requiredReceipt $actualReceipt).Count -ne 0 -or
   $receipt.kind -cne 'ESTERIUM_LOCAL_INSTALL_ACTIVATION' -or [int]$receipt.schemaVersion -ne 1 -or
   $receipt.releaseManifestSHA256 -cne $manifestHash -or $receipt.erpService -cne 'EsteriumERP' -or
   $receipt.postgresService -cne $PostgresServiceName -or $receipt.garnetService -cne $GarnetServiceName -or
   $receipt.automaticStartupApproved -cne $true -or $receipt.fiscalEmissionApproved -cne $false -or
   $receipt.backupRestoreApproved -cne $false){
    throw 'Installation receipt does not describe this verified, non-fiscal release.'
}
$origin=$null
if(-not [Uri]::TryCreate([string]$receipt.allowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
   $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or $origin.AbsolutePath -ne '/' -or
   $origin.Query -ne '' -or $origin.Fragment -ne ''){throw 'Installation receipt has an invalid HTTPS origin.'}
$listenerParts=([string]$receipt.listenAddress) -split ':',2
if($listenerParts.Count -ne 2 -or $listenerParts[1] -notmatch '^\d{1,5}$' -or
   $listenerParts[0] -notin @('127.0.0.1','0.0.0.0') -or [int]$listenerParts[1] -ne [int]$receipt.httpsPort){
    throw 'Installation receipt listener is inconsistent.'
}
$isLan=$listenerParts[0] -eq '0.0.0.0'
$originPort=if($origin.IsDefaultPort){443}else{$origin.Port}
if($originPort -ne [int]$receipt.httpsPort -or
   ($isLan -and $origin.Host -in @('localhost','127.0.0.1','::1')) -or
   (-not $isLan -and $origin.Host -in @('localhost','127.0.0.1','::1') -and $listenerParts[0] -ne '127.0.0.1')){
    throw 'Installation receipt HTTPS host/port does not match its listener mode.'
}
$ruleName='EsteriumERP-HTTPS'
if(($isLan -and $receipt.firewallRule -cne $ruleName) -or
   (-not $isLan -and ($receipt.firewallRule -ne '' -or $receipt.trustedRemoteAddress -ne ''))){
    throw 'Installation receipt firewall scope is inconsistent with its listener.'
}
$expectedBinary=Join-Path $install 'esterium-server.exe'
$service=@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
if($service.Count -ne 1 -or
   -not [string]::Equals(([string]$service[0].PathName).Trim().Trim('"'),$expectedBinary,[StringComparison]::OrdinalIgnoreCase) -or
   $service[0].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE')){
    throw 'Repair requires the exact EsteriumERP LocalService registration; unrelated services are never changed.'
}
if($service[0].State -cne 'Stopped'){throw 'Stop and reconcile all checkout/POS activity before repair.'}
$serviceRegistry='HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP'
$environment=@{}
foreach($entry in @((Get-ItemProperty -LiteralPath $serviceRegistry -Name Environment -ErrorAction Stop).Environment)){
    $separator=$entry.IndexOf('=')
    if($separator -le 0){throw 'ERP service environment is malformed.'}
    $name=$entry.Substring(0,$separator)
    if($environment.ContainsKey($name)){throw 'ERP service environment contains duplicate variables.'}
    $environment[$name]=$entry.Substring($separator+1)
}
$expectedOrigin=$origin.GetLeftPart([UriPartial]::Authority)
foreach($key in @('DATABASE_URL_FILE','REDIS_URL_FILE','REDIS_CA_CERT_FILE','JWT_SECRET_FILE',
    'METRICS_BEARER_TOKEN_FILE','ESTERIUM_TLS_CERT_FILE','ESTERIUM_TLS_KEY_FILE')){
    if([string]::IsNullOrWhiteSpace($environment[$key]) -or
       -not (Test-Path -LiteralPath $environment[$key] -PathType Leaf)){
        throw "Protected ERP input is missing: $key. Repair will not regenerate or overwrite credentials."
    }
    $filePath=[IO.Path]::GetFullPath($environment[$key])
    if(-not $filePath.StartsWith($programDataRoot,[StringComparison]::OrdinalIgnoreCase)){
        throw "ERP input is outside protected ProgramData: $key."
    }
    $localServiceRead=$false
    foreach($ace in (Get-Acl -LiteralPath $filePath).Access){
        if($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or
           ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -eq 0){continue}
        try{$sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value}
        catch{throw "Could not verify protected ERP input ACL: $key."}
        if($sid -notin @('S-1-5-18','S-1-5-32-544','S-1-5-19')){throw "ERP input has an unapproved reader: $key."}
        if($sid -eq 'S-1-5-19'){$localServiceRead=$true}
    }
    if(-not $localServiceRead){throw "LocalService cannot read protected ERP input: $key."}
}
if($environment['HTTP_ADDR'] -cne [string]$receipt.listenAddress -or
   $environment['APP_PUBLIC_URL'] -cne $expectedOrigin -or
   $environment['CORS_ALLOWED_ORIGINS'] -cne $expectedOrigin -or
   $environment['FISCAL_PROVIDER'] -cne 'disabled' -or
   $environment['NFCE_SEFAZ_PRODUCTION_ENABLED'] -cne 'false' -or
   $environment['NFCE_SEFAZ_HOMOLOGATION_ENABLED'] -cne 'false'){
    throw 'ERP service config differs from its activation receipt or enables fiscal transmission.'
}
$deps=@($PostgresServiceName,$GarnetServiceName)
foreach($name in $deps){
    $dependency=@(Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop)
    if($dependency.Count -ne 1 -or $dependency[0].State -notin @('Stopped','Running')){
        throw "Required dependency service $name is missing or ambiguous."
    }
}
$shortcutRoot=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)
$shortcut=Join-Path $shortcutRoot 'Esterium ERP.url'
if(-not [string]::Equals([IO.Path]::GetFullPath([string]$receipt.shortcut),$shortcut,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Installation receipt shortcut path does not match the managed Start Menu location.'
}
$existingShortcut=Test-Path -LiteralPath $shortcut
if($existingShortcut -and ([IO.File]::ReadAllText($shortcut) -notmatch ('(?m)^URL='+[Regex]::Escape($expectedOrigin)+'\r?$'))){
    throw 'Existing Start Menu shortcut was changed by another owner; manual review required.'
}
$rule=@(Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue)
if($rule.Count -gt 1){throw 'Multiple firewall rules use the managed Esterium name.'}
if($rule.Count -eq 1){
    $ports=$rule[0] | Get-NetFirewallPortFilter
    $addresses=$rule[0] | Get-NetFirewallAddressFilter
    $app=$rule[0] | Get-NetFirewallApplicationFilter
    if($rule[0].Enabled -ne 'True' -or $rule[0].Action -ne 'Allow' -or
       $rule[0].Direction -ne 'Inbound' -or [string]$rule[0].Profile -notmatch 'Domain' -or
       [string]$rule[0].Profile -notmatch 'Private' -or [string]$ports.Protocol -ne 'TCP' -or
       [string]$ports.LocalPort -ne [string]$receipt.httpsPort -or
       -not (Test-ExactFirewallRemoteScope -Actual @($addresses.RemoteAddress) -Expected ([string]$receipt.trustedRemoteAddress)) -or
       -not [string]::Equals([string]$app.Program,$expectedBinary,[StringComparison]::OrdinalIgnoreCase)){
        throw 'Existing managed firewall rule was changed; it will not be overwritten.'
    }
}elseif($isLan){
    $remote=[string]$receipt.trustedRemoteAddress
    $parts=$remote -split '/',2;$remoteIP=$null
    if($parts.Count -ne 2 -or $parts[1] -notmatch '^\d{1,2}$' -or
       -not [Net.IPAddress]::TryParse($parts[0],[ref]$remoteIP) -or
       $remoteIP.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork){throw 'Activation receipt LAN scope is invalid.'}
    $prefix=[int]$parts[1];$octets=$remoteIP.GetAddressBytes()
    $private=($octets[0] -eq 10) -or
       ($octets[0] -eq 172 -and $octets[1] -ge 16 -and $octets[1] -le 31) -or
       ($octets[0] -eq 192 -and $octets[1] -eq 168)
    $privateCIDR=$private -and $prefix -ge 16 -and $prefix -le 32
    $loopbackFixture=$UnsignedDisposableCIFixture -and
        $octets[0] -eq 127 -and $prefix -eq 32
    if(-not $privateCIDR -and -not $loopbackFixture){
        throw 'Activation receipt LAN scope must be a private IPv4 /16-or-narrower CIDR (CI may use only 127.0.0.1/32).'
    }
    for($bit=$prefix;$bit -lt 32;$bit++){
        $byte=[int][Math]::Floor($bit/8);$mask=1 -shl (7-($bit%8))
        if(($octets[$byte] -band $mask) -ne 0){throw 'Activation receipt LAN CIDR is not canonical.'}
    }
}
$existingDependencies=@()
$registryItem=Get-Item -LiteralPath $serviceRegistry -ErrorAction Stop
if($registryItem.GetValueNames() -contains 'DependOnService'){$existingDependencies=@($registryItem.GetValue('DependOnService'))}
if($existingDependencies.Count -gt 0 -and
   (@($existingDependencies | Where-Object {$_ -notin $deps}).Count -gt 0 -or
    @($deps | Where-Object {$_ -notin $existingDependencies}).Count -gt 0)){
    throw 'ERP SCM dependencies differ from the installed PostgreSQL/Garnet pair; manual review required.'
}
if(-not $Apply){
    Write-Output 'PLAN: verify signed files, protected credentials, stopped service, dependencies and activation receipt.'
    Write-Output 'PLAN: repair only ERP SCM startup/dependency metadata, its missing owned firewall rule/shortcut, then HTTPS health.'
    Write-Output 'PLAN: application files, SQL, credentials, TLS materials and backups stay unchanged.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'Administrator elevation is required for repair.'
}
if(-not $UnsignedDisposableCIFixture){
    if($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$'){
        throw 'Repair requires independent release digest and publisher approval.'
    }
    foreach($scriptPath in @($PSCommandPath,(Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'),
        (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'))){
        $signature=Get-AuthenticodeSignature -LiteralPath $scriptPath
        if($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
           $null -eq $signature.SignerCertificate -or
           $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()){
            throw 'Repair entrypoints are not signed by the independently approved publisher.'
        }
    }
    if(-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),(Join-Path $install 'tools'),[StringComparison]::OrdinalIgnoreCase)){
        throw 'Repair must run from the signed installed bundle.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $install `
        -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
}elseif($ApprovedManifestSHA256 -and $manifestHash -cne $ApprovedManifestSHA256.ToUpperInvariant()){
    throw 'Disposable repair fixture manifest digest mismatch.'
}
if($isLan -and $rule.Count -eq 0){
    $profiles=@(Get-NetFirewallProfile -PolicyStore ActiveStore -Profile Domain,Private,Public)
    if((Get-Service -Name 'MpsSvc' -ErrorAction Stop).Status -ne 'Running' -or
       $profiles.Count -ne 3 -or @($profiles | Where-Object {-not $_.Enabled}).Count -gt 0){
        throw 'Windows Firewall must be enabled before LAN repair.'
    }
}
$oldStates=@{}
foreach($name in @($deps+'EsteriumERP')){
    $current=@(Get-CimInstance Win32_Service -Filter "Name='$name'" -ErrorAction Stop)[0]
    $oldStates[$name]=@{StartMode=[string]$current.StartMode;State=[string]$current.State}
}
$ruleCreated=$false;$shortcutCreated=$false;$dependenciesCreated=$false;$success=$false
$sc=Join-Path $env:SystemRoot 'System32\sc.exe'
try{
    foreach($name in $deps){Set-Service -Name $name -StartupType Automatic -ErrorAction Stop}
    $dependencyResult=& $sc config EsteriumERP 'depend=' (($PostgresServiceName+'/'+$GarnetServiceName)) 2>&1
    if($LASTEXITCODE -ne 0){throw "Could not restore ERP service dependency metadata: $dependencyResult"}
    $dependenciesCreated=($existingDependencies.Count -eq 0)
    Set-Service -Name EsteriumERP -StartupType Automatic -ErrorAction Stop
    $failure=& $sc failure EsteriumERP 'reset=' '86400' 'actions=' 'restart/5000/restart/30000/none/0' 2>&1
    if($LASTEXITCODE -ne 0){throw "Could not restore finite ERP service recovery: $failure"}
    $flag=& $sc failureflag EsteriumERP '1' 2>&1
    if($LASTEXITCODE -ne 0){throw "Could not restore ERP nonzero-exit recovery: $flag"}
    if($isLan -and $rule.Count -eq 0){
        New-NetFirewallRule -Name $ruleName -DisplayName 'Esterium ERP HTTPS' `
            -Description 'Esterium managed repair; private LAN CIDR only.' -Group 'Esterium ERP' `
            -Direction Inbound -Action Allow -Enabled True -Profile Domain,Private `
            -Protocol TCP -LocalPort ([int]$receipt.httpsPort) -RemoteAddress ([string]$receipt.trustedRemoteAddress) `
            -Program $expectedBinary | Out-Null
        $ruleCreated=$true
    }
    if(-not $existingShortcut){
        [IO.File]::WriteAllText($shortcut,"[InternetShortcut]`r`nURL=$expectedOrigin`r`n",[Text.UTF8Encoding]::new($false))
        $shortcutCreated=$true
    }
    foreach($name in $deps){
        if((Get-Service -Name $name -ErrorAction Stop).Status -ne 'Running'){
            Start-Service -Name $name -ErrorAction Stop
            (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(90))
        }
    }
    Start-Service -Name EsteriumERP -ErrorAction Stop
    (Get-Service -Name EsteriumERP -ErrorAction Stop).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $healthUri=$expectedOrigin.TrimEnd('/')+'/health/ready';$ready=$false
    for($attempt=0;$attempt -lt 90;$attempt++){
        try{$health=Invoke-RestMethod -Uri $healthUri -Method Get -UseBasicParsing -TimeoutSec 5;if($health.status -ceq 'ready'){$ready=$true;break}}catch{}
        Start-Sleep -Seconds 1
    }
    if(-not $ready){throw 'ERP did not return trusted HTTPS readiness after repair.'}
    $success=$true
    Write-Output 'PASS: SCM dependencies/recovery, owned HTTPS firewall rule, Start Menu shortcut and HTTPS readiness repaired.'
    Write-Output 'PASS: application files, database, secrets, TLS materials and backup files were not modified.'
    Write-Output 'FISCAL remains disabled; backup restore and external production approvals remain false.'
}finally{
    if(-not $success){
        try{$current=Get-Service -Name EsteriumERP -ErrorAction SilentlyContinue;if($null -ne $current -and $current.Status -ne 'Stopped'){Stop-Service -Name EsteriumERP -Force -ErrorAction Stop;(Get-Service -Name EsteriumERP).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))}}catch{Write-Warning 'Could not stop the ERP service after unsuccessful repair.'}
        if($ruleCreated){try{Remove-NetFirewallRule -Name $ruleName -ErrorAction Stop}catch{Write-Warning 'Could not remove the firewall rule created by failed repair.'}}
        if($shortcutCreated){try{Remove-Item -LiteralPath $shortcut -Force -ErrorAction Stop}catch{Write-Warning 'Could not remove the shortcut created by failed repair.'}}
        foreach($name in @($deps+'EsteriumERP')){
            try{
                $type=switch($oldStates[$name].StartMode){'Auto'{'Automatic'}'Manual'{'Manual'}default{'Disabled'}}
                Set-Service -Name $name -StartupType $type -ErrorAction Stop
                if($oldStates[$name].State -eq 'Stopped' -and (Get-Service -Name $name).Status -ne 'Stopped'){
                    Stop-Service -Name $name -Force -ErrorAction Stop
                    (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(60))
                }elseif($oldStates[$name].State -eq 'Running' -and (Get-Service -Name $name).Status -ne 'Running'){
                    Start-Service -Name $name -ErrorAction Stop
                    (Get-Service -Name $name).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(90))
                }
            }catch{Write-Warning "Could not restore prior service state for $name."}
        }
        if($dependenciesCreated){
            $currentRegistry=Get-Item -LiteralPath $serviceRegistry -ErrorAction SilentlyContinue
            if($null -ne $currentRegistry){try{if($currentRegistry.GetValueNames() -contains 'DependOnService'){Remove-ItemProperty -LiteralPath $serviceRegistry -Name DependOnService -ErrorAction Stop}}catch{Write-Warning 'Could not remove partially repaired ERP dependencies.'}}
        }
    }
}

