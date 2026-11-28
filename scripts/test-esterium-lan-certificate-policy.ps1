#requires -Version 5.1
# Pure diagnostic tests: extract only the function, no network or certificate changes.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$tool=Join-Path $PSScriptRoot 'check-esterium-lan-terminal.ps1'
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($tool,[ref]$t,[ref]$e)
if($e.Count){throw 'LAN checker syntax invalid.'}
$defs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Get-LanCertificateRejection'},$true))
if($defs.Count -ne 1){throw 'Expected unique LAN certificate policy.'}
. ([scriptblock]::Create($defs[0].Extent.Text))
$cases=@(
 @{Name='valid';Cert=$true;Chain=$true;Errors='None';Flags=@();Pin=$true;Want='pass'},
 @{Name='missing certificate';Cert=$false;Chain=$true;Errors='None';Flags=@();Pin=$true;Want='certificate-missing'},
 @{Name='unavailable certificate';Cert=$true;Chain=$true;Errors='RemoteCertificateNotAvailable';Flags=@();Pin=$true;Want='certificate-missing'},
 @{Name='missing chain';Cert=$true;Chain=$false;Errors='None';Flags=@();Pin=$true;Want='chain-missing'},
 @{Name='revoked';Cert=$true;Chain=$true;Errors='RemoteCertificateChainErrors';Flags=@('Revoked');Pin=$true;Want='certificate-revoked'},
 @{Name='unknown revocation';Cert=$true;Chain=$true;Errors='RemoteCertificateChainErrors';Flags=@('RevocationStatusUnknown');Pin=$true;Want='revocation-unavailable'},
 @{Name='offline revocation';Cert=$true;Chain=$true;Errors='None';Flags=@('OfflineRevocation');Pin=$true;Want='revocation-unavailable'},
 @{Name='revoked dominates unknown';Cert=$true;Chain=$true;Errors='RemoteCertificateChainErrors';Flags=@('RevocationStatusUnknown','Revoked');Pin=$true;Want='certificate-revoked'},
 @{Name='wrong DNS';Cert=$true;Chain=$true;Errors='RemoteCertificateNameMismatch';Flags=@();Pin=$true;Want='name-mismatch'},
 @{Name='untrusted root';Cert=$true;Chain=$true;Errors='RemoteCertificateChainErrors';Flags=@('UntrustedRoot');Pin=$true;Want='chain-untrusted'},
 @{Name='expired';Cert=$true;Chain=$true;Errors='None';Flags=@('NotTimeValid');Pin=$true;Want='chain-untrusted'},
 @{Name='policy error without flags';Cert=$true;Chain=$true;Errors='RemoteCertificateChainErrors';Flags=@();Pin=$true;Want='chain-untrusted'},
 @{Name='wrong pin';Cert=$true;Chain=$true;Errors='None';Flags=@('NoError');Pin=$false;Want='pin-mismatch'},
 @{Name='matching pin cannot override revocation';Cert=$true;Chain=$true;Errors='None';Flags=@('RevocationStatusUnknown');Pin=$true;Want='revocation-unavailable'}
)
foreach($case in $cases){
    $actual=Get-LanCertificateRejection $case.Cert $case.Chain $case.Errors $case.Flags $case.Pin
    if($actual -cne $case.Want){throw ('LAN policy case '+$case.Name+' expected '+$case.Want+' got '+$actual)}
    Write-Output ('PASS '+$case.Name)
}
Write-Output ('TOTAL '+$cases.Count+' pure TLS diagnostic cases passed; no live trust evidence.')
