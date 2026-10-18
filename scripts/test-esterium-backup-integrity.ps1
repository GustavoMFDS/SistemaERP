#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Synthetic backup checker test requires a hosted Windows CI runner.'
}
$base = Join-Path $env:RUNNER_TEMP ('esterium-backup-health-' + [Guid]::NewGuid().ToString('N'))
$local = Join-Path $base 'local'
$external = Join-Path $base 'external'
$name = 'esterium-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-1234abcd.dump'
$checker = Join-Path $PSScriptRoot 'check-esterium-backup-integrity.ps1'
try {
    New-Item -ItemType Directory -Path $local, $external -Force | Out-Null
    $bytes = [byte[]](0..255)
    foreach ($dir in @($local, $external)) {
        [IO.File]::WriteAllBytes((Join-Path $dir $name), $bytes)
    }
    $sha = (Get-FileHash (Join-Path $local $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    foreach ($dir in @($local, $external)) {
        [IO.File]::WriteAllText((Join-Path $dir "$name.sha256"), "$sha  $name" + [Environment]::NewLine)
    }
    $statusFile = Join-Path $local 'backup-status.json'
    $status = @{
        state = 'ok'
        last_run_utc = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
        archive_verified = $true
        external_copy_verified = $true
        archive_name = $name
        archive_sha256 = $sha
    }
    function Write-SyntheticStatus {
        [IO.File]::WriteAllText($statusFile, ($status | ConvertTo-Json -Compress))
    }
    Write-SyntheticStatus
    $args = @{ BackupDir = $local; ExternalBackupDir = $external; SyntheticCIFixture = $true }
    $ok = @(& $checker @args) -join ' '
    if ($ok -notmatch 'PASS: recent local') {
        throw 'Health gate rejected a fully verified local/external fixture.'
    }
    function Require-Rejection([string]$reason) {
        $rejected = $false
        try { & $checker @args | Out-Null }
        catch { $rejected = $true }
        if (-not $rejected) { throw "Health gate accepted $reason" }
    }
    # The stronger gate must prove two distinct backed-up generations using
    # only disposable archive bytes and independently checked SHA-256 files.
    $yesterday = 'esterium-' + [DateTime]::UtcNow.AddDays(-1).ToString('yyyyMMddTHHmmssZ') + '-87654321.dump'
    foreach ($dir in @($local, $external)) {
        [IO.File]::WriteAllBytes((Join-Path $dir $yesterday), $bytes)
        [IO.File]::WriteAllText((Join-Path $dir "$yesterday.sha256"),
            "$sha  $yesterday" + [Environment]::NewLine)
    }
    $args.MinimumVerifiedCopies = 2
    $two = @(& $checker @args) -join ' '
    if ($two -notmatch 'PASS: recent local') { throw 'Backup gate rejected two complete synthetic generations.' }
    Remove-Item -LiteralPath (Join-Path $external "$yesterday.sha256") -Force
    Require-Rejection 'two generations when the older remote checksum is missing'
    [IO.File]::WriteAllText((Join-Path $external "$yesterday.sha256"),
        "$sha  $yesterday" + [Environment]::NewLine)
    [IO.File]::AppendAllText((Join-Path $external $yesterday), 'altered')
    Require-Rejection 'two generations when the older remote bytes disagree'
    [IO.File]::WriteAllBytes((Join-Path $external $yesterday), $bytes)
    $args.Remove('MinimumVerifiedCopies')

    [IO.File]::AppendAllText((Join-Path $external $name), 'altered')
    Require-Rejection 'a corrupted external archive'
    [IO.File]::WriteAllBytes((Join-Path $external $name), $bytes)
    $status.last_run_utc = [DateTime]::UtcNow.AddHours(-72).ToString('yyyy-MM-ddTHH:mm:ssZ')
    Write-SyntheticStatus
    Require-Rejection 'a stale backup'
    $status.last_run_utc = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $oldName = 'esterium-20200101T000000Z-1234abcd.dump'
    foreach ($dir in @($local, $external)) {
        [IO.File]::WriteAllBytes((Join-Path $dir $oldName), $bytes)
        [IO.File]::WriteAllText((Join-Path $dir "$oldName.sha256"), "$sha  $oldName" + [Environment]::NewLine)
    }
    $status.archive_name = $oldName
    Write-SyntheticStatus
    $staleNameRejected = $false
    try { & $checker @args | Out-Null }
    catch { $staleNameRejected = $_.Exception.Message -match 'archive itself is stale' }
    if (-not $staleNameRejected) { throw 'Accepted old backup bytes with a fresh status timestamp.' }
    $status.archive_name = $name
    $status.external_copy_verified = $false
    Write-SyntheticStatus
    Require-Rejection 'a failed external-copy status'
    $status.external_copy_verified = $true
    $status.archive_name = '..\secret.dump'
    Write-SyntheticStatus
    Require-Rejection 'an unsafe archive name from JSON'
    $status.archive_name = $name
    Write-SyntheticStatus
    Remove-Item -LiteralPath (Join-Path $external "$name.sha256") -Force
    Require-Rejection 'a missing external checksum file'
    $args.ExternalBackupDir = $local
    Require-Rejection 'a backup allegedly on the same disk and path'
    Write-Output 'PASS: freshness, two verified archives, tampering, missing checksum, failed status and traversal guards.'
}
finally {
    if (Test-Path -LiteralPath $base) {
        Remove-Item -LiteralPath $base -Recurse -Force
    }
}
