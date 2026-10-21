#requires -Version 5.1
<#
  Build strict PostgreSQL 16 localhost TLS/SCRAM policy templates for a
  NEW cluster. Never edits a real postgresql.conf, pg_hba.conf, database or
  Windows service. Apply only publishes new ACL-restricted template files.
  The PostgreSQL TLS cert/key must be installed separately after verification.
#>
[CmdletBinding()]
param(
    [string]$DestinationDir=(Join-Path $env:ProgramData 'Esterium\PostgresTlsPolicy'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'PostgreSQL policy templates require Windows.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_POLICY_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned policy fixture only supported in disposable Windows CI.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[0-9a-fA-F]{40}$') {
        throw 'PostgreSQL policy Apply requires an externally approved publisher certificate.'
    }
    $signature=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
        $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'PostgreSQL policy creator is not signed by the approved publisher.'
    }
}
if ($PostgresPort -in @(6379,8080)) { throw 'Reserved Esterium app/cache port.' }
if (-not [IO.Path]::IsPathRooted($DestinationDir) -or $DestinationDir.StartsWith('\\')) {
    throw 'TLS policy destination must use a local absolute path.'
}
$base=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target=[IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
if (-not $target.StartsWith($base+'\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'TLS policy must reside in ProgramData\Esterium.'
}
if (Test-Path -LiteralPath $target) {
    throw 'Existing PostgreSQL policy files must not be overwritten.'
}
$cursor=Split-Path -Parent $target
while ($cursor) {
    if (Test-Path -LiteralPath $cursor) {
        if (((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'PostgreSQL policy directory ancestors must not be links.'
        }
    }
    $next=Split-Path -Parent $cursor
    if ($next -eq $cursor) { break }
    $cursor=$next
}
$postgresConfig=@(
    "# Esterium: NEW-CLUSTER policy template only. Not yet installed."
    "listen_addresses = 'localhost'"
    "port = $PostgresPort"
    "ssl = on"
    "ssl_min_protocol_version = 'TLSv1.2'"
    "ssl_cert_file = 'server.crt'"
    "ssl_key_file = 'server.key'"
    "password_encryption = 'scram-sha-256'"
) -join [Environment]::NewLine
$hbaConfig=@(
    '# Reject cleartext and nonlocal authentication regardless of database or user.'
    'hostnossl all all 0.0.0.0/0 reject'
    'hostnossl all all ::/0 reject'
    'hostssl all all 127.0.0.1/32 scram-sha-256'
    'hostssl all all ::1/128 scram-sha-256'
) -join [Environment]::NewLine
if (-not $Apply) {
    Write-Output 'PLAN: new immutable PostgreSQL 16 TLSv1.2+ SCRAM-only localhost policy templates.'
    Write-Output 'PLAN: no PostgreSQL config, service, data, certificate or port will be changed.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Administrator permission required for protected PostgreSQL templates.'
}
$parent=Split-Path -Parent $target
[IO.Directory]::CreateDirectory($parent) | Out-Null
$stage=Join-Path $parent ('.esterium-pg-tls-'+[Guid]::NewGuid().ToString('N'))
$published=$false
$icacls=Join-Path $env:SystemRoot 'System32\icacls.exe'
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    & $icacls $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot protect staged PostgreSQL policy.' }
    $pg=Join-Path $stage 'postgresql.conf'
    $hba=Join-Path $stage 'pg_hba.conf'
    $encoding=[Text.UTF8Encoding]::new($false)
    foreach ($record in @(@{path=$pg;text=$postgresConfig},@{path=$hba;text=$hbaConfig})) {
        $bytes=$encoding.GetBytes($record.text+[Environment]::NewLine)
        try {
            $handle=[IO.File]::Open($record.path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
            try { $handle.Write($bytes,0,$bytes.Length);$handle.Flush($true) }
            finally { $handle.Dispose() }
        } finally { [Array]::Clear($bytes,0,$bytes.Length) }
        & $icacls $record.path '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Cannot protect staged PostgreSQL policy file.' }
    }
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $pg -PgHbaConf $hba | Out-Null
    if (Test-Path -LiteralPath $target) { throw 'Another process created PostgreSQL policy destination.' }
    Move-Item -LiteralPath $stage -Destination $target -ErrorAction Stop
    $published=$true
    Write-Output 'PASS: new protected PostgreSQL 16 TLS/SCRAM policy templates created and statically checked.'
    Write-Output 'NOT APPLIED: live database config, server certificate/private key, startup, SQL roles, migrations or TLS verification.'
} finally {
    if (-not $published -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    }
}
