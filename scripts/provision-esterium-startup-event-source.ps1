#requires -Version 5.1
# Separate, explicitly approved Windows maintenance; never part of the
# application-only file swap. Never deletes event history or starts services.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$ApprovedManifestSHA256,
    [Parameter(Mandatory=$true)][string]$TrustedPublisherThumbprint,
    [switch]$Apply,
    [switch]$ConfirmEventSource
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($env:OS -ne 'Windows_NT'){throw 'Windows required.'}
if($ApprovedManifestSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or $TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$'){throw 'Independent release approval required.'}
if($Apply -and -not $ConfirmEventSource){throw 'Explicit event-source maintenance confirmation required.'}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Elevated inspection required for event-source maintenance.'}
$bundle=Split-Path -Parent $PSScriptRoot
$signer=$TrustedPublisherThumbprint.ToUpperInvariant()
foreach($path in @($PSCommandPath,(Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'),(Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'))){
    $signature=Get-AuthenticodeSignature -LiteralPath $path
    if($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $signer){throw 'Maintenance entrypoint or helper is not signed by approved publisher.'}
}
& (Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1') -BundleDir $bundle -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $signer | Out-Null

$source='EsteriumERPStartup'
$resource=Join-Path $env:SystemRoot 'System32\EventCreate.exe'
$key='HKLM:\SYSTEM\CurrentControlSet\Services\EventLog\Application\'+$source
function Get-StartupSourcePolicy([bool]$Exists,[string]$Log,[string]$MessageFile,[int]$Types,[string]$ExpectedResource) {
    if(-not $Exists){return 'missing'}
    if($Log -cne 'Application'){return 'wrong-log'}
    if(-not [string]::Equals([Environment]::ExpandEnvironmentVariables($MessageFile),$ExpectedResource,[StringComparison]::OrdinalIgnoreCase)){return 'wrong-resource'}
    if(($Types -band 1) -eq 0){return 'wrong-type'}
    return 'pass'
}
function Read-StartupSourcePolicy {
    if(-not [Diagnostics.EventLog]::SourceExists($source)){
        if(Test-Path -LiteralPath $key){throw 'Ambiguous existing event source; inspect manually.'}
        return 'missing'
    }
    $log=[Diagnostics.EventLog]::LogNameFromSourceName($source,'.')
    if($log -cne 'Application'){return 'wrong-log'}
    $p=Get-ItemProperty -LiteralPath $key -ErrorAction Stop
    return Get-StartupSourcePolicy $true $log ([string]$p.EventMessageFile) ([int]$p.TypesSupported) $resource
}
function Assert-StoppedEsteriumService {
    $svc=@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'" -ErrorAction Stop)
    $binary=Join-Path $env:ProgramFiles 'EsteriumERP\esterium-server.exe'
    if($svc.Count -ne 1 -or $svc[0].State -cne 'Stopped' -or [int]$svc[0].ProcessId -ne 0 -or [string]$svc[0].StartName -ine 'NT AUTHORITY\LocalService' -or [string]$svc[0].PathName -ine ('"'+$binary+'"')){throw 'Expected installed EsteriumERP service must be stopped with its original LocalService identity and binary binding.'}
}
Assert-StoppedEsteriumService
if(-not (Test-Path -LiteralPath $resource -PathType Leaf)){throw 'Windows event resource unavailable.'}
$state=Read-StartupSourcePolicy
if($state -notin @('pass','missing')){throw ('Existing event source rejected: '+$state+'. No changes made.')}
if(-not $Apply){Write-Output ('PLAN: startup event source '+$state+'; no changes made.');return}
if($state -eq 'pass'){Write-Output 'PASS: existing approved startup event source preserved; no changes made.';return}
Assert-StoppedEsteriumService
if((Read-StartupSourcePolicy) -cne 'missing'){throw 'Event source state changed before creation; retry plan.'}
try{
    New-EventLog -LogName Application -Source $source -MessageResourceFile $resource -ErrorAction Stop
    New-ItemProperty -LiteralPath $key -Name TypesSupported -PropertyType DWord -Value 7 -Force -ErrorAction Stop | Out-Null
    if((Read-StartupSourcePolicy) -cne 'pass'){throw 'Created event source failed verification.'}
    Assert-StoppedEsteriumService
    Write-Output 'PASS: startup event source provisioned; service remains stopped.'
}catch{
    # Never delete a source or event history automatically, including after a
    # partial failure. No application swap or start is authorized by this tool.
    throw 'Event source provisioning incomplete; preserve registry and event history, inspect manually before starting or updating ERP.'
}
