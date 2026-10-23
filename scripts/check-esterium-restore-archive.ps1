#requires -Version 5.1
<#
  READ-ONLY PostgreSQL recovery archive preflight for an independent Windows PC.
  Verifies archive digest + sidecar, independently approved pg_restore.exe bytes,
  PostgreSQL major version and a minimal Esterium SQL table inventory.
  NEVER connects to a server or invokes pg_restore with --dbname/--create.
  The full restore rehearsal must still use a disposable, isolated database.
#>
param(
    [Parameter(Mandatory = $true)][string]$ArchivePath,
    [Parameter(Mandatory = $true)][string]$ExpectedArchiveSHA256,
    [Parameter(Mandatory = $true)][string]$PgRestoreExe,
    [Parameter(Mandatory = $true)][string]$ExpectedPgRestoreSHA256,
    [switch]$SyntheticCIFixture
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Esterium archive inspection requires Windows.' }
if ($SyntheticCIFixture -and ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_RESTORE_DISPOSABLE_CI -ne '1')) {
    throw 'Synthetic restore fixtures require a disposable GitHub-hosted Windows runner.'
}
foreach ($value in @($ArchivePath, $PgRestoreExe)) {
    if (-not [IO.Path]::IsPathRooted($value)) {
        throw 'Archive and PostgreSQL tool paths must be absolute.'
    }
}
foreach ($hash in @($ExpectedArchiveSHA256, $ExpectedPgRestoreSHA256)) {
    if ($hash -cnotmatch '^[0-9a-fA-F]{64}$') {
        throw 'Archive and pg_restore hashes must be independently approved SHA-256 digests.'
    }
}

function Assert-PhysicalFile([string]$path, [string]$label) {
    $item = Get-Item -LiteralPath $path -Force -ErrorAction Stop
    if ($item.PSIsContainer -or
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
        $item.Length -le 0) {
        throw "Recovery $label must be a nonempty physical file."
    }
    # Reject a symlink/junction in ANY intermediate directory, not just
    # in the final file, to prevent redirecting approved bytes elsewhere.
    $folder = [IO.Path]::GetDirectoryName($item.FullName)
    $root = [IO.Path]::GetPathRoot($folder).TrimEnd('\', '/')
    while ($folder -and $folder.TrimEnd('\', '/') -ne $root) {
        $entry = Get-Item -LiteralPath $folder -Force -ErrorAction Stop
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Recovery $label path must not cross a junction or symbolic link."
        }
        $next = [IO.Path]::GetDirectoryName($folder.TrimEnd('\', '/'))
        if ($next -eq $folder) { break }
        $folder = $next
    }
    return $item.FullName
}

$archive = Assert-PhysicalFile $ArchivePath 'archive'
$name = [IO.Path]::GetFileName($archive)
if ($name -cnotmatch '^esterium-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}\.dump$') {
    throw 'Recovery archive name does not match a versioned Esterium backup.'
}
$checksumFile = Assert-PhysicalFile ($archive + '.sha256') 'checksum'
$expectedArchive = $ExpectedArchiveSHA256.ToLowerInvariant()
$recorded = (Get-Content -LiteralPath $checksumFile -Raw -ErrorAction Stop).Trim()
if ($recorded -cne ($expectedArchive + '  ' + $name)) {
    throw 'The backup checksum sidecar differs from independent approval.'
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expectedArchive) {
    throw 'Backup archive bytes disagree with independent approval.'
}

$pgRestore = Assert-PhysicalFile $PgRestoreExe 'pg_restore'
if ([IO.Path]::GetFileName($pgRestore) -cne 'pg_restore.exe') {
    throw 'Recovery inspection accepts only the approved pg_restore.exe executable.'
}
if (-not $SyntheticCIFixture) {
    $approvedDir = [IO.Path]::GetFullPath((Join-Path $env:ProgramFiles 'PostgreSQL\16\bin')).TrimEnd('\', '/')
    if (-not [string]::Equals([IO.Path]::GetDirectoryName($pgRestore), $approvedDir,
        [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Production recovery inspection requires installed PostgreSQL 16 in Program Files.'
    }
}
$approvedTool = $ExpectedPgRestoreSHA256.ToLowerInvariant()
function Assert-ApprovedPgRestore() {
    $actual = (Get-FileHash -LiteralPath $pgRestore -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $approvedTool) {
        throw 'pg_restore executable differs from independently approved SHA-256.'
    }
}
Assert-ApprovedPgRestore

$version = @(& $pgRestore '--version' 2>&1)
if ($LASTEXITCODE -ne 0 -or (@($version) -join ' ') -notmatch '^pg_restore \(PostgreSQL\) 16\.') {
    throw 'Recovery requires a working PostgreSQL 16 pg_restore executable.'
}
# Recheck approved bytes before the actual archive parser is executed.
Assert-ApprovedPgRestore
$entries = @(& $pgRestore '--list' $archive 2>&1)
if ($LASTEXITCODE -ne 0) {
    throw 'PostgreSQL rejected the backup archive directory; abort recovery planning.'
}
$listing = @($entries) -join [Environment]::NewLine
# A custom-format archive can have a valid header but be partial. Require
# critical business and migration table definitions, not just a success code.
foreach ($table in @('companies', 'users', 'products', 'product_images', 'sales', 'schema_migrations')) {
    $pattern = '(?m)^\s*\d+;\s+\d+\s+\d+\s+TABLE\s+public\s+' + [regex]::Escape($table) + '\s+'
    if ($listing -notmatch $pattern) {
        throw 'The inspected backup lacks required Esterium business/migration tables.'
    }
    # A schema-only dump has every TABLE definition but NO recoverable
    # table contents. Never approve it as a full recovery archive.
    $dataPattern = '(?m)^\s*\d+;\s+\d+\s+\d+\s+TABLE DATA\s+public\s+' + [regex]::Escape($table) + '\s+'
    if ($listing -notmatch $dataPattern) {
        throw 'The inspected backup lacks required table data entries; schema-only archives are not complete backups.'
    }
}
Write-Output 'PASS: independently approved PostgreSQL 16 archive and pg_restore validated; critical table inventory present.'
Write-Output 'READ ONLY: no database connection, restore, migration, service or customer data change was attempted.'
Write-Output 'NEXT: restore this verified archive into a NEW isolated PostgreSQL instance and compare the business/fiscal records.'
