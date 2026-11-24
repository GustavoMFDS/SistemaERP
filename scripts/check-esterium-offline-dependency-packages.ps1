#requires -Version 5.1
<#
  READ-ONLY gate for independently approved offline Windows prerequisites.
  Does NOT download, unpack, install, execute or trust any vendor package.
  Signed executables require the approved Authenticode publisher identity.
  The official PostgreSQL binary ZIP is accepted only at its fixed upstream SHA.
#>
param(
    [Parameter(Mandatory = $true)][string]$PostgresInstaller,
    [Parameter(Mandatory = $true)][string]$ExpectedPostgresSHA256,
    [string]$ExpectedPostgresPublisherThumbprint = '',
    [Parameter(Mandatory = $true)][string]$GarnetPackage,
    [Parameter(Mandatory = $true)][string]$ExpectedGarnetSHA256,
    [string]$ExpectedGarnetPublisherThumbprint = '',
    [switch]$SyntheticUnsignedCIFixture,
    [switch]$PostgresBinaryZip
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') {
    throw 'Offline dependency source verification requires Windows.'
}
if ($SyntheticUnsignedCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI -ne '1')) {
    throw 'Unsigned prerequisite fixture is restricted to disposable hosted Windows CI.'
}
foreach ($digest in @($ExpectedPostgresSHA256, $ExpectedGarnetSHA256)) {
    if ($digest -cnotmatch '^[a-fA-F0-9]{64}$') {
        throw 'A trusted, independently approved SHA-256 is required for each prerequisite.'
    }
}
$officialPostgresZipHash='43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'
if ($PostgresBinaryZip) {
    if ($ExpectedPostgresPublisherThumbprint) {
        throw 'A PostgreSQL ZIP cannot claim an EDB Authenticode publisher.'
    }
    if ($ExpectedPostgresSHA256.ToUpperInvariant() -cne $officialPostgresZipHash) {
        throw 'PostgreSQL ZIP requires the independently pinned official SHA-256.'
    }
} elseif ($ExpectedPostgresPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
    throw 'An independently approved PostgreSQL publisher certificate thumbprint is mandatory.'
}
function Resolve-ApprovedLocalFile([string]$name, [string]$value) {
    if (-not [IO.Path]::IsPathRooted($value) -or $value.StartsWith('\\')) {
        throw "$name must be an absolute local file path, not a network share."
    }
    $path = (Resolve-Path -LiteralPath $value -ErrorAction Stop).Path
    $item = Get-Item -LiteralPath $path -Force
    if ($item.PSIsContainer -or $item.Length -le 0 -or
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "$name must be a nonempty ordinary file without symlinks."
    }
    $folder = Split-Path -Parent $path
    while (-not [string]::IsNullOrWhiteSpace($folder)) {
        $parent = Get-Item -LiteralPath $folder -Force
        if (($parent.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "$name has a symlinked parent directory."
        }
        $previous = Split-Path -Parent $folder
        if ($previous -eq $folder) { break }
        $folder = $previous
    }
    if ($path.StartsWith($env:ProgramFiles.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase) -or
        $path.StartsWith($env:ProgramData.TrimEnd('\') + '\Esterium\Secrets\',
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "$name must be staged outside installed programs and protected credentials."
    }
    return $path
}
$postgres = Resolve-ApprovedLocalFile 'PostgreSQL package' $PostgresInstaller
$garnet = Resolve-ApprovedLocalFile 'Garnet package' $GarnetPackage
if ([string]::Equals($postgres, $garnet, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'PostgreSQL and Garnet must have separate approved artifacts.'
}
if ($PostgresBinaryZip) {
    if ([IO.Path]::GetExtension($postgres).ToLowerInvariant() -ne '.zip') {
        throw 'Official PostgreSQL binary package must be a ZIP.'
    }
} elseif ([IO.Path]::GetExtension($postgres).ToLowerInvariant() -ne '.exe') {
    throw 'PostgreSQL Windows installer must be an approved signed .exe.'
}
$garnetExt = [IO.Path]::GetExtension($garnet).ToLowerInvariant()
if ($garnetExt -notin @('.zip', '.exe')) {
    throw 'Garnet package must be a separately approved signed .exe or reviewed ZIP.'
}
if ($garnetExt -eq '.exe' -and
    $ExpectedGarnetPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
    throw 'An independently approved Garnet executable signer thumbprint is required.'
}
if ($garnetExt -eq '.zip' -and $ExpectedGarnetPublisherThumbprint) {
    throw 'A ZIP has no Authenticode publisher signature; review its release authenticity separately.'
}
$hashes = @(
    @{ path=$postgres; expected=$ExpectedPostgresSHA256 },
    @{ path=$garnet; expected=$ExpectedGarnetSHA256 }
)
foreach ($artifact in $hashes) {
    $hash = (Get-FileHash -LiteralPath $artifact.path -Algorithm SHA256).Hash
    if ($hash -cne $artifact.expected.ToUpperInvariant()) {
        throw 'An offline prerequisite differs from the independently approved SHA-256.'
    }
}
function Assert-ApprovedSignature([string]$path, [string]$thumbprint) {
    $signature = Get-AuthenticodeSignature -LiteralPath $path
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne $thumbprint.ToUpperInvariant()) {
        throw 'A prerequisite executable lacks the independently approved Authenticode signer.'
    }
}
if (-not $SyntheticUnsignedCIFixture) {
    if (-not $PostgresBinaryZip) { Assert-ApprovedSignature $postgres $ExpectedPostgresPublisherThumbprint }
    if ($garnetExt -eq '.exe') {
        Assert-ApprovedSignature $garnet $ExpectedGarnetPublisherThumbprint
    }
}
if ($PostgresBinaryZip) {
    if ($ExpectedPostgresSHA256.ToUpperInvariant() -cne $officialPostgresZipHash) {
        throw 'PostgreSQL binary ZIP SHA-256 is not the fixed official artifact.'
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive=[IO.Compression.ZipFile]::OpenRead($postgres)
    try {
        if ($archive.Entries.Count -lt 5 -or $archive.Entries.Count -gt 30000) {
            throw 'The official PostgreSQL ZIP has an invalid entry count.'
        }
        $required=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
        foreach ($name in @('pgsql/bin/postgres.exe','pgsql/bin/initdb.exe','pgsql/bin/pg_ctl.exe',
            'pgsql/bin/psql.exe','pgsql/bin/pg_isready.exe')) { [void]$required.Add($name) }
        $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        $total=[int64]0
        foreach ($entry in $archive.Entries) {
            $name=$entry.FullName.Replace('\','/')
            $normalized=$name.TrimEnd('/')
            $parts=@($normalized.Split('/'))
            if ([string]::IsNullOrWhiteSpace($normalized) -or
                -not ($normalized -ceq 'pgsql' -or $normalized.StartsWith('pgsql/',[StringComparison]::Ordinal)) -or
                $name.StartsWith('/') -or $name.Contains(':') -or $name -match '[\x00-\x1f]' -or
                $name.Length -gt 200 -or -not $seen.Add($normalized) -or
                ($parts.Count -lt 2 -and -not ($normalized -ceq 'pgsql' -and $name.EndsWith('/'))) -or
                (([int64]$entry.ExternalAttributes -shr 16) -band 0xF000) -eq 0xA000) {
                throw 'PostgreSQL ZIP contains an unsafe or unexpected path.'
            }
            foreach ($part in $parts) {
                if ($part -in @('','.','..') -or $part.EndsWith('.') -or $part.EndsWith(' ') -or
                    $part -match '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$') {
                    throw 'PostgreSQL ZIP contains an unsafe path segment.'
                }
            }
            if ($entry.Length -gt 512MB -or $entry.Length -lt 0) {
                throw 'PostgreSQL ZIP contains an oversized entry.'
            }
            $total += [int64]$entry.Length
            if ($total -gt (1300MB)) { throw 'PostgreSQL ZIP expanded size exceeds the approved limit.' }
            if ($entry.Length -gt 1MB -and ($entry.CompressedLength -eq 0 -or
                ($entry.Length / [Math]::Max(1,$entry.CompressedLength)) -gt 200)) {
                throw 'PostgreSQL ZIP has an excessive compression ratio.'
            }
            [void]$required.Remove($normalized)
        }
        if ($required.Count -ne 0) { throw 'PostgreSQL ZIP is missing required server tools.' }
    } finally { $archive.Dispose() }
}
if ($garnetExt -eq '.zip') {
    # Inspect ZIP metadata without extracting untrusted vendor files. SHA-256
    # proves only byte agreement, NOT Microsoft publisher authenticity.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($garnet)
    try {
        if ($archive.Entries.Count -eq 0 -or $archive.Entries.Count -gt 2000) {
            throw 'The ZIP has an invalid or excessive number of entries.'
        }
        $totalUncompressed = [int64]0
        $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        foreach ($entry in $archive.Entries) {
            $path = $entry.FullName.Replace('\','/')
            $normalized = $path.TrimEnd('/')
            if ([string]::IsNullOrWhiteSpace($normalized) -or $path.StartsWith('/') -or
                $path.Contains(':') -or $path -match '[\x00-\x1f]' -or
                $path.Length -gt 260) {
                throw 'The offline ZIP contains an unsafe path or excessive entry size.'
            }
            if (-not $seen.Add($normalized)) {
                throw 'The offline ZIP contains duplicate or case-colliding entries.'
            }
            $segments = @($normalized.Split('/'))
            foreach ($segment in $segments) {
                if ($segment -in @('.', '..', '') -or
                    $segment.EndsWith('.') -or $segment.EndsWith(' ') -or
                    $segment -match '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$') {
                    throw 'The offline ZIP contains an unsafe path or excessive entry size.'
                }
            }
            # POSIX symbolic links are represented by S_IFLNK in the ZIP mode.
            # Do not accept link payloads even though this gate never extracts.
            $posixType = (([int64]$entry.ExternalAttributes -shr 16) -band 0xF000)
            if ($posixType -eq 0xA000) {
                throw 'The offline ZIP contains a symbolic-link entry.'
            }
            if ($entry.Length -gt (512MB) -or $entry.Length -lt 0) {
                throw 'The offline ZIP contains an unsafe path or excessive entry size.'
            }
            $totalUncompressed += [int64]$entry.Length
            if ($totalUncompressed -gt (2GB)) {
                throw 'The offline ZIP exceeds the allowed total expanded size.'
            }
            if ($entry.Length -gt (1MB) -and
                ($entry.CompressedLength -eq 0 -or
                    ($entry.Length / [Math]::Max(1, $entry.CompressedLength)) -gt 200)) {
                throw 'The offline ZIP has an excessive compression ratio.'
            }
        }
    } finally { $archive.Dispose() }
}
Write-Output 'PASS: offline prerequisite package hashes, file paths and applicable publisher checks passed without executing software.'
if ($garnetExt -eq '.zip') {
    Write-Output 'LIMITATION: a ZIP hash does not establish publisher authenticity; independent Microsoft release review is still required before installation.'
}
if ($SyntheticUnsignedCIFixture) {
    Write-Output 'SYNTHETIC CI ONLY: executable Authenticode checks bypassed; NEVER a production approval.'
}
Write-Output 'NOT INSTALLED: PostgreSQL, Garnet, TLS, Windows services, database schema or SEFAZ.'
