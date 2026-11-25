#requires -Version 5.1
<#
  VirtualBox laboratory only. Continue after first-admin commit with the
  original signed installed probe and activation helpers. Never creates a
  receipt itself, reinstalls dependencies, edits service environment or
  enables fiscal traffic. PLAN does not start services.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$OriginalManifestSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{40}$')][string]$OriginalPublisher,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$DiagnosticSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{40}$')][string]$DiagnosticPublisher,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$InspectorScriptSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$TLSCertSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$TLSKeySHA256,
    [switch]$Activate
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
function Assert-Physical([string]$Path){
    $full=[IO.Path]::GetFullPath($Path)
    if($full.StartsWith('\\')){throw 'Network paths prohibited.'}
    $cursor=$full
    while($cursor){
        if((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Reparse point prohibited.'}
        $parent=Split-Path -Parent $cursor
        if($parent -eq $cursor){break}
        $cursor=$parent
    }
    return $full
}
function Assert-Signed([string]$Path,[string]$Publisher){
    $sig=Get-AuthenticodeSignature -LiteralPath $Path
    if($sig.Status -ne 'Valid' -or $null -eq $sig.SignerCertificate -or $sig.SignerCertificate.Thumbprint -ine $Publisher){throw 'Approved signature required.'}
}
function Assert-Protected([string]$Path){
    $mask=[Security.AccessControl.FileSystemRights]::Write -bor [Security.AccessControl.FileSystemRights]::Delete -bor [Security.AccessControl.FileSystemRights]::ChangePermissions -bor [Security.AccessControl.FileSystemRights]::TakeOwnership
    foreach($ace in (Get-Acl -LiteralPath $Path).Access){
        if($ace.AccessControlType -eq 'Allow' -and ($ace.FileSystemRights -band $mask) -and $ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin @('S-1-5-18','S-1-5-32-544')){throw 'Unapproved writer.'}
    }
}
function Get-ExactService([string]$Name){
    $s=@(Get-CimInstance Win32_Service -Filter "Name='$Name'")
    if($s.Count -ne 1){throw 'Service identity ambiguous.'}
    return $s[0]
}
function Assert-ServiceState([string]$Name,[string]$State,[string]$Mode){
    $s=Get-ExactService $Name
    if($s.State -cne $State -or $s.StartMode -cne $Mode -or ($State -eq 'Stopped' -and [int]$s.ProcessId -ne 0) -or ($State -eq 'Running' -and [int]$s.ProcessId -le 0)){throw 'Unexpected service state.'}
}
function Assert-FreePorts{
    if(@(Get-NetTCPConnection -State Listen -ErrorAction Stop | Where-Object LocalPort -in @(5432,6379,8443)).Count){throw 'Managed ports occupied.'}
}
$names=@('EsteriumERP','EsteriumGarnet','EsteriumPostgreSQL16')
if($env:OS -ne 'Windows_NT'){throw 'Windows required.'}
$machine=Get-CimInstance Win32_ComputerSystem
if(($machine.Manufacturer+' '+$machine.Model) -notmatch '(?i)VirtualBox|innotek'){throw 'VirtualBox only.'}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Elevation required.'}
$dir=Assert-Physical (Join-Path $env:ProgramFiles 'EsteriumRecoveryDiagnostics')
if([IO.Path]::GetFullPath($PSScriptRoot) -ine $dir){throw 'Dedicated protected directory required.'}
$self=Assert-Physical $PSCommandPath
$inspector=Assert-Physical (Join-Path $dir 'inspect-esterium-partial-install.ps1')
foreach($path in @($dir,$self,$inspector)){Assert-Protected $path}
foreach($path in @($self,$inspector)){Assert-Signed $path $DiagnosticPublisher}
if((Get-FileHash -LiteralPath $inspector -Algorithm SHA256).Hash -ine $InspectorScriptSHA256){throw 'Inspector script digest mismatch.'}
$inspectArgs=@{OriginalManifestSHA256=$OriginalManifestSHA256;OriginalPublisher=$OriginalPublisher;DiagnosticSHA256=$DiagnosticSHA256;DiagnosticPublisher=$DiagnosticPublisher}
& $inspector @inspectArgs | Out-Null
$bundle=Assert-Physical (Join-Path $env:ProgramFiles 'EsteriumERP')
$probe=Assert-Physical (Join-Path $bundle 'tools\probe-esterium-local-first-run.ps1')
$activation=Assert-Physical (Join-Path $bundle 'tools\activate-esterium-local-first-install.ps1')
foreach($path in @($probe,$activation)){Assert-Signed $path $OriginalPublisher}
$root=Assert-Physical (Join-Path $env:ProgramData 'Esterium')
$cert=Assert-Physical (Join-Path $root 'Secrets\https-server-chain.pem')
$key=Assert-Physical (Join-Path $root 'Secrets\https-server-key.pem')
if((Get-FileHash -LiteralPath $cert).Hash -ine $TLSCertSHA256 -or (Get-FileHash -LiteralPath $key).Hash -ine $TLSKeySHA256){throw 'Original staged TLS material mismatch.'}
# Bind to the exact lab origin/CIDR approved for this VM. Do not infer an
# expanded network scope or rewrite existing service configuration.
$origin='https://10.0.2.15:8443'
$listen='0.0.0.0:8443'
$cidr='10.0.2.15/32'
if(@(Get-NetIPAddress -AddressFamily IPv4 -IPAddress '10.0.2.15' -ErrorAction Stop).Count -ne 1){throw 'Original guest IP absent.'}
$envMap=@{}
foreach($entry in @((Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP' -Name Environment).Environment)){
    $i=$entry.IndexOf('=')
    if($i -le 0){throw 'Malformed ERP service configuration.'}
    $k=$entry.Substring(0,$i)
    if($envMap.ContainsKey($k)){throw 'Duplicate service setting.'}
    $envMap[$k]=$entry.Substring($i+1)
}
if($envMap['HTTP_ADDR'] -cne $listen -or $envMap['APP_PUBLIC_URL'] -cne $origin -or $envMap['CORS_ALLOWED_ORIGINS'] -cne $origin -or
    $envMap['ESTERIUM_TLS_CERT_FILE'] -ine $cert -or $envMap['ESTERIUM_TLS_KEY_FILE'] -ine $key -or
    $envMap['FISCAL_PROVIDER'] -cne 'disabled' -or $envMap['NFCE_SEFAZ_PRODUCTION_ENABLED'] -cne 'false' -or $envMap['NFCE_SEFAZ_HOMOLOGATION_ENABLED'] -cne 'false'){throw 'Original lab settings mismatch.'}
$erp=Get-ExactService 'EsteriumERP'
if($erp.StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or ([string]$erp.PathName).Trim().Trim('"') -ine (Join-Path $bundle 'esterium-server.exe')){throw 'ERP service identity mismatch.'}
foreach($name in $names){Assert-ServiceState $name 'Stopped' 'Disabled'}
Assert-FreePorts
$readiness=Join-Path $root 'FirstRunReadiness'
$installed=Join-Path $root 'Installation'
if((Test-Path -LiteralPath $readiness) -or (Test-Path -LiteralPath $installed)){throw 'Prior receipts require manual review.'}
if(@(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction SilentlyContinue).Count){throw 'Existing managed firewall rule; manual review.'}
$common=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)
if(-not $common -or (Test-Path -LiteralPath (Join-Path $common 'Esterium ERP.url'))){throw 'Existing shortcut or missing shortcut root.'}
$argsForOriginal=@{
    BundleDir=$bundle;AllowedOrigin=$origin;ListenAddress=$listen;TrustedRemoteAddress=$cidr
    TLSCertFile=$cert;TLSKeyFile=$key;ApprovedManifestSHA256=$OriginalManifestSHA256
    TrustedPublisherThumbprint=$OriginalPublisher;ReadinessReceiptDir=$readiness
}
if(-not $Activate){
    Write-Output 'PLAN: signed original delegates, lab configuration, TLS digests and disabled services verified.'
    Write-Output 'PLAN: admin commit must be rechecked before activation; no services changed.'
    return
}
# The signed inspector itself owns the temporary start and cleanup.
$inspection=@(& $inspector @inspectArgs -Inspect)
if($inspection.Count -ne 2 -or [string]$inspection[0] -cne 'ADMIN_COMMITTED' -or [string]$inspection[1] -cne 'PASS: all three services stopped and disabled; no recovery approval granted.'){throw 'ACTIVATION_REFUSED_ADMIN_NOT_COMMITTED'}
foreach($name in $names){Assert-ServiceState $name 'Stopped' 'Disabled'}
Assert-FreePorts
$success=$false
$stage='ACTIVATION_PROBE_FAILED'
try {
    Set-Service -Name EsteriumERP -StartupType Manual
    & $probe @argsForOriginal -Apply | Out-Null
    Assert-ServiceState 'EsteriumERP' 'Stopped' 'Manual'
    foreach($name in @('EsteriumGarnet','EsteriumPostgreSQL16')){Assert-ServiceState $name 'Stopped' 'Disabled'}
    Assert-FreePorts
    if(-not (Test-Path -LiteralPath (Join-Path $readiness 'probe-result.json') -PathType Leaf)){throw 'Original probe receipt missing.'}
    $stage='ACTIVATION_HTTPS_FAILED'
    & $activation @argsForOriginal -ActivationReceiptDir $installed -Apply | Out-Null
    $stage='ACTIVATION_POSTCHECK_FAILED'
    foreach($name in $names){Assert-ServiceState $name 'Running' 'Auto'}
    if(-not (Test-Path -LiteralPath (Join-Path $installed 'installation-ready.json') -PathType Leaf)){throw 'Original activation receipt missing.'}
    $success=$true
} catch {
    # No arbitrary delegate output or exception text at the recovery boundary.
    $success=$false
} finally {
    if(-not $success){
        $cleanupFailed=$false
        foreach($name in $names){
            try{Set-Service -Name $name -StartupType Disabled}catch{$cleanupFailed=$true}
            try{
                $service=Get-Service -Name $name
                if($service.Status -ne 'Stopped'){
                    Stop-Service -Name $name
                    $service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
            }catch{$cleanupFailed=$true}
        }
        foreach($name in $names){try{Assert-ServiceState $name 'Stopped' 'Disabled'}catch{$cleanupFailed=$true}}
        try{Assert-FreePorts}catch{$cleanupFailed=$true}
        if($cleanupFailed){throw 'ACTIVATION_CLEANUP_FAILED: isolate VM and review manually.'}
        # Preserve any genuine partial receipts or firewall/shortcut remnants.
        # Do not blindly remove, regenerate or reuse them.
    }
}
if(-not $success){throw ($stage+': services stopped and disabled; preserve receipts and review manually.')}
Write-Output 'PASS: original signed probe and HTTPS activation completed; services running/automatic.'
Write-Output 'LAB ONLY: fiscal disabled; functional, reboot, backup and commercial gates remain separate.'
