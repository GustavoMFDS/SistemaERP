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
$rollbackFixture = ''
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
        'plan-esterium-windows-upgrade.ps1',
        'plan-esterium-local-setup.ps1',
        'install-esterium-local.ps1',
        'apply-esterium-app-update.ps1',
        'check-esterium-signed-release.ps1'
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
    # A frontend-only update must be allowed; the app binary and all migration
    # versions may be identical while compiled web assets change.
    Remove-Item -LiteralPath $candidate -Recurse -Force
    Write-FixtureBundle $candidate 1 'old-synthetic-server'
    [IO.File]::AppendAllText((Join-Path $candidate 'web\index.html'), '-new-ui')
    Remove-Item -LiteralPath (Join-Path $candidate 'bundle-manifest.json') -Force
    & $manifestWriter -BundleDir $candidate | Out-Null
    $uiOnly = @(& $plan @planParameters 3>$null) -join ' '
    if ($uiOnly -notmatch 'PASS: read-only technical upgrade gate') {
        throw 'Upgrade plan rejected a valid UI-only release.'
    }
    # Identical verified packages are a no-op and must not be presented as an upgrade.
    [IO.File]::WriteAllText((Join-Path $candidate 'web\index.html'), 'synthetic UI, not executable')
    Remove-Item -LiteralPath (Join-Path $candidate 'bundle-manifest.json') -Force
    & $manifestWriter -BundleDir $candidate | Out-Null
    Must-Reject 'two identical packages' 'no software update identified'
    # Application-only update uses disposable Program Files fixture and no DB.
    $appUpdater = Join-Path $PSScriptRoot 'apply-esterium-app-update.ps1'
    $env:ESTERIUM_SCM_DISPOSABLE_CI = '1'
    $appArgs = @{
        InstalledDir=$installed; CandidateBundleDir=$candidate
        BackupDir=$backupDir; ExternalBackupDir=$external
        UnsignedDisposableCIFixture=$true
    }
    try {
        Remove-Item -LiteralPath $candidate -Recurse -Force
        Write-FixtureBundle $candidate 2 'synthetic-new-server'
        $sqlUpdateBlocked = $false
        try { & $appUpdater @appArgs 3>$null | Out-Null }
        catch { $sqlUpdateBlocked = $_.Exception.Message -match 'refuses added or removed database migrations' }
        if (-not $sqlUpdateBlocked) {
            throw 'Application-only update accepted migration additions without a database recovery plan.'
        }
        Remove-Item -LiteralPath $candidate -Recurse -Force
        Write-FixtureBundle $candidate 1 'synthetic-new-server'
        $dryAppPlan = @(& $appUpdater @appArgs 3>$null) -join ' '
        if ($dryAppPlan -notmatch 'no changes made') {
            throw 'Application-only upgrade default mode is not a read-only plan.'
        }
        $changed = @(& $appUpdater @appArgs -Apply 3>$null) -join [Environment]::NewLine
        if ($changed -notmatch 'PASS: application-only files updated') {
            throw 'Synthetic application-only replacement did not complete.'
        }
        if ((Get-Content -LiteralPath (Join-Path $installed 'esterium-server.exe') -Raw) -ne 'synthetic-new-server') {
            throw 'Application-only updater did not stage the new checked package.'
        }
        $rollbackFixture = (($changed -split [Environment]::NewLine) |
            Where-Object { $_ -like 'ROLLBACK FOLDER: *' } |
            Select-Object -First 1).Substring('ROLLBACK FOLDER: '.Length)
        if (-not $rollbackFixture.StartsWith((Split-Path -Parent $installed) + '\.esterium-update-rollback-',
                [StringComparison]::OrdinalIgnoreCase) -or
            -not (Test-Path -LiteralPath (Join-Path $rollbackFixture 'esterium-server.exe') -PathType Leaf)) {
            throw 'Application update failed to retain the prior package for rollback.'
        }
        if ((Get-Content -LiteralPath (Join-Path $rollbackFixture 'esterium-server.exe') -Raw) -ne 'old-synthetic-server') {
            throw 'Application rollback copy no longer matches the old program.'
        }
        Write-Output 'PASS: synthetic application-only update preserved the old bundle and never changed SQL or database.'
    } finally {
        Remove-Item Env:ESTERIUM_SCM_DISPOSABLE_CI -ErrorAction SilentlyContinue
    }
    Write-Output 'PASS: synthetic upgrade plan rejects active service, enabled backup, stale copy, unsigned release, rewritten SQL and no-op; UI-only upgrades allowed.'
} finally {
    if (Test-Path -LiteralPath $installed) { Remove-Item -LiteralPath $installed -Recurse -Force }
    if ($rollbackFixture -and (Test-Path -LiteralPath $rollbackFixture)) {
        Remove-Item -LiteralPath $rollbackFixture -Recurse -Force
    }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    foreach ($name in @('ESTERIUM_UPGRADE_CI_SERVICE_STATE','ESTERIUM_UPGRADE_CI_TASK_STATE','ESTERIUM_UPGRADE_CI_EXE')) {
        Remove-Variable -Name $name -Scope Global -ErrorAction SilentlyContinue
    }
}
