#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = Join-Path ([IO.Path]::GetTempPath()) ('esterium-backup-test-' + [Guid]::NewGuid().ToString('N'))
$srcDir = Join-Path $root 'source'
$destDir = Join-Path $root 'external-synthetic'
try {
    [IO.Directory]::CreateDirectory($srcDir) | Out-Null
    [IO.Directory]::CreateDirectory($destDir) | Out-Null
    $source = Join-Path $srcDir 'esterium-synthetic.dump'
    [IO.File]::WriteAllBytes($source, [byte[]](0..255))
    $originalHash = (Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant()
    $helper = Join-Path $PSScriptRoot 'copy-verified-external-backup.ps1'
    & $helper -SourceDump $source -DestinationDir $destDir | Out-Null
    $target = Join-Path $destDir 'esterium-synthetic.dump'
    $targetHash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($targetHash -ne $originalHash) {
        throw 'Copied archive hash differs from source'
    }
    $checksumPath = "$target.sha256"
    if (-not (Test-Path -LiteralPath $checksumPath)) {
        throw 'External backup checksum sidecar missing'
    }
    $expectedSidecar = "$originalHash  esterium-synthetic.dump"
    if ((Get-Content -LiteralPath $checksumPath -Raw).Trim() -ne $expectedSidecar) {
        throw 'External backup checksum sidecar incorrect'
    }
    # Never permit a second backup to replace an existing verified archive.
    $rejected = $false
    try {
        & $helper -SourceDump $source -DestinationDir $destDir | Out-Null
    } catch {
        $rejected = $_.Exception.Message -match 'already exists'
    }
    if (-not $rejected) { throw 'External backup overwrite was not blocked' }
    if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant() -ne $originalHash) {
        throw 'Existing verified external archive changed after duplicate request'
    }
    $empty = Join-Path $srcDir 'empty.dump'
    [IO.File]::WriteAllBytes($empty, [byte[]]@())
    $rejected = $false
    try {
        & $helper -SourceDump $empty -DestinationDir $destDir | Out-Null
    } catch {
        $rejected = $_.Exception.Message -match 'nonempty'
    }
    if (-not $rejected) { throw 'Empty dump was not rejected' }
    Write-Output 'PASS: external backup helper stages, verifies, refuses overwrite and rejects empty archives'
} finally {
    if (Test-Path -LiteralPath $root) {
        Remove-Item -LiteralPath $root -Recurse -Force
    }
}
