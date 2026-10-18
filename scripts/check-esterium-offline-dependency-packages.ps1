#requires -Version 5.1
<#
  READ-ONLY gate for independently approved offline Windows prerequisites.
  Does NOT download, unpack, install, execute or trust any vendor package.
  Signed executables require the approved Authenticode publisher identity;
  a ZIP needs separate supplier acceptance before it may be deployed.
#>
param(
    [Parameter(Mandatory = $true)][string]$PostgresInstaller,
    [Parameter(Mandatory = $true)][string]$ExpectedPostgresSHA256,
    [Parameter(Mandatory = $true)][string]$ExpectedPostgresPublisherThumbprint,
    [Parameter(Mandatory = $true)][string]$GarnetPackage,
    [Parameter(Mandatory = $true)][string]$ExpectedGarnetSHA256,
    [string]$ExpectedGarnetPublisherThumbprint = '',
    [switch]$SyntheticUnsignedCIFixture
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
if ($ExpectedPostgresPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
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
if ([IO.Path]::GetExtension($postgres).ToLowerInvariant() -ne '.exe') {
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
    Assert-ApprovedSignature $postgres $ExpectedPostgresPublisherThumbprint
    if ($garnetExt -eq '.exe') {
        Assert-ApprovedSignature $garnet $ExpectedGarnetPublisherThumbprint
    }
}
if ($garnetExt -eq '.zip') {
    # Inspect file paths without ever extracting or executing vendor contents.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($garnet)
    try {
        if ($archive.Entries.Count -eq 0 -or $archive.Entries.Count -gt 2000) {
            throw 'The ZIP has an invalid or excessive number of entries.'
        }
        foreach ($entry in $archive.Entries) {
            $path = $entry.FullName.Replace('\','/')
            $segments = @($path.Split('/'))
            if ([string]::IsNullOrWhiteSpace($path) -or $path.StartsWith('/') -or
                $path.Contains(':') -or @($segments | Where-Object { $_ -eq '..' }).Count -gt 0 -or
                $entry.Length -gt (512MB)) {
                throw 'The offline ZIP contains an unsafe path or excessive entry size.'
            }
        }
    } finally { $archive.Dispose() }
}
Write-Output 'PASS: offline prerequisite package hashes, file paths and executable publisher requirements were checked without executing software.'
if ($garnetExt -eq '.zip') {
    Write-Output 'LIMITATION: a ZIP hash does not establish publisher authenticity; independent Microsoft release review is still required before installation.'
}
if ($SyntheticUnsignedCIFixture) {
    Write-Output 'SYNTHETIC CI ONLY: executable Authenticode checks bypassed; NEVER a production approval.'
}
Write-Output 'NOT INSTALLED: PostgreSQL, Garnet, TLS, Windows services, database schema or SEFAZ.'
