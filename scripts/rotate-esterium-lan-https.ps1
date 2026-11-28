#requires -Version 5.1
<#
  Assisted same-port LAN HTTPS rotation. No CA installation, certificate issuance,
  private key export, firewall change, database mutation or fiscal activation.
  Apply requires signed installed tools, independent pins and exclusive downtime.
  A protected pending journal survives process interruption; never auto-overwrite it.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$NewOrigin,
    [Parameter(Mandatory=$true)][string]$NewCertificateFile,
    [Parameter(Mandatory=$true)][string]$NewKeyFile,
    [Parameter(Mandatory=$true)][string]$ApprovedRootFile,
    [Parameter(Mandatory=$true)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$NewLeafSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$ApprovedRootFileSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$ApprovedManifestSHA256,
    [string]$TrustedPublisherThumbprint='',
    [string]$InstallDir=(Join-Path $env:ProgramFiles 'EsteriumERP'),
    [string]$PostgresBin=(Join-Path $env:ProgramFiles 'PostgreSQL\16\bin'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [ValidateRange(10,90)][int]$ReadinessTimeoutSeconds=90,
    [string]$NewTrustedRemoteAddress='',
    [switch]$Apply,
    [switch]$ConfirmMaintenance
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'LAN maintenance requires Windows.' }
$install=[IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
$data=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$receiptDir=Join-Path $data 'Installation'
$receiptFile=Join-Path $receiptDir 'installation-ready.json'
$journal=Join-Path $receiptDir 'lan-rotation-pending.json'
$baselineScript=Join-Path $PSScriptRoot 'check-esterium-lan-rotation-baseline.ps1'
$terminal=Join-Path $PSScriptRoot 'check-esterium-lan-terminal.ps1'
$dependencyCheck=Join-Path $PSScriptRoot 'check-esterium-local-dependencies.ps1'
$gate=Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
$preflight=Join-Path $PSScriptRoot 'esterium-lan-tls-preflight.exe'
$registry='HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP'
$managed=@('APP_PUBLIC_URL','CORS_ALLOWED_ORIGINS','ESTERIUM_TLS_CERT_FILE','ESTERIUM_TLS_KEY_FILE')

function Assert-ProtectedTLS([string]$Path,[bool]$ServiceRead) {
    if ($Path -notmatch '^[A-Za-z]:\\' -or $Path.Substring(2).Contains(':')) { throw 'TLS inputs must be absolute local files.' }
    $full=[IO.Path]::GetFullPath($Path)
    if (-not $full.StartsWith($data+'\',[StringComparison]::OrdinalIgnoreCase)) { throw 'TLS input must be under ProgramData\Esterium.' }
    $cursor=$full
    $isFile=$true
    $hasReader=$false
    $writeMask=[Security.AccessControl.FileSystemRights]::Write -bor
        [Security.AccessControl.FileSystemRights]::Delete -bor
        [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
        [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
        [Security.AccessControl.FileSystemRights]::TakeOwnership
    while ($cursor.Length -ge $data.Length) {
        $item=Get-Item -LiteralPath $cursor -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
            ($isFile -and ($item.PSIsContainer -or $item.Length -eq 0))) {
            throw 'TLS input must be a nonempty physical file.'
        }
        $acl=Get-Acl -LiteralPath $cursor
        $owner=([Security.Principal.NTAccount]::new($acl.Owner)).Translate([Security.Principal.SecurityIdentifier]).Value
        if ($owner -notin @('S-1-5-18','S-1-5-32-544')) { throw 'TLS path owner is not approved.' }
        foreach ($ace in $acl.Access) {
            if ($ace.AccessControlType -ne 'Allow') { continue }
            $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
            if ($sid -in @('S-1-5-18','S-1-5-32-544')) { continue }
            if (($ace.FileSystemRights -band $writeMask) -ne 0) { throw 'Unapproved TLS path writer.' }
            if ($isFile -and $ServiceRead -and $sid -ne 'S-1-5-19') { throw 'Unapproved TLS input reader.' }
            if ($isFile -and $sid -eq 'S-1-5-19' -and
                ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0) { $hasReader=$true }
        }
        if ($cursor -eq $data) { break }
        $cursor=Split-Path -Parent $cursor
        $isFile=$false
    }
    if ($ServiceRead -and -not $hasReader) { throw 'LocalService requires TLS input read access.' }
    return $full
}
function Assert-PrivateScope([string]$Value) {
    if ($Value -cnotmatch '^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/([0-9]{1,2})$') { throw 'Expected canonical private IPv4 CIDR.' }
    $addressText=$Matches[1];$prefix=[int]$Matches[2]
    $ip=$null
    if (-not [Net.IPAddress]::TryParse($addressText,[ref]$ip) -or $ip.ToString() -cne $addressText -or $prefix -lt 16 -or $prefix -gt 32) { throw 'Expected canonical private IPv4 CIDR /16 through /32.' }
    $v=$ip.GetAddressBytes()
    if (-not (($v[0] -eq 10) -or ($v[0] -eq 172 -and $v[1] -ge 16 -and $v[1] -le 31) -or ($v[0] -eq 192 -and $v[1] -eq 168))) { throw 'Firewall scope must remain private RFC1918.' }
    for ($bit=$prefix;$bit -lt 32;$bit++) {
        if (($v[[int][Math]::Floor($bit/8)] -band (1 -shl (7-($bit%8)))) -ne 0) { throw 'Firewall scope must use canonical network address.' }
    }
    return $addressText+'/'+$prefix
}
function Set-OwnedScope([string]$Scope) {
    $null=Assert-PrivateScope $Scope
    $rules=@(Get-NetFirewallRule -Name 'EsteriumERP-HTTPS' -PolicyStore PersistentStore -ErrorAction Stop)
    if ($rules.Count -ne 1) { throw 'Expected exactly one persistent owned HTTPS rule.' }
    $rules[0] | Get-NetFirewallAddressFilter | Set-NetFirewallAddressFilter -RemoteAddress $Scope -ErrorAction Stop | Out-Null
}
function Read-ManagedEnvironment {
    $result=@{}
    foreach ($entry in @((Get-ItemProperty -LiteralPath $registry -Name Environment).Environment)) {
        $i=$entry.IndexOf('=')
        if ($i -le 0) { throw 'Invalid service environment.' }
        $name=$entry.Substring(0,$i)
        if ($result.ContainsKey($name)) { throw 'Duplicate service environment key.' }
        $result[$name]=$entry.Substring($i+1)
    }
    return $result
}
function Set-ManagedEnvironment([hashtable]$Values) {
    $entries=@((Get-ItemProperty -LiteralPath $registry -Name Environment).Environment)
    $seen=@{}
    $updated=@(foreach ($entry in $entries) {
        $i=$entry.IndexOf('=')
        if ($i -le 0) { throw 'Invalid service environment.' }
        $name=$entry.Substring(0,$i)
        if ($seen.ContainsKey($name)) { throw 'Duplicate service environment key.' }
        $seen[$name]=$true
        if ($name -cin $managed) { $name+'='+[string]$Values[$name] } else { $entry }
    })
    foreach ($name in $managed) {
        if (-not $seen.ContainsKey($name)) { throw 'Managed service value missing.' }
    }
    Set-ItemProperty -LiteralPath $registry -Name Environment -Value ([string[]]$updated) -ErrorAction Stop
}
function Write-DurableNew([string]$Path,[byte[]]$Bytes) {
    $stream=[IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $stream.Write($Bytes,0,$Bytes.Length); $stream.Flush($true) }
    finally { $stream.Dispose() }
}
function Replace-Bytes([string]$Path,[byte[]]$Bytes) {
    $temp=Join-Path (Split-Path -Parent $Path) ('.esterium-rotation-'+[Guid]::NewGuid().ToString('N')+'.tmp')
    try {
        Write-DurableNew $temp $Bytes
        [IO.File]::Replace($temp,$Path,[NullString]::Value)
    } finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force }
    }
}
function Wait-Ready([string]$Origin) {
    $until=[DateTime]::UtcNow.AddSeconds($ReadinessTimeoutSeconds)
    do {
        try {
            $health=Invoke-RestMethod -Uri ($Origin+'/health/ready') -Method Get -UseBasicParsing -MaximumRedirection 0 -TimeoutSec 5
            if ($health.status -ceq 'ready') { return }
        } catch {}
        Start-Sleep -Seconds 1
    } while ([DateTime]::UtcNow -lt $until)
    throw 'Trusted HTTPS readiness failed.'
}
function Stop-ERP {
    if ((Get-Service EsteriumERP).Status -ne 'Stopped') { Stop-Service EsteriumERP -ErrorAction Stop }
    (Get-Service EsteriumERP).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(45))
}
function Start-ERP {
    & $dependencyCheck -PostgresServiceName 'EsteriumPostgreSQL16' -RedisServiceName 'EsteriumGarnet' -PostgresBin $PostgresBin -PostgresPort $PostgresPort -RedisPort $GarnetPort -RequireServiceOwnership | Out-Null
    Start-Service EsteriumERP -ErrorAction Stop
    (Get-Service EsteriumERP).WaitForStatus('Running',[TimeSpan]::FromSeconds(45))
}

if ($Apply) {
    if (-not $ConfirmMaintenance -or $TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Apply requires explicit maintenance confirmation and approved publisher.'
    }
    $principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Elevation required.' }
    if ($PSScriptRoot -ne (Join-Path $install 'tools')) { throw 'Use the signed installed tools.' }
    foreach ($file in @($PSCommandPath,$baselineScript,$terminal,$dependencyCheck,$gate,$preflight,(Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1'))) {
        $sig=Get-AuthenticodeSignature -LiteralPath $file
        if ($sig.Status -ne 'Valid' -or $null -eq $sig.SignerCertificate -or
            $sig.SignerCertificate.Thumbprint -ne $TrustedPublisherThumbprint) { throw 'Unapproved maintenance tool signature.' }
    }
    & $gate -BundleDir $install -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null
}
if (Test-Path -LiteralPath $journal) { throw 'Pending LAN rotation journal exists; reconcile before retry.' }
$baseline=(& $baselineScript -InstallDir $install -ReceiptDir $receiptDir -ApprovedManifestSHA256 $ApprovedManifestSHA256) | ConvertFrom-Json
if ($baseline.serviceState -cne 'Running') { throw 'Rotation requires an active baseline service.' }
$newUri=$null
if (-not [Uri]::TryCreate($NewOrigin,[UriKind]::Absolute,[ref]$newUri) -or
    $newUri.HostNameType -ne 'Dns' -or $newUri.Host -match '(?i)\.(example|invalid|test)$' -or
    $newUri.Port -ne $baseline.httpsPort -or $newUri.Port -in @(80,8080,5432,6379) -or
    ($newUri.Port -lt 1024 -and $newUri.Port -ne 443)) { throw 'Use a real LAN DNS origin on the existing HTTPS port.' }
$authority=$newUri.GetLeftPart([UriPartial]::Authority)
$oldScope=Assert-PrivateScope ([string]$baseline.trustedRemoteAddress)
$newScope=if ($NewTrustedRemoteAddress) { Assert-PrivateScope $NewTrustedRemoteAddress } else { $oldScope }
$cert=Assert-ProtectedTLS $NewCertificateFile $true
$key=Assert-ProtectedTLS $NewKeyFile $true
$root=Assert-ProtectedTLS $ApprovedRootFile $false
if ($cert -eq $key -or $cert -eq $root -or $key -eq $root) { throw 'Distinct TLS files required.' }
& $preflight -origin $NewOrigin -port $baseline.httpsPort -cert-file $cert -key-file $key -root-file $root -leaf-sha256 $NewLeafSHA256 -root-file-sha256 $ApprovedRootFileSHA256 | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'New TLS identity preflight refused.' }
$localIPs=@(Get-NetIPAddress -AddressFamily IPv4 | ForEach-Object IPAddress)
$resolved=@([Net.Dns]::GetHostAddresses($newUri.DnsSafeHost))
if ($resolved.Count -eq 0) { throw 'New origin DNS did not resolve.' }
foreach ($address in $resolved) {
    $v=$address.GetAddressBytes()
    if ($address.AddressFamily -ne 'InterNetwork' -or
        -not (($v[0] -eq 10) -or ($v[0] -eq 172 -and $v[1] -ge 16 -and $v[1] -le 31) -or ($v[0] -eq 192 -and $v[1] -eq 168)) -or
        $address.ToString() -notin $localIPs) { throw 'New origin must resolve exclusively to this server private IPv4 addresses.' }
}
if (-not $Apply) {
    Write-Output 'PLAN: same-port rotation preflight passed; no service or files changed.'
    return
}
$lock=[Threading.Mutex]::new($false,'Global\EsteriumERP-LANRotation')
$held=$false
try {
    $held=$lock.WaitOne(0)
    if (-not $held) { throw 'Another LAN maintenance operation is active.' }
    if (Test-Path -LiteralPath $journal) { throw 'Pending rotation requires manual reconciliation.' }
    $fresh=(& $baselineScript -InstallDir $install -ReceiptDir $receiptDir -ApprovedManifestSHA256 $ApprovedManifestSHA256) | ConvertFrom-Json
    if ($fresh.receiptSHA256 -ne $baseline.receiptSHA256 -or $fresh.shortcutSHA256 -ne $baseline.shortcutSHA256 -or $fresh.serviceState -cne 'Running') {
        throw 'Baseline changed during preflight.'
    }
    $oldOrigin=[string]$fresh.allowedOrigin
    Wait-Ready $oldOrigin
    $envBefore=Read-ManagedEnvironment
    $old=@{}
    foreach ($name in $managed) { $old[$name]=[string]$envBefore[$name] }
    $null=Assert-ProtectedTLS $old['ESTERIUM_TLS_CERT_FILE'] $true
    $null=Assert-ProtectedTLS $old['ESTERIUM_TLS_KEY_FILE'] $true
    if ($cert -eq $old['ESTERIUM_TLS_CERT_FILE'] -or $key -eq $old['ESTERIUM_TLS_KEY_FILE']) {
        throw 'Use new protected TLS files; old files must remain available for rollback.'
    }
    $next=@{APP_PUBLIC_URL=$authority;CORS_ALLOWED_ORIGINS=$authority;ESTERIUM_TLS_CERT_FILE=$cert;ESTERIUM_TLS_KEY_FILE=$key}
    $receiptBefore=[IO.File]::ReadAllBytes($receiptFile)
    $receipt=[Text.Encoding]::UTF8.GetString($receiptBefore) | ConvertFrom-Json
    $shortcut=[string]$receipt.shortcut
    $shortcutBefore=[IO.File]::ReadAllBytes($shortcut)
    $record=[ordered]@{
        kind='ESTERIUM_LAN_ROTATION_PENDING';schemaVersion=1;manifestSHA256=$ApprovedManifestSHA256.ToUpperInvariant()
        oldOrigin=$oldOrigin;newOrigin=$authority;oldValues=$old;newValues=$next
        oldScope=$oldScope;newScope=$newScope
        receiptBase64=[Convert]::ToBase64String($receiptBefore)
        shortcutBase64=[Convert]::ToBase64String($shortcutBefore)
        createdUtc=[DateTime]::UtcNow.ToString('o')
    }
    $utf8=[Text.UTF8Encoding]::new($false)
    Write-DurableNew $journal ($utf8.GetBytes(($record | ConvertTo-Json -Depth 5)))
    & icacls.exe $journal '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not explicitly protect pending journal; reconcile before retry.' }
    try {
        Stop-ERP
        Set-ManagedEnvironment $next
        if ($newScope -cne $oldScope) { Set-OwnedScope $newScope }
        $receipt.trustedRemoteAddress=$newScope
        $receipt.allowedOrigin=$authority
        $receipt.checkedUtc=[DateTime]::UtcNow.ToString('o')
        Replace-Bytes $shortcut ($utf8.GetBytes("[InternetShortcut]"+[Environment]::NewLine+'URL='+$authority+[Environment]::NewLine))
        Replace-Bytes $receiptFile ($utf8.GetBytes(($receipt | ConvertTo-Json -Depth 5)+[Environment]::NewLine))
        Start-ERP
        Wait-Ready $authority
        & $terminal -ERPUrl $authority -ExpectedCertificateSHA256 $NewLeafSHA256 | Out-Null
        & $baselineScript -InstallDir $install -ReceiptDir $receiptDir -ApprovedManifestSHA256 $ApprovedManifestSHA256 | Out-Null
        $committed=Join-Path $receiptDir ('lan-rotation-committed-'+[Guid]::NewGuid().ToString('N')+'.json')
        [IO.File]::Move($journal,$committed)
        Write-Output 'PASS: same-port HTTPS rotation committed; external terminal acceptance remains required.'
    } catch {
        try {
            Stop-ERP
            Set-ManagedEnvironment $old
            if ($newScope -cne $oldScope) { Set-OwnedScope $oldScope }
            Replace-Bytes $shortcut $shortcutBefore
            Replace-Bytes $receiptFile $receiptBefore
            Start-ERP
            Wait-Ready $oldOrigin
            $restored=(& $baselineScript -InstallDir $install -ReceiptDir $receiptDir -ApprovedManifestSHA256 $ApprovedManifestSHA256) | ConvertFrom-Json
            if ($restored.receiptSHA256 -ne $fresh.receiptSHA256 -or $restored.shortcutSHA256 -ne $fresh.shortcutSHA256) { throw 'Restored receipt or shortcut differs from baseline.' }
            $actual=Read-ManagedEnvironment
            foreach ($name in $managed) { if ($actual[$name] -cne $old[$name]) { throw 'Restored managed environment differs.' } }
            [IO.File]::Move($journal,(Join-Path $receiptDir ('lan-rotation-restored-'+[Guid]::NewGuid().ToString('N')+'.json')))
        } catch {
            throw 'RECOVERY_REQUIRED: retain pending journal and reconcile original configuration with authorized administrator.'
        }
        throw 'Rotation refused; original HTTPS configuration restored and ready.'
    }
} finally {
    if ($held) { $lock.ReleaseMutex() }
    $lock.Dispose()
}
