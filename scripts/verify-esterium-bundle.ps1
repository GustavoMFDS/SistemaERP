#requires -Version 5.1
<#
  Fail closed if a prebuilt Windows bundle differs from its manifest.
  A manifest INSIDE a bundle is not an authenticity guarantee.
#>
param([Parameter(Mandatory=$true)][string]$BundleDir)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = (Resolve-Path -LiteralPath $BundleDir).Path.TrimEnd('\','/')
$manifest = Join-Path $root 'bundle-manifest.json'
if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) {
    throw 'Bundle SHA-256 manifest is missing.'
}
$document = Get-Content -LiteralPath $manifest -Raw | ConvertFrom-Json
if ($null -eq $document -or $document.schema -ne 1 -or @($document.files).Count -lt 2) {
    throw 'Unsupported or empty bundle SHA-256 manifest.'
}
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
    'tools/check-esterium-postgres-tls-policy.ps1',
    'tools/esterium-migrate.exe',
    'README_LOCAL.md'
)
$seen = @{}
foreach ($record in @($document.files)) {
    $relative = [string]$record.path
    $expected = [string]$record.sha256
    if ($relative.StartsWith('migrations/', [StringComparison]::Ordinal) -and
        $relative -notmatch '^migrations/[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$') {
        throw 'Unexpected migration path in manifest.'
    }
    if ($relative -ne 'esterium-server.exe' -and
        -not $relative.StartsWith('web/', [StringComparison]::Ordinal) -and
        -not $relative.StartsWith('migrations/', [StringComparison]::Ordinal) -and
        -not ($requiredTools -contains $relative)) {
        throw 'Unexpected manifest path.'
    }
    if ($relative.Contains('\') -or $relative.Contains(':') -or
        @($relative.Split('/') | Where-Object { $_ -in @('', '.', '..') }).Count -gt 0) {
        throw 'Unsafe relative path in manifest.'
    }
    # Do not accept a correctly hashed bundle containing secrets under web/.
    # The verifier must enforce the same policy as the manifest writer.
    if ($relative -match '(^|/)(\.env(\.[^/]*)?|\.pgpass|id_rsa|id_ed25519|[^/]*\.(pem|key|pfx|p12|p8|pvk|jks|keystore|kdbx|crt|cer|der|db|sqlite|sqlite3|secret|dump|backup|options|rdb|aof))($|/)') {
        throw 'Bundle manifest includes secret, certificate or database material.'
    }
    # The verifier must not trust a forged, rehashed manifest containing
    # extra static assets that Vite would not normally emit.
    if ($relative.StartsWith('web/',[StringComparison]::Ordinal)) {
        $ext=[IO.Path]::GetExtension($relative).ToLowerInvariant()
        $webAssetExtensions=@('.html','.js','.mjs','.css','.svg','.png','.ico',
            '.webmanifest','.jpg','.jpeg','.gif','.webp','.avif','.woff',
            '.woff2','.ttf','.otf','.wasm')
        if ($ext -notin $webAssetExtensions) {
            throw 'Web bundle manifest includes an unsupported static asset extension.'
        }
    }
    if ($expected -notmatch '^[0-9a-fA-F]{64}$') { throw 'Invalid SHA-256 entry.' }
    if ($seen.ContainsKey($relative)) { throw 'Duplicate bundle manifest path.' }
    $target = Join-Path $root ($relative.Replace('/', '\'))
    if (-not (Test-Path -LiteralPath $target -PathType Leaf)) {
        throw "Missing bundle file: $relative"
    }
    $hash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne $expected.ToLowerInvariant()) {
        throw "Bundle file SHA-256 mismatch: $relative"
    }
    $seen[$relative] = $true
}
if (-not $seen.ContainsKey('esterium-server.exe') -or -not $seen.ContainsKey('web/index.html')) {
    throw 'Bundle manifest lacks executable or web/index.html.'
}
foreach ($needed in $requiredTools) {
    if (-not $seen.ContainsKey($needed)) {
        throw "Required installer helper missing: $needed"
    }
}
$migrations = @($seen.Keys | Where-Object { $_ -match '^migrations/[0-9]{4}_[a-z0-9_]+\.(up|down)\.sql$' })
$ups = @($migrations | Where-Object { $_.EndsWith('.up.sql') })
if ($ups.Count -eq 0 -or $migrations.Count -ne 2 * $ups.Count) {
    throw 'SQL migration files in bundle are incomplete.'
}
$versions = @{}
foreach ($up in $ups) {
    $version = [int]$up.Substring('migrations/'.Length, 4)
    if ($versions.ContainsKey($version)) {
        throw "Duplicate SQL migration version: $version"
    }
    $versions[$version] = $true
    if (-not $seen.ContainsKey($up.Replace('.up.sql', '.down.sql'))) {
        throw "Unpaired SQL migration: $up"
    }
}
for ($version = 1; $version -le $ups.Count; $version++) {
    if (-not $versions.ContainsKey($version)) {
        throw "Missing or non-contiguous SQL migration version: $version"
    }
}
foreach ($item in @(Get-ChildItem -LiteralPath $root -Recurse -Force)) {
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Bundle includes an unexpected symbolic link/junction.'
    }
    if ($item.PSIsContainer) { continue }
    $relative = $item.FullName.Substring($root.Length + 1).Replace('\', '/')
    if ($relative -ne 'bundle-manifest.json' -and -not $seen.ContainsKey($relative)) {
        throw "Unlisted file in bundle: $relative"
    }
}
Write-Output 'PASS: all bundle files match the SHA-256 manifest.'
