#requires -Version 5.1
<#
  Copies a completed PostgreSQL custom-format backup to an independent
  destination with staging, SHA-256 verification and no overwrite.
  This helper is also testable with synthetic archive bytes only.
#>
param(
    [Parameter(Mandatory = $true)][string]$SourceDump,
    [Parameter(Mandatory = $true)][string]$DestinationDir
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$source = Get-Item -LiteralPath $SourceDump -ErrorAction Stop
if ($source.PSIsContainer -or $source.Length -le 0 -or
    -not $source.Name.EndsWith('.dump', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Source must be a nonempty PostgreSQL .dump file.'
}
$destination = [IO.Path]::GetFullPath($DestinationDir)
[IO.Directory]::CreateDirectory($destination) | Out-Null
$target = Join-Path $destination $source.Name
$checksumTarget = "$target.sha256"
if ((Test-Path -LiteralPath $target -PathType Leaf) -or
    (Test-Path -LiteralPath $checksumTarget -PathType Leaf)) {
    throw 'External backup already exists; refusing to overwrite it.'
}
$hash = (Get-FileHash -LiteralPath $source.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
$nonce = [Guid]::NewGuid().ToString('N')
$staging = Join-Path $destination ".$($source.Name).$nonce.partial"
$stagingChecksum = "$staging.sha256"
try {
    Copy-Item -LiteralPath $source.FullName -Destination $staging -ErrorAction Stop
    $stagedHash = (Get-FileHash -LiteralPath $staging -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($stagedHash -ne $hash) { throw 'External staging SHA256 mismatch.' }
    [IO.File]::WriteAllText($stagingChecksum,
        "$hash  $($source.Name)" + [Environment]::NewLine,
        [Text.UTF8Encoding]::new($false))
    # Each rename is within the destination volume/share.
    if ((Test-Path -LiteralPath $target) -or (Test-Path -LiteralPath $checksumTarget)) {
        throw 'A concurrent backup already created this archive.'
    }
    Move-Item -LiteralPath $staging -Destination $target -ErrorAction Stop
    Move-Item -LiteralPath $stagingChecksum -Destination $checksumTarget -ErrorAction Stop
    $verifiedHash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($verifiedHash -ne $hash) { throw 'External final SHA256 mismatch.' }
    Write-Output 'PASS: external archive copied and SHA256 verified'
} finally {
    if (Test-Path -LiteralPath $staging) {
        Remove-Item -LiteralPath $staging -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $stagingChecksum) {
        Remove-Item -LiteralPath $stagingChecksum -Force -ErrorAction SilentlyContinue
    }
}
