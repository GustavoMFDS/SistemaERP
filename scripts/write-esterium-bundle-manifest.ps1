#requires -Version 5.1
<#
  Make a SHA-256 manifest for a prebuilt Esterium Windows bundle.
  This detects accidental damage, not package provenance/signature.
#>
param([Parameter(Mandatory=$true)][string]$BundleDir)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = (Resolve-Path -LiteralPath $BundleDir).Path.TrimEnd('\','/')
$manifest = Join-Path $root 'bundle-manifest.json'
if (Test-Path -LiteralPath $manifest) { throw 'Bundle manifest already exists; refusing to overwrite.' }
$requiredTools = @(
    'tools/register-esterium-windows-service.ps1',
    'tools/verify-esterium-bundle.ps1',
    'tools/postgres-daily-backup.ps1',
    'tools/copy-verified-external-backup.ps1',
    'README_LOCAL.md'
)
$items = @(Get-ChildItem -LiteralPath $root -Recurse -File -Force)
$records = @()
foreach ($file in $items) {
    if (($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Bundle may not contain symbolic links or junctions.'
    }
    $relative = $file.FullName.Substring($root.Length + 1).Replace('\', '/')
    if ($relative -ne 'esterium-server.exe' -and -not $relative.StartsWith('web/', [StringComparison]::Ordinal) -and
        -not ($requiredTools -contains $relative)) {
        throw "Unexpected bundle file: $relative"
    }
    if ($relative -match '(^|/)(\.env|.*\.pem|.*\.key|.*\.db|.*\.sqlite|.*\.secret)(/|$)') {
        throw 'Bundle contains a file resembling secret/database material.'
    }
    $records += @{
        path = $relative
        sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    }
}
if (-not ($records | Where-Object { $_.path -eq 'esterium-server.exe' }) -or
    -not ($records | Where-Object { $_.path -eq 'web/index.html' })) {
    throw 'Bundle requires esterium-server.exe and web/index.html'
}
foreach ($needed in $requiredTools) {
    if (-not ($records | Where-Object { $_.path -eq $needed })) {
        throw "Incomplete local tools bundle: $needed"
    }
}
$payload = @{ schema = 1; files = @($records | Sort-Object -Property path) } | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText($manifest, $payload + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
Write-Output 'PASS: bundle SHA-256 manifest generated (not a digital signature).'
