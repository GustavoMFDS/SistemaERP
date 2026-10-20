#requires -Version 5.1
<#
  Fail-closed publisher authenticity gate for an approved Windows release.
  Expected SHA-256 and signer thumbprint must come from an INDEPENDENT,
  trusted approval channel, never from the same untrusted ZIP.
  This script is read-only and does not perform code signing.
#>
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [Parameter(Mandatory = $true)][string]$ExpectedManifestSHA256,
    [Parameter(Mandatory = $true)][string]$ExpectedPublisherThumbprint
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Approved release verification requires Windows.' }
if ($ExpectedManifestSHA256 -notmatch '^[a-fA-F0-9]{64}$' -or
    $ExpectedPublisherThumbprint -notmatch '^[a-fA-F0-9]{40}$') {
    throw 'Release approval requires a valid SHA-256 and SHA-1 certificate thumbprint.'
}
$bundle = (Resolve-Path -LiteralPath $BundleDir -ErrorAction Stop).Path
& (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
$manifest = Join-Path $bundle 'bundle-manifest.json'
$hash = (Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash
if ($hash -cne $ExpectedManifestSHA256.ToUpperInvariant()) {
    throw 'Bundle manifest differs from the independently approved SHA-256.'
}
$signer = $ExpectedPublisherThumbprint.ToUpperInvariant()
# The complete executable and every script that can execute privileged
# operations must be Authenticode-valid under the same approved publisher.
$targets = @(
    (Join-Path $bundle 'esterium-server.exe'),
    (Join-Path $bundle 'tools\esterium-migrate.exe')
)
$targets += @(Get-ChildItem -LiteralPath (Join-Path $bundle 'tools') -Filter '*.ps1' -File |
    ForEach-Object { $_.FullName })
foreach ($file in $targets) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
        throw 'Required executable or installer maintenance script is missing.'
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $file
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $signer) {
        # No file content, certificate details or untrusted paths in diagnostics.
        throw 'Release refused: an executable or privileged maintenance script lacks approved Authenticode signature.'
    }
}
Write-Output 'PASS: approved manifest hash and code-signing identity match all executables and maintenance scripts.'
Write-Output 'NOT INCLUDED: peripheral certification, database migrations, actual SEFAZ authorization or physical pilot acceptance.'
