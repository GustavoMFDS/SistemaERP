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
    'tools/provision-esterium-app-secrets.ps1',
    'tools/verify-esterium-bundle.ps1',
    'tools/postgres-daily-backup.ps1',
    'tools/copy-verified-external-backup.ps1',
    'tools/check-esterium-backup-integrity.ps1',
    'tools/esterium-migrate.exe',
    'README_LOCAL.md'
)
$items = @(Get-ChildItem -LiteralPath $root -Recurse -File -Force)
$records = @()
foreach ($file in $items) {
    if (($file.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Bundle may not contain symbolic links or junctions.'
    }
    $relative = $file.FullName.Substring($root.Length + 1).Replace('\', '/')
    if ($relative.StartsWith('migrations/', [StringComparison]::Ordinal) -and
        $relative -notmatch '^migrations/[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$') {
        throw "Unexpected migration file: $relative"
    }
    if ($relative -ne 'esterium-server.exe' -and -not $relative.StartsWith('web/', [StringComparison]::Ordinal) -and
        -not $relative.StartsWith('migrations/', [StringComparison]::Ordinal) -and
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
$migrationFiles = @($records | Where-Object { $_.path -match '^migrations/[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$' })
$ups = @($migrationFiles | Where-Object { $_.path.EndsWith('.up.sql') })
if ($ups.Count -eq 0 -or $migrationFiles.Count -ne 2 * $ups.Count) {
    throw 'Bundle requires matched database migration files.'
}
$versions = @{}
foreach ($up in $ups) {
    $version = [int]$up.path.Substring('migrations/'.Length, 4)
    if ($versions.ContainsKey($version)) {
        throw "Duplicate SQL migration version: $version"
    }
    $versions[$version] = $true
    $down = $up.path.Replace('.up.sql', '.down.sql')
    if (-not ($migrationFiles | Where-Object { $_.path -eq $down })) {
        throw "Database migration pair is missing: $($up.path)"
    }
}
for ($version = 1; $version -le $ups.Count; $version++) {
    if (-not $versions.ContainsKey($version)) {
        throw "Missing or non-contiguous SQL migration version: $version"
    }
}
$payload = @{ schema = 1; files = @($records | Sort-Object -Property path) } | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText($manifest, $payload + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
Write-Output 'PASS: bundle SHA-256 manifest generated (not a digital signature).'
