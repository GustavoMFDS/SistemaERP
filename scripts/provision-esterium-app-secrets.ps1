#requires -Version 5.1
<#
  Provision only application-owned JWT and metrics tokens.
  Never provisions database/Redis credentials, never prints secrets.
  Default is dry-run; -Apply is required for any write.
#>
param(
    [string]$DestinationDir = (Join-Path $env:ProgramData 'Esterium\Secrets'),
    [switch]$Apply,
    [string]$TrustedPublisherThumbprint = '',
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT') { throw 'This helper requires Windows.' }
# A direct -Apply bypasses the signed orchestration script. Each helper
# therefore authenticates its own publisher before reading private inputs
# or creating new credential/configuration files.
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SECRET_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned configuration fixture requires isolated GitHub-hosted Windows CI.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Application secret creation requires an independently approved publisher identity.'
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
            $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Application secret creation requires the original helper signed by the approved publisher.'
    }
}

$base = [IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target = [IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
if (-not $target.StartsWith($base + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Destination must be a new folder under ProgramData\Esterium.'
}
if (Test-Path -LiteralPath $target) {
    throw 'Destination already exists; refusing to overwrite any credentials.'
}
$parent = Split-Path -Parent $target
$cursor = $parent
while ($cursor.Length -ge $base.Length) {
    if (Test-Path -LiteralPath $cursor) {
        $item = Get-Item -LiteralPath $cursor -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Credential destination parent must not be a junction or symbolic link.'
        }
    }
    $cursor = Split-Path -Parent $cursor
    if ([string]::IsNullOrWhiteSpace($cursor)) { break }
}
if (-not $Apply) {
    Write-Output 'PLAN: create two random application credentials in a new restricted folder.'
    Write-Output 'PLAN: JWT_SECRET_FILE=jwt.secret, METRICS_BEARER_TOKEN_FILE=metrics.secret.'
    Write-Output 'PLAN: allow LocalService read; administrators and SYSTEM full access.'
    Write-Output 'PLAN: PostgreSQL/Redis connection credentials are not created or changed.'
    Write-Output 'PLAN: no files or services changed. Use -Apply only after reviewing.'
    return
}
$current = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($current)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Elevated administrator privileges are required to create protected credentials.'
}
$icacls = Join-Path $env:SystemRoot 'System32\icacls.exe'
if (-not (Test-Path -LiteralPath $icacls)) { throw 'icacls.exe unavailable.' }

[IO.Directory]::CreateDirectory($parent) | Out-Null
$staging = Join-Path $parent ('.esterium-secrets-' + [Guid]::NewGuid().ToString('N'))
$renamed = $false
try {
    New-Item -ItemType Directory -Path $staging -ErrorAction Stop | Out-Null
    & $icacls $staging '/inheritance:r' '/grant:r' '*S-1-5-19:(OI)(CI)RX' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to secure the credential staging folder.' }
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        foreach ($filename in @('jwt.secret', 'metrics.secret')) {
            $bytes = [byte[]]::new(48)
            $rng.GetBytes($bytes)
            $value = [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
            $file = Join-Path $staging $filename
            $handle = [IO.File]::Open($file, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try {
                $blob = [Text.UTF8Encoding]::new($false).GetBytes($value + [Environment]::NewLine)
                $handle.Write($blob, 0, $blob.Length)
                $handle.Flush($true)
            } finally {
                $handle.Dispose()
            }
            & $icacls $file '/inheritance:r' '/grant:r' '*S-1-5-19:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "Failed to secure generated credential file: $filename" }
            [Array]::Clear($bytes, 0, $bytes.Length)
        }
    } finally {
        $rng.Dispose()
    }
    if (Test-Path -LiteralPath $target) { throw 'Another process created the destination.' }
    Move-Item -LiteralPath $staging -Destination $target -ErrorAction Stop
    $renamed = $true
    Write-Output 'PASS: generated two application credentials with restricted ACLs.'
    Write-Output 'Only file paths may be passed to the Windows service. No credential values are printed.'
} finally {
    if (-not $renamed -and (Test-Path -LiteralPath $staging)) {
        Remove-Item -LiteralPath $staging -Recurse -Force
    }
}
