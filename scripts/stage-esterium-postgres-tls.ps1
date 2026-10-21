#requires -Version 5.1
<#
  Install staged localhost TLS/SCRAM config into a NEW PostgreSQL 16 cluster.
  Never starts/stops/enables services and refuses a second application.
  No existing production cluster may be upgraded by this first-install helper.
  A fresh-cluster attestation is created only by the signed vendor installer
  wrapper after checking its new, disabled Windows service and PG_VERSION.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PostgresDataDir,
    [Parameter(Mandatory=$true)][string]$TlsPolicyDir,
    [Parameter(Mandatory=$true)][string]$FreshClusterMarker,
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'New PostgreSQL TLS activation requires Windows.' }
if ($UnsignedDisposableCIFixture -and ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_ACTIVATE_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned TLS activation is allowed only in disposable Windows CI.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[A-Fa-f0-9]{40}$') {
        throw 'Publisher certificate must be approved independently.'
    }
    $sig=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($sig.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $sig.SignerCertificate -or
        $sig.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
          $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'New PostgreSQL TLS activation requires a signed helper.'
    }
}
if ($PostgresServiceName -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or
    $PostgresServiceAccount -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,30}$') {
    throw 'PostgreSQL service/account names are invalid.'
}
function Assert-Local([string]$path,[bool]$directory) {
    if (-not [IO.Path]::IsPathRooted($path) -or $path.StartsWith('\\')) {
        throw 'Only absolute local disk paths are permitted.'
    }
    $full=[IO.Path]::GetFullPath($path).TrimEnd('\')
    $cursor=$full
    while ($cursor) {
        $item=Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Junctions and linked PostgreSQL paths are prohibited.'
        }
        $next=Split-Path -Parent $cursor
        if ($next -eq $cursor) { break }
        $cursor=$next
    }
    if ((Get-Item -LiteralPath $full).PSIsContainer -ne $directory) {
        throw 'PostgreSQL policy path has the wrong file type.'
    }
    return $full
}
$data=Assert-Local $PostgresDataDir $true
$policy=Assert-Local $TlsPolicyDir $true
$marker=Assert-Local $FreshClusterMarker $false
$markerDir=[IO.Path]::GetDirectoryName($marker)
if (-not $UnsignedDisposableCIFixture) {
    $programDataBase=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')+'\'
    if (-not $data.StartsWith($programDataBase,[StringComparison]::OrdinalIgnoreCase) -or
        -not $policy.StartsWith($programDataBase,[StringComparison]::OrdinalIgnoreCase) -or
        -not $marker.StartsWith($programDataBase,[StringComparison]::OrdinalIgnoreCase)) {
        throw 'PostgreSQL TLS first installation must stay below ProgramData\Esterium.'
    }
}
if ($markerDir -eq $data -or $markerDir -eq $policy) {
    throw 'Fresh cluster attestation cannot be stored in mutable PostgreSQL data or certificate directories.'
}
if ([IO.File]::ReadAllText((Join-Path $data 'PG_VERSION')).Trim() -cne '16') {
    throw 'TLS staging is restricted to a PostgreSQL 16 cluster.'
}
$record=[IO.File]::ReadAllText($marker).Trim()
$fields=$record -split [char]10
if ($fields.Count -ne 3 -or
    $fields[0].Trim() -cne 'ESTERIUM_FRESH_POSTGRES_16' -or
    $fields[1].Trim() -cne $PostgresServiceName -or
    -not [string]::Equals($fields[2].Trim(),$data,[StringComparison]::OrdinalIgnoreCase)) {
    throw 'New-cluster attestation does not match the selected PostgreSQL service and data directory.'
}
$liveConfig=Assert-Local (Join-Path $data 'postgresql.conf') $false
$liveHba=Assert-Local (Join-Path $data 'pg_hba.conf') $false
$template=Assert-Local (Join-Path $policy 'postgresql.conf') $false
$templateHba=Assert-Local (Join-Path $policy 'pg_hba.conf') $false
$cert=Assert-Local (Join-Path $policy 'certificates\server.crt') $false
$key=Assert-Local (Join-Path $policy 'certificates\server.key') $false
$ca=Assert-Local (Join-Path $policy 'certificates\root.crt') $false
foreach ($p in @($liveConfig,$liveHba,$template,$templateHba,$cert,$key,$ca)) {
    $size=(Get-Item -LiteralPath $p -Force).Length
    if ($size -le 0 -or $size -gt 1048576) {
        throw 'PostgreSQL TLS file must be nonempty and at most 1 MiB.'
    }
}
$expectedCert=(Join-Path $policy 'certificates\server.crt').Replace('\','/')
$expectedKey=(Join-Path $policy 'certificates\server.key').Replace('\','/')
$configuration=[IO.File]::ReadAllText($template)
if (-not $configuration.Contains("ssl_cert_file = '$expectedCert'") -or
    -not $configuration.Contains("ssl_key_file = '$expectedKey'") -or
    -not $configuration.Contains("port = $PostgresPort")) {
    throw 'TLS policy template does not refer to approved local cert/key and port.'
}
& (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $template -PgHbaConf $templateHba | Out-Null
# Refuse any existing first-install backup/receipt; original files remain
# available for manual recovery if a later runtime preflight fails.
$receipt=Join-Path $markerDir ('PostgresTlsApplied-'+$PostgresServiceName+'.receipt')
$originalConf=Join-Path $markerDir ('OriginalPostgresql-'+$PostgresServiceName+'.conf')
$originalHba=Join-Path $markerDir ('OriginalPgHba-'+$PostgresServiceName+'.conf')
foreach ($p in @($receipt,$originalConf,$originalHba)) {
    if (Test-Path -LiteralPath $p) {
        throw 'PostgreSQL configuration already staged or an interrupted first install requires inspection.'
    }
}
if (-not $Apply) {
    Write-Output 'PLAN: validate new disabled/stopped PostgreSQL 16 service and replace only initial conf/HBA files.'
    Write-Output 'PLAN: keep original configuration backups and leave PostgreSQL disabled; no SQL or network calls.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator required for PostgreSQL TLS first-install staging.'
}
if (-not $UnsignedDisposableCIFixture) {
    $services=@(Get-CimInstance Win32_Service -Filter "Name='$PostgresServiceName'" -ErrorAction Stop)
    if ($services.Count -ne 1 -or $services[0].State -cne 'Stopped' -or
        $services[0].StartMode -cne 'Disabled' -or [int]$services[0].ProcessId -ne 0) {
        throw 'PostgreSQL must be fully stopped and disabled before TLS configuration.'
    }
    $identity=[string]$services[0].StartName
    if ($identity -notin @($PostgresServiceAccount,('.\'+$PostgresServiceAccount),($env:COMPUTERNAME+'\'+$PostgresServiceAccount))) {
        throw 'PostgreSQL Windows service does not have the approved dedicated identity.'
    }
}
if (@(Get-NetTCPConnection -LocalPort $PostgresPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'PostgreSQL port is already accepting connections: refuse offline TLS configuration.'
}
$stagedConf=Join-Path $data ('.esterium-tls-'+[Guid]::NewGuid().ToString('N')+'.conf')
$stagedHba=Join-Path $data ('.esterium-hba-'+[Guid]::NewGuid().ToString('N')+'.conf')
$changedConf=$false
$changedHba=$false
try {
    # The original EDB files are not deleted. File.Replace creates a backup
    # atomically, even if the computer crashes between the two config swaps.
    Copy-Item -LiteralPath $template -Destination $stagedConf -ErrorAction Stop
    Copy-Item -LiteralPath $templateHba -Destination $stagedHba -ErrorAction Stop
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $stagedConf -PgHbaConf $stagedHba | Out-Null
    [IO.File]::Replace($stagedConf,$liveConfig,$originalConf)
    $changedConf=$true
    [IO.File]::Replace($stagedHba,$liveHba,$originalHba)
    $changedHba=$true
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $liveConfig -PgHbaConf $liveHba | Out-Null
    $file=[IO.File]::Open($receipt,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try {
        $payload=[Text.UTF8Encoding]::new($false).GetBytes(
            'TLS_POLICY_STAGED_NEVER_STARTED'+[Environment]::NewLine+$data+[Environment]::NewLine)
        $file.Write($payload,0,$payload.Length)
        $file.Flush($true)
    } finally { $file.Dispose() }
    # A receipt is not a permission to start. Runtime TLS handshake,
    # SQL account, schema, cache and backup gates remain mandatory.
    Write-Output 'PASS: initial PostgreSQL TLS/SCRAM files atomically staged with original config backups.'
    Write-Output 'NOT ENABLED: service remains Disabled/Stopped; no PostgreSQL database is certified.'
} catch {
    if ($changedHba -and (Test-Path -LiteralPath $originalHba)) {
        try { Copy-Item -LiteralPath $originalHba -Destination $liveHba -Force -ErrorAction Stop }
        catch { Write-Warning 'Manual recovery required for original pg_hba.conf.' }
    }
    if ($changedConf -and (Test-Path -LiteralPath $originalConf)) {
        try { Copy-Item -LiteralPath $originalConf -Destination $liveConfig -Force -ErrorAction Stop }
        catch { Write-Warning 'Manual recovery required for original postgresql.conf.' }
    }
    throw
} finally {
    foreach ($p in @($stagedConf,$stagedHba)) {
        if (Test-Path -LiteralPath $p) { Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue }
    }
}
