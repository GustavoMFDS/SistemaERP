#requires -Version 5.1
<#
  Conservative application uninstall. Removes only the exact ERP SCM service,
  its owned HTTPS rule/shortcut and verified Program Files bundle. PostgreSQL,
  Garnet, ProgramData secrets, SQL data, fiscal files and backups stay intact.
#>
[CmdletBinding()]
param(
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$ActivationReceiptDir=(Join-Path $env:ProgramData 'Esterium\Installation'),
    [string]$ReadinessReceiptDir=(Join-Path $env:ProgramData 'Esterium\FirstRunReadiness'),
    [string]$ReceiptArchiveDir=(Join-Path $env:ProgramData 'Esterium\Uninstalled'),
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$ConfirmUninstall,
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
if($env:OS -ne 'Windows_NT'){throw 'Esterium uninstall requires Windows.'}
if($UnsignedDisposableCIFixture -and (
   $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
   $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1' -or $env:ESTERIUM_UNINSTALL_DISPOSABLE_CI -ne '1')){
    throw 'Unsigned uninstall is restricted to the disposable GitHub-hosted Windows lab.'
}
if($Apply -and -not $ConfirmUninstall){throw 'Explicit -ConfirmUninstall is required before uninstall.'}
if(-not [IO.Path]::IsPathRooted($InstallDir) -or -not [IO.Path]::IsPathRooted($ActivationReceiptDir) -or
   -not [IO.Path]::IsPathRooted($ReadinessReceiptDir) -or -not [IO.Path]::IsPathRooted($ReceiptArchiveDir)){
    throw 'Uninstall and receipt paths must be absolute.'
}
$install=[IO.Path]::GetFullPath((Resolve-Path -LiteralPath $InstallDir -ErrorAction Stop).Path).TrimEnd('\')
$programFiles=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
if(-not $install.StartsWith($programFiles+'\',[StringComparison]::OrdinalIgnoreCase) -or
   (Split-Path -Parent $install) -ine $programFiles -or (Split-Path -Leaf $install) -notmatch '^EsteriumERP([_-][A-Za-z0-9-]+)?$'){
    throw 'Uninstall target must be a dedicated EsteriumERP folder directly under Program Files.'
}
$installInfo=Get-Item -LiteralPath $install -Force -ErrorAction Stop
if(-not $installInfo.PSIsContainer -or ($installInfo.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
    throw 'Uninstall target must be a physical directory.'
}
$manifestHash=(Get-FileHash -LiteralPath (Join-Path $install 'bundle-manifest.json') -Algorithm SHA256 -ErrorAction Stop).Hash
if($ApprovedManifestSHA256 -and $manifestHash -cne $ApprovedManifestSHA256.ToUpperInvariant()){
    throw 'Installed release manifest differs from the independently approved digest.'
}
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $install | Out-Null
$programDataRoot=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
foreach($dir in @($ActivationReceiptDir,$ReadinessReceiptDir,$ReceiptArchiveDir)){
    $full=[IO.Path]::GetFullPath($dir)
    if(-not $full.StartsWith($programDataRoot,[StringComparison]::OrdinalIgnoreCase)){
        throw 'Receipt archive and installation records must remain under ProgramData\Esterium.'
    }
}
$activationDir=[IO.Path]::GetFullPath($ActivationReceiptDir).TrimEnd('\')
$receiptPath=Join-Path $activationDir 'installation-ready.json'
$receipt=Get-Content -LiteralPath $receiptPath -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
if($receipt.kind -cne 'ESTERIUM_LOCAL_INSTALL_ACTIVATION' -or [int]$receipt.schemaVersion -ne 1 -or
   $receipt.releaseManifestSHA256 -cne $manifestHash -or $receipt.erpService -cne 'EsteriumERP' -or
   $receipt.fiscalEmissionApproved -cne $false){
    throw 'Installation receipt does not match this non-fiscal bundle.'
}
$expectedOrigin=[string]$receipt.allowedOrigin
$service=@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
if($service.Count -ne 1 -or
   -not [string]::Equals(([string]$service[0].PathName).Trim().Trim('"'),(Join-Path $install 'esterium-server.exe'),[StringComparison]::OrdinalIgnoreCase) -or
   $service[0].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE')){
    throw 'Uninstall requires the exact EsteriumERP LocalService registration; unrelated services are never changed.'
}
if($service[0].State -cne 'Stopped'){throw 'Stop and reconcile all POS activity before uninstall.'}
$backupTask=Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction SilentlyContinue
if($null -ne $backupTask -and $backupTask.State -ne 'Disabled'){
    throw 'Disable and review the Esterium backup task before removing the application.'
}
$shortcutRoot=[Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)
$shortcut=Join-Path $shortcutRoot 'Esterium ERP.url'
if(-not [string]::Equals([IO.Path]::GetFullPath([string]$receipt.shortcut),$shortcut,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Installation receipt shortcut path differs from the managed Start Menu location.'
}
$shortcutExists=Test-Path -LiteralPath $shortcut
if($shortcutExists -and [IO.File]::ReadAllText($shortcut) -notmatch ('(?m)^URL='+[Regex]::Escape($expectedOrigin)+'\r?$')){
    throw 'Start Menu shortcut was changed by another owner; manual review required.'
}
$firewall=@(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction SilentlyContinue)
if($firewall.Count -gt 1){throw 'Multiple firewall rules use the managed Esterium name.'}
if($firewall.Count -eq 1){
    $ports=$firewall[0] | Get-NetFirewallPortFilter
    $addresses=$firewall[0] | Get-NetFirewallAddressFilter
    $app=$firewall[0] | Get-NetFirewallApplicationFilter
    if($firewall[0].Enabled -ne 'True' -or $firewall[0].Action -ne 'Allow' -or
       $firewall[0].Direction -ne 'Inbound' -or [string]$firewall[0].Profile -notmatch 'Domain' -or
       [string]$firewall[0].Profile -notmatch 'Private' -or [string]$ports.Protocol -ne 'TCP' -or
       [string]$ports.LocalPort -ne [string]$receipt.httpsPort -or
       -not (Test-ExactFirewallRemoteScope -Actual @($addresses.RemoteAddress) -Expected ([string]$receipt.trustedRemoteAddress)) -or
       -not [string]::Equals([string]$app.Program,(Join-Path $install 'esterium-server.exe'),[StringComparison]::OrdinalIgnoreCase)){
        throw 'Managed firewall rule was changed; uninstall refuses to remove a rule with a different scope.'
    }
}
$unregister=Join-Path $PSScriptRoot 'unregister-esterium-windows-service.ps1'
if(-not (Test-Path -LiteralPath $unregister -PathType Leaf)){throw 'Verified service removal helper is missing.'}
if(-not $Apply){
    Write-Output 'PLAN: remove only the exact stopped ERP service, matching HTTPS rule/shortcut and verified Program Files bundle.'
    Write-Output 'PLAN: archive installer receipts; preserve PostgreSQL/Garnet, ProgramData credentials, SQL data, fiscal files and all backups.'
    Write-Output 'PLAN: no changes made. Dependency services remain installed and retain their current startup state.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Administrator elevation is required for uninstall.'}
if(-not $UnsignedDisposableCIFixture){
    if($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$'){
        throw 'Uninstall requires independent release digest and publisher approval.'
    }
    foreach($scriptPath in @($PSCommandPath,$unregister,(Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'),
        (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'))){
        $signature=Get-AuthenticodeSignature -LiteralPath $scriptPath
        if($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
           $null -eq $signature.SignerCertificate -or
           $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $TrustedPublisherThumbprint.ToUpperInvariant()){
            throw 'Uninstall entrypoints are not signed by the independently approved publisher.'
        }
    }
    if(-not [string]::Equals([IO.Path]::GetFullPath($PSScriptRoot),(Join-Path $install 'tools'),[StringComparison]::OrdinalIgnoreCase)){
        throw 'Uninstall must run from the signed installed bundle.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $install `
        -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
}elseif($ApprovedManifestSHA256 -and $manifestHash -cne $ApprovedManifestSHA256.ToUpperInvariant()){
    throw 'Disposable uninstall fixture manifest digest mismatch.'
}
if(-not $UnsignedDisposableCIFixture -and [string]::IsNullOrWhiteSpace($TrustedPublisherThumbprint)){
    throw 'Trusted publisher thumbprint is required.'
}
$unregisterArgs=@{InstallDir=$install;UnsignedDisposableCIFixture=$UnsignedDisposableCIFixture}
if($ApprovedManifestSHA256){$unregisterArgs.ApprovedManifestSHA256=$ApprovedManifestSHA256}
if($TrustedPublisherThumbprint){$unregisterArgs.TrustedPublisherThumbprint=$TrustedPublisherThumbprint}
& $unregister @unregisterArgs -Apply | Out-Null
$serviceGone=$false
for($attempt=0;$attempt -lt 60;$attempt++){
    if(@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction SilentlyContinue).Count -eq 0){$serviceGone=$true;break}
    Start-Sleep -Seconds 1
}
if(-not $serviceGone){throw 'SCM still reports the ERP service; verified application files were preserved for manual cleanup.'}
if($firewall.Count -eq 1){Remove-NetFirewallRule -Name 'EsteriumERP-HTTPS' -ErrorAction Stop}
if($shortcutExists){Remove-Item -LiteralPath $shortcut -Force -ErrorAction Stop}
$archiveRoot=[IO.Path]::GetFullPath($ReceiptArchiveDir)
if(Test-Path -LiteralPath $archiveRoot){
    $archiveInfo=Get-Item -LiteralPath $archiveRoot -Force
    if(-not $archiveInfo.PSIsContainer -or ($archiveInfo.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
        throw 'Receipt archive destination is not a physical directory.'
    }
}else{[IO.Directory]::CreateDirectory($archiveRoot)|Out-Null}
$archive=Join-Path $archiveRoot ([DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')+'-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($archive)|Out-Null
foreach($sourceDir in @($activationDir,[IO.Path]::GetFullPath($ReadinessReceiptDir).TrimEnd('\'))){
    if(Test-Path -LiteralPath $sourceDir){
        $info=Get-Item -LiteralPath $sourceDir -Force
        if(-not $info.PSIsContainer -or ($info.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
            throw 'Receipt directory is not physical; application files were left installed for manual review.'
        }
        Move-Item -LiteralPath $sourceDir -Destination (Join-Path $archive (Split-Path -Leaf $sourceDir)) -ErrorAction Stop
    }
}
Remove-Item -LiteralPath $install -Recurse -Force -ErrorAction Stop
Write-Output 'PASS: ERP service, owned HTTPS firewall rule, shortcut and verified application bundle removed.'
Write-Output "PASS: installation receipts archived at $archive."
Write-Output 'PRESERVED: PostgreSQL/Garnet services and data, ProgramData credentials/TLS, SQL/fiscal data, backups and update rollback bundles.'
Write-Output 'Review dependency services separately; this conservative uninstall does not stop or delete them.'

