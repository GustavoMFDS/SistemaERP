#requires -Version 5.1
<#
  Read-only baseline of an activated LAN installation before HTTPS maintenance.
  Emits a sanitized report, never the full registry environment or private keys.
  Not a durable rollback snapshot and never changes any Windows resource.
#>
[CmdletBinding()]
param(
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$ReceiptDir=(Join-Path $env:ProgramData 'Esterium\Installation'),
    [Parameter(Mandatory=$true)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$ApprovedManifestSHA256
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows baseline requires Windows.' }
function Assert-Physical([string]$Path) {
    $cursor=$Path
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Baseline path contains a reparse point.'
        }
        $next=Split-Path -Parent $cursor
        if (-not $next -or $next -eq $cursor) { break }
        $cursor=$next
    }
}
$install=[IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
$program=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
$receiptRoot=[IO.Path]::GetFullPath($ReceiptDir).TrimEnd('\')
$data=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
if (-not $install.StartsWith($program+'\',[StringComparison]::OrdinalIgnoreCase) -or
    -not $receiptRoot.StartsWith($data+'\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Baseline requires the installed program and protected receipt directories.'
}
$manifest=Join-Path $install 'bundle-manifest.json'
$receiptFile=Join-Path $receiptRoot 'installation-ready.json'
Assert-Physical $manifest
Assert-Physical $receiptFile
$manifestHash=(Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash
if ($manifestHash -ne $ApprovedManifestSHA256) { throw 'Installed manifest differs from approved digest.' }
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $install | Out-Null
foreach ($path in @($receiptRoot,$receiptFile)) {
    foreach ($ace in (Get-Acl -LiteralPath $path).Access) {
        if ($ace.AccessControlType -ne 'Allow') { continue }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if ($sid -notin @('S-1-5-18','S-1-5-32-544')) { throw 'Activation receipt has an unapproved ACL.' }
    }
}
$r=Get-Content -LiteralPath $receiptFile -Raw | ConvertFrom-Json
if ($r.kind -cne 'ESTERIUM_LOCAL_INSTALL_ACTIVATION' -or $r.schemaVersion -ne 1 -or
    $r.erpService -cne 'EsteriumERP' -or $r.releaseManifestSHA256 -ne $manifestHash -or
    $r.automaticStartupApproved -cne $true -or $r.fiscalEmissionApproved -cne $false -or
    $r.firewallRule -cne 'EsteriumERP-HTTPS') { throw 'Unsupported LAN activation receipt.' }
$origin=$null
if (-not [Uri]::TryCreate([string]$r.allowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
    $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or $origin.AbsolutePath -ne '/' -or
    $origin.Query -ne '' -or $origin.Fragment -ne '' -or $origin.Port -ne [int]$r.httpsPort -or
    $r.listenAddress -cne ('0.0.0.0:'+[string]$r.httpsPort)) { throw 'LAN origin or listener mismatch.' }
$authority=$origin.GetLeftPart([UriPartial]::Authority)
$binary=Join-Path $install 'esterium-server.exe'
$svc=@(Get-CimInstance Win32_Service -Filter "Name='EsteriumERP'")
if ($svc.Count -ne 1 -or $svc[0].State -notin @('Running','Stopped') -or
    ([string]$svc[0].PathName).Trim().Trim('"') -ne $binary -or
    $svc[0].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE')) {
    throw 'Unexpected ERP service identity or executable.'
}
$map=@{}
foreach ($entry in @((Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP' -Name Environment).Environment)) {
    $separator=$entry.IndexOf('=')
    if ($separator -le 0) { throw 'Invalid service environment.' }
    $name=$entry.Substring(0,$separator)
    if ($map.ContainsKey($name)) { throw 'Duplicate service environment key.' }
    $map[$name]=$entry.Substring($separator+1)
}
if ($map['APP_PUBLIC_URL'] -cne $authority -or $map['CORS_ALLOWED_ORIGINS'] -cne $authority -or
    $map['HTTP_ADDR'] -cne $r.listenAddress -or $map['FISCAL_PROVIDER'] -cne 'disabled' -or
    $map['NFCE_SEFAZ_PRODUCTION_ENABLED'] -cne 'false' -or $map['NFCE_SEFAZ_HOMOLOGATION_ENABLED'] -cne 'false') {
    throw 'Service configuration differs from activation receipt or fiscal flags.'
}
foreach ($key in @('ESTERIUM_TLS_CERT_FILE','ESTERIUM_TLS_KEY_FILE')) {
    $path=[string]$map[$key]
    if (-not [IO.Path]::IsPathRooted($path) -or -not ([IO.Path]::GetFullPath($path)).StartsWith($data+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'TLS input outside protected ProgramData.'
    }
    Assert-Physical $path
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'TLS input missing.' }
}
$shortcut=Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)) 'Esterium ERP.url'
if ([string]$r.shortcut -ne $shortcut) { throw 'Unexpected shortcut path.' }
Assert-Physical $shortcut
if ([IO.File]::ReadAllText($shortcut) -notmatch ('(?m)^URL='+[regex]::Escape($authority)+'\r?$')) {
    throw 'Shortcut origin mismatch.'
}
$rule=@(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -PolicyStore ActiveStore)
if ($rule.Count -ne 1) { throw 'Managed firewall rule missing or ambiguous.' }
$port=$rule[0] | Get-NetFirewallPortFilter
$address=$rule[0] | Get-NetFirewallAddressFilter
$app=$rule[0] | Get-NetFirewallApplicationFilter
$actualScope=@($address.RemoteAddress)
$expectedScope=[string]$r.trustedRemoteAddress
$scopeMatches=$actualScope.Count -eq 1 -and
    ($actualScope[0] -eq $expectedScope -or ($expectedScope.EndsWith('/32') -and $actualScope[0] -eq $expectedScope.Substring(0,$expectedScope.Length-3)))
$profiles=([string]$rule[0].Profile -split ',') | ForEach-Object { $_.Trim() }
if ($rule[0].Enabled -ne 'True' -or $rule[0].Direction -ne 'Inbound' -or $rule[0].Action -ne 'Allow' -or
    @($profiles).Count -ne 2 -or 'Domain' -notin $profiles -or 'Private' -notin $profiles -or
    [string]$port.Protocol -notin @('TCP','6') -or [string]$port.LocalPort -ne [string]$r.httpsPort -or
    [string]$app.Program -ne $binary -or -not $scopeMatches) { throw 'Managed HTTPS firewall rule differs from receipt.' }
[ordered]@{
    kind='ESTERIUM_LAN_ROTATION_BASELINE_REPORT'
    manifestSHA256=$manifestHash
    receiptSHA256=(Get-FileHash -LiteralPath $receiptFile -Algorithm SHA256).Hash
    shortcutSHA256=(Get-FileHash -LiteralPath $shortcut -Algorithm SHA256).Hash
    allowedOrigin=$authority
    httpsPort=[int]$r.httpsPort
    trustedRemoteAddress=$expectedScope
    serviceState=[string]$svc[0].State
    liveReadinessVerified=$false
    durableRollbackSnapshot=$false
} | ConvertTo-Json -Compress
