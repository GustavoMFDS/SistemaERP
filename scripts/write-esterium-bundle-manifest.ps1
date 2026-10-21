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
# Verify all directories before hashing a single payload. Get-ChildItem -File
# alone can miss directory junctions, and a link under web/ might point to
# material outside the signed build staging directory.
if (((Get-Item -LiteralPath $root -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw 'Bundle root must not be a symbolic link or junction.'
}
foreach ($entry in @(Get-ChildItem -LiteralPath $root -Recurse -Force)) {
    if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Bundle may not contain symbolic links or junctions.'
    }
}
if (Test-Path -LiteralPath $manifest) { throw 'Bundle manifest already exists; refusing to overwrite.' }
$requiredTools = @(
    'tools/register-esterium-windows-service.ps1',
    'tools/provision-esterium-app-secrets.ps1',
    'tools/verify-esterium-bundle.ps1',
    'tools/postgres-daily-backup.ps1',
    'tools/copy-verified-external-backup.ps1',
    'tools/check-esterium-backup-integrity.ps1',
    'tools/run-esterium-scheduled-backup.ps1',
    'tools/register-esterium-backup-task.ps1',
    'tools/check-esterium-windows-host.ps1',
    'tools/unregister-esterium-windows-service.ps1',
    'tools/check-esterium-local-dependencies.ps1',
    'tools/plan-esterium-windows-upgrade.ps1',
    'tools/plan-esterium-local-setup.ps1',
    'tools/install-esterium-local.ps1',
    'tools/apply-esterium-app-update.ps1',
    'tools/check-esterium-signed-release.ps1',
    'tools/check-esterium-offline-dependency-packages.ps1',
    'tools/provision-esterium-garnet-config.ps1',
    'tools/provision-esterium-postgres-optionfile.ps1',
    'tools/prepare-esterium-local-dependency-configs.ps1',
    'tools/install-esterium-local-dependencies.ps1',
    'tools/provision-esterium-dependency-bootstrap.ps1',
    'tools/setup-esterium-local-dependencies.ps1',
    'tools/check-esterium-postgres-tls-policy.ps1',
    'tools/provision-esterium-postgres-tls-policy.ps1',
    'tools/stage-esterium-postgres-tls.ps1',
    'tools/probe-esterium-local-first-run.ps1',
    'tools/bootstrap-esterium-local-schema.ps1',
    'tools/stage-esterium-approved-windows-release.ps1',
    'tools/initialize-esterium-postgres-app-db.ps1',
    'tools/apply-esterium-initial-schema.ps1',
    'tools/esterium-migrate.exe',
    'tools/esterium-pg-tls-certgen.exe',
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
    # Even web/ is an allowed output subtree: never publish private keys,
    # certificate containers, env files or database archives as static assets.
    if ($relative -match '(^|/)(\.env(\.[^/]*)?|\.pgpass|id_rsa|id_ed25519|[^/]*\.(pem|key|pfx|p12|p8|pvk|jks|keystore|kdbx|crt|cer|der|db|sqlite|sqlite3|secret|dump|backup|options|rdb|aof))($|/)') {
        throw 'Bundle contains a file resembling secret, certificate or database material.'
    }
    # A web/ asset is publicly served by the local UI: keep unknown
    # extensions out, even if a secret is disguised as harmless JSON/TXT.
    # Adjust this allowlist deliberately if the compiled frontend changes.
    if ($relative.StartsWith('web/',[StringComparison]::Ordinal)) {
        $ext=[IO.Path]::GetExtension($relative).ToLowerInvariant()
        $webAssetExtensions=@('.html','.js','.mjs','.css','.svg','.png','.ico',
            '.webmanifest','.jpg','.jpeg','.gif','.webp','.avif','.woff',
            '.woff2','.ttf','.otf','.wasm')
        if ($ext -notin $webAssetExtensions) {
            throw 'Web bundle includes an unsupported static asset extension.'
        }
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
