#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Upgrade simulation requires a disposable GitHub-hosted Windows CI runner.'
}
$guid = [Guid]::NewGuid().ToString('N')
$root = Join-Path $env:RUNNER_TEMP "esterium-upgrade-fixture-$guid"
$installed = Join-Path $env:ProgramFiles "EsteriumUpgradeFixture-$guid"
$candidate = Join-Path $root 'candidate'
$backupDir = Join-Path $root 'backup-local'
$external = Join-Path $root 'backup-other-host-synthetic'
$plan = Join-Path $PSScriptRoot 'plan-esterium-windows-upgrade.ps1'
$manifestWriter = Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1'
$global:ESTERIUM_UPGRADE_CI_SERVICE_STATE = 'Stopped'
$global:ESTERIUM_UPGRADE_CI_TASK_STATE = 'Disabled'
$global:ESTERIUM_UPGRADE_CI_EXE = Join-Path $installed 'esterium-server.exe'
function Get-CimInstance {
    [CmdletBinding()]
    param([string]$ClassName, [string]$Filter)
    if ($ClassName -ne 'Win32_Service' -or $Filter -ne "Name='EsteriumERP'") {
        throw 'Unexpected service inspection.'
    }
    return [PSCustomObject]@{
        PathName=('"'+$global:ESTERIUM_UPGRADE_CI_EXE+'"')
        State=$global:ESTERIUM_UPGRADE_CI_SERVICE_STATE
    }
}
function Get-ScheduledTask {
    [CmdletBinding()]
    param([string]$TaskName)
    if ($TaskName -ne 'EsteriumERP-Backup') { throw 'Unexpected scheduled task inspection.' }
    return [PSCustomObject]@{State=$global:ESTERIUM_UPGRADE_CI_TASK_STATE}
}
function Write-FixtureBundle([string]$path, [int]$migrationCount, [string]$serverText) {
    [IO.Directory]::CreateDirectory((Join-Path $path 'web')) | Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $path 'migrations')) | Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $path 'tools')) | Out-Null
    [IO.File]::WriteAllText((Join-Path $path 'esterium-server.exe'), $serverText)
    [IO.File]::WriteAllText((Join-Path $path 'web\index.html'), 'synthetic UI, not executable')
    [IO.File]::WriteAllText((Join-Path $path 'README_LOCAL.md'), 'Synthetic CI-only bundle.')
    foreach ($tool in @(
        'register-esterium-windows-service.ps1',
        'provision-esterium-app-secrets.ps1',
        'verify-esterium-bundle.ps1',
        'postgres-daily-backup.ps1',
        'copy-verified-external-backup.ps1',
        'check-esterium-backup-integrity.ps1',
        'run-esterium-scheduled-backup.ps1',
        'register-esterium-backup-task.ps1',
        'check-esterium-windows-host.ps1',
        'unregister-esterium-windows-service.ps1',
        'check-esterium-local-dependencies.ps1',
        'plan-esterium-windows-upgrade.ps1'
    )) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $tool) -Destination (Join-Path $path 'tools') -ErrorAction Stop
    }
    [IO.File]::WriteAllText((Join-Path $path 'tools\esterium-migrate.exe'), 'synthetic migrate stub never executed')
    for ($i = 1; $i -le $migrationCount; $i++) {
        $prefix = '{0:d4}_ci_fixture' -f $i
        foreach ($direction in @('up', 'down')) {
            [IO.File]::WriteAllText((Join-Path $path "migrations\$prefix.$direction.sql"), "-- synthetic migration $i $direction")
        }
    }
    & $manifestWriter -BundleDir $path | Out-Null
}
try {
    Write-FixtureBundle $installed 1 'old-synthetic-server'
    Write-FixtureBundle $candidate 2 'new-synthetic-server'
    [IO.Directory]::CreateDirectory($backupDir) | Out-Null
    [IO.Directory]::CreateDirectory($external) | Out-Null
    $name = 'esterium-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-1234abcd.dump'
    $archive = Join-Path $backupDir $name
    [IO.File]::WriteAllBytes($archive, [byte[]](0..127))
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    foreach ($folder in @($backupDir, $external)) {
        [IO.File]::WriteAllBytes((Join-Path $folder $name), [byte[]](0..127))
        [IO.File]::WriteAllText((Join-Path $folder "$name.sha256"), "$hash  $name" + [Environment]::NewLine)
    }
    $statusPath = Join-Path $backupDir 'backup-status.json'
    $status = @{
        state='ok'; archive_verified=$true; external_copy_verified=$true
        last_run_utc=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
        archive_name=$name; archive_sha256=$hash
    }
    function Write-Status { [IO.File]::WriteAllText($statusPath, ($status | ConvertTo-Json -Compress)) }
    Write-Status
    $planParameters = @{
        InstalledDir=$installed; CandidateBundleDir=$candidate; BackupDir=$backupDir
        ExternalBackupDir=$external; TechnicalUnsignedPlan=$true; SyntheticCIFixture=$true
    }
    $result = @(& $plan @planParameters 3>$null) -join ' '
    if ($result -notmatch 'PASS: read-only technical upgrade gate') {
        throw 'Valid upgrade plan was rejected.'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $installed 'esterium-server.exe'))) {
        throw 'Upgrade plan modified the installed binary.'
    }
    function Must-Reject([string]$label, [string]$reason) {
        $rejected = $false
        try { & $plan @planParameters 3>$null | Out-Null }
        catch { $rejected = $_.Exception.Message -match $reason }
        if (-not $rejected) { throw "Upgrade plan accepted $label." }
    }
    $global:ESTERIUM_UPGRADE_CI_SERVICE_STATE = 'Running'
    Must-Reject 'a running POS service' 'Stop and reconcile'
    $global:ESTERIUM_UPGRADE_CI_SERVICE_STATE = 'Stopped'
    $global:ESTERIUM_UPGRADE_CI_TASK_STATE = 'Ready'
    Must-Reject 'an enabled backup task' 'Disable and review'
    $global:ESTERIUM_UPGRADE_CI_TASK_STATE = 'Disabled'
    $status.last_run_utc = [DateTime]::UtcNow.AddDays(-3).ToString('yyyy-MM-ddTHH:mm:ssZ')
    Write-Status
    Must-Reject 'a stale off-machine backup' 'too old'
    $status.last_run_utc = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    Write-Status
    $planParameters.Remove('TechnicalUnsignedPlan')
    Must-Reject 'an unsigned candidate in production mode' 'Unsigned candidate'
    $planParameters.TechnicalUnsignedPlan = $true
    $sqlPath = Join-Path $candidate 'migrations\0001_ci_fixture.up.sql'
    [IO.File]::WriteAllText($sqlPath, '-- modified old migration: forbidden')
    Remove-Item -LiteralPath (Join-Path $candidate 'bundle-manifest.json') -Force
    & $manifestWriter -BundleDir $candidate | Out-Null
    Must-Reject 'changed historical SQL migrations despite valid new manifest' 'altered an existing'
    Write-Output 'PASS: synthetic upgrade plan rejects active service, enabled backup, stale copy, unsigned release and historical SQL changes.'
} finally {
    if (Test-Path -LiteralPath $installed) { Remove-Item -LiteralPath $installed -Recurse -Force }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    foreach ($name in @('ESTERIUM_UPGRADE_CI_SERVICE_STATE','ESTERIUM_UPGRADE_CI_TASK_STATE','ESTERIUM_UPGRADE_CI_EXE')) {
        Remove-Variable -Name $name -Scope Global -ErrorAction SilentlyContinue
    }
}
