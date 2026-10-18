#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Offline prerequisite validation fixture requires disposable hosted Windows CI.'
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$root = Join-Path $env:RUNNER_TEMP ('esterium-offline-dependency-fixture-' + [Guid]::NewGuid().ToString('N'))
$postgres = Join-Path $root 'postgresql-16-ci.exe'
$garnet = Join-Path $root 'garnet-ci.zip'
$checker = Join-Path $PSScriptRoot 'check-esterium-offline-dependency-packages.ps1'
function Write-Zip([string]$path, [string]$entryPath) {
    if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
    $archive = [IO.Compression.ZipFile]::Open($path, [IO.Compression.ZipArchiveMode]::Create)
    try {
        $entry = $archive.CreateEntry($entryPath)
        $stream = $entry.Open()
        $writer = [IO.StreamWriter]::new($stream)
        try { $writer.Write('synthetic-not-executable') } finally { $writer.Dispose() }
    } finally { $archive.Dispose() }
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($postgres, 'synthetic-unsigned-vendor-exe-never-run')
    Write-Zip $garnet 'garnet/readme.txt'
    $env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI = '1'
    $gateArgs = @{
        PostgresInstaller=$postgres
        ExpectedPostgresSHA256=(Get-FileHash -LiteralPath $postgres -Algorithm SHA256).Hash
        ExpectedPostgresPublisherThumbprint=('A' * 40)
        GarnetPackage=$garnet
        ExpectedGarnetSHA256=(Get-FileHash -LiteralPath $garnet -Algorithm SHA256).Hash
        SyntheticUnsignedCIFixture=$true
    }
    $result = @(& $checker @gateArgs) -join ' '
    if ($result -notmatch 'PASS: offline prerequisite package hashes' -or
        $result -notmatch 'SYNTHETIC CI ONLY') {
        throw 'Synthetic offline dependency audit failed.'
    }
    function Assert-Fails([string]$label, [string]$expected) {
        $msg = '(accepted)'
        try { & $checker @gateArgs | Out-Null } catch { $msg = $_.Exception.Message }
        if ($msg -notmatch $expected) { throw "Audit unexpectedly accepted $label, or failed for another reason: $msg" }
    }
    $gateArgs.ExpectedPostgresSHA256 = '0' * 64
    Assert-Fails 'an unapproved PostgreSQL hash' 'independently approved SHA-256'
    $gateArgs.ExpectedPostgresSHA256 = (Get-FileHash -LiteralPath $postgres -Algorithm SHA256).Hash
    [IO.File]::AppendAllText($postgres, 'tampered')
    Assert-Fails 'tampered offline executable' 'independently approved SHA-256'
    $gateArgs.ExpectedPostgresSHA256 = (Get-FileHash -LiteralPath $postgres -Algorithm SHA256).Hash
    $gateArgs.Remove('SyntheticUnsignedCIFixture')
    Assert-Fails 'unsigned installer without CI-only exception' 'approved Authenticode signer'
    $gateArgs.SyntheticUnsignedCIFixture = $true
    Write-Zip $garnet '../evil.ps1'
    $gateArgs.ExpectedGarnetSHA256 = (Get-FileHash -LiteralPath $garnet -Algorithm SHA256).Hash
    Assert-Fails 'malformed archive paths even when hashed correctly' 'unsafe path'
    Write-Zip $garnet 'garnet/readme.txt'
    $gateArgs.ExpectedGarnetSHA256 = (Get-FileHash -LiteralPath $garnet -Algorithm SHA256).Hash
    $gateArgs.GarnetPackage = $postgres
    Assert-Fails 'same artifact for both prerequisites' 'separate approved artifacts'
    $gateArgs.GarnetPackage = $garnet
    Write-Output 'PASS: synthetic offline prerequisite gate rejects tampering, unsigned binaries, archive traversal and artifact aliasing.'
} finally {
    Remove-Item Env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
