#requires -Version 5.1
<#
  Hosted Windows CI runner only. Creates a real Windows SCM service with a
  synthetic (non-executable) bundle, validates registration, then deletes
  only the service and files created in this isolated CI run.
  NEVER start the service or use real DB credentials.
#>
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Real SCM smoke is restricted to a GitHub-hosted disposable Windows runner.'
}
$sc = Join-Path $env:SystemRoot 'System32\sc.exe'
$existing = & $sc query EsteriumERP 2>$null
$previous = $LASTEXITCODE
$global:LASTEXITCODE = 0
if ($previous -ne 1060) {
    throw "SCM smoke requires no pre-existing EsteriumERP service (query code=$previous)."
}
$guid = [Guid]::NewGuid().ToString('N')
$root = Join-Path $env:RUNNER_TEMP "esterium-scm-$guid"
$bundle = Join-Path $root 'bundle'
$web = Join-Path $bundle 'web'
$secrets = Join-Path $root 'secrets'
$install = Join-Path $env:ProgramFiles "EsteriumERP-CI-$guid"
$created = $false
$installCalled = $false
$taskInstallAttempted = $false

try {
    New-Item -ItemType Directory -Path $web, $secrets -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $bundle 'esterium-server.exe'), 'synthetic-CI-binary-not-runnable')
    [IO.File]::WriteAllText((Join-Path $web 'index.html'), '<html>synthetic</html>')
    $migrationDir = Join-Path $bundle 'migrations'
    [IO.Directory]::CreateDirectory($migrationDir) | Out-Null
    [IO.File]::WriteAllText((Join-Path $migrationDir '0001_ci_fixture.up.sql'), '-- Synthetic migration for bundle integrity only')
    [IO.File]::WriteAllText((Join-Path $migrationDir '0001_ci_fixture.down.sql'), '-- Synthetic rollback file')

    $tools = Join-Path $bundle 'tools'
    New-Item -ItemType Directory -Path $tools | Out-Null
    [IO.File]::WriteAllText((Join-Path $tools 'esterium-migrate.exe'), 'synthetic-CLI-never-executed')
    foreach ($name in @(
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
    'check-esterium-signed-release.ps1',
    'check-esterium-offline-dependency-packages.ps1',
    'provision-esterium-garnet-config.ps1',
    'provision-esterium-postgres-optionfile.ps1',
    'prepare-esterium-local-dependency-configs.ps1',
        'install-esterium-local-dependencies.ps1',
        'provision-esterium-dependency-bootstrap.ps1',
    'setup-esterium-local-dependencies.ps1',
    'check-esterium-postgres-tls-policy.ps1',
        'provision-esterium-postgres-tls-policy.ps1'
    )) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination $tools
    }
    [IO.File]::WriteAllText((Join-Path $bundle 'README_LOCAL.md'), 'CI synthetic test instructions')
    & (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $bundle | Out-Null
    & (Join-Path $tools 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null

    $secretFiles = @{}
    foreach ($name in @('database', 'redis', 'jwt', 'metrics')) {
        $file = Join-Path $secrets "$name.secret"
        [IO.File]::WriteAllText($file, "synthetic-placeholder-$guid")
        # Explicitly restrict the file to LocalService, SYSTEM and admin;
        # generic Users/Authenticated Users/Everyone may not read secrets.
        $icacls = & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-19:R' '*S-1-5-18:F' '*S-1-5-32-544:F' 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Secret ACL preparation failed: $icacls" }
        $secretFiles[$name] = $file
    }
    $arguments = @{
        BundleDir = $bundle
        DatabaseURLFile = $secretFiles.database
        RedisURLFile = $secretFiles.redis
        JWTSecretFile = $secretFiles.jwt
        MetricsBearerTokenFile = $secretFiles.metrics
        AllowedOrigin = 'https://esterium.ci.invalid'
        InstallDir = $install
        UnsignedDisposableCIFixture = $true
    }
    # The same direct registrar must fail closed in production mode
    # without independently approved publisher/hash, even with valid paths.
    $realArguments = $arguments.Clone()
    $realArguments.Remove('UnsignedDisposableCIFixture')
    $unapprovedRejected = $false
    try { & (Join-Path $tools 'register-esterium-windows-service.ps1') @realArguments | Out-Null }
    catch { $unapprovedRejected = $_.Exception.Message -match 'requires independently approved release digest' }
    if (-not $unapprovedRejected) {
        throw 'The direct SCM registrar accepted an unsigned release without approval.'
    }
    # Fail closed before creating a service when LocalService could edit a
    # password file or another account has direct read access.
    $registrar = Join-Path $tools 'register-esterium-windows-service.ps1'
    $weakSecret = $secretFiles.jwt
    $badAcl = & icacls.exe $weakSecret '/grant:r' '*S-1-5-19:M' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not create synthetic writable secret: $badAcl" }
    $mutableRejected = $false
    try { & $registrar @arguments | Out-Null }
    catch { $mutableRejected = $_.Exception.Message -match 'must not be able to change credential' }
    if (-not $mutableRejected) { throw 'Installer accepted a writable LocalService credential.' }
    $fixedAcl = & icacls.exe $weakSecret '/grant:r' '*S-1-5-19:R' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not restore synthetic read-only secret: $fixedAcl" }
    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $grantUser = & icacls.exe $weakSecret '/grant' ("*" + $currentSid + ':R') 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not add synthetic extra credential reader: $grantUser" }
    $otherReaderRejected = $false
    try { & $registrar @arguments | Out-Null }
    catch { $otherReaderRejected = $_.Exception.Message -match 'non-approved Windows identity' }
    if (-not $otherReaderRejected) { throw 'Installer accepted an unrelated Windows account reading credentials.' }
    $removeReader = & icacls.exe $weakSecret '/remove' ("*" + $currentSid) 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not remove synthetic extra credential reader: $removeReader" }
    Write-Output 'PASS: SCM refuses mutable secrets and direct read access by unrelated users before registration.'
    # Late installed-file tampering must be detected BEFORE even creating
    # the disposable SCM service, despite a verified staging manifest.
    $tamperInstall = Join-Path $env:ProgramFiles "EsteriumERP-CI-tampered-$guid"
    $tamperArgs = $arguments.Clone()
    $tamperArgs.InstallDir = $tamperInstall
    $tamperDenied = $false
    $env:ESTERIUM_TEST_TAMPER_INSTALLED_BUNDLE = '1'
    try {
        try { & $registrar @tamperArgs | Out-Null }
        catch { $tamperDenied = $_.Exception.Message -match 'Bundle file SHA-256 mismatch' }
    } finally {
        Remove-Item Env:ESTERIUM_TEST_TAMPER_INSTALLED_BUNDLE -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath $tamperInstall) {
            Remove-Item -LiteralPath $tamperInstall -Recurse -Force
        }
    }
    if (-not $tamperDenied) { throw 'SCM registrar accepted tampering after moving the staged installation.' }
    $scQueryAfterTamper = & $sc query EsteriumERP 2>$null
    $noServiceAfterTamper = ($LASTEXITCODE -eq 1060)
    $global:LASTEXITCODE = 0
    if (-not $noServiceAfterTamper) {
        throw 'SCM service was created before the final installation passed integrity verification.'
    }
    Write-Output 'PASS: corrupted final install refused before the Windows service was created.'
    # Exercise an actual SCM create followed by an injected failure.
    # The service must be removed automatically; package/data stay untouched.
    $faultInstall = Join-Path $env:ProgramFiles "EsteriumERP-CI-failed-$guid"
    $faultArguments = $arguments.Clone()
    $faultArguments.InstallDir = $faultInstall
    $installCalled = $true
    $faultRejected = $false
    $env:ESTERIUM_TEST_FAIL_AFTER_SC_CREATE = '1'
    try {
        try {
            & (Join-Path $tools 'register-esterium-windows-service.ps1') @faultArguments | Out-Null
        } catch {
            $faultRejected = $_.Exception.Message -match 'CI synthetic fault after Windows service creation'
        }
    } finally {
        Remove-Item Env:ESTERIUM_TEST_FAIL_AFTER_SC_CREATE -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath $faultInstall) {
            Remove-Item -LiteralPath $faultInstall -Force -Recurse
        }
    }
    if (-not $faultRejected) { throw 'The injected SCM configuration failure was not raised.' }
    $serviceGone = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $check = & $sc query EsteriumERP 2>$null
        $checkCode = $LASTEXITCODE
        $global:LASTEXITCODE = 0
        if ($checkCode -eq 1060) { $serviceGone = $true; break }
        Start-Sleep -Milliseconds 250
    }
    if (-not $serviceGone) {
        throw 'Failed SCM install left a partial Windows service behind.'
    }
    Write-Output 'PASS: injected post-create failure rolled back only the synthetic SCM entry.'
    & (Join-Path $tools 'register-esterium-windows-service.ps1') @arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'SCM installer returned nonzero.' }
    $query = & $sc qc EsteriumERP 2>&1
    if ($LASTEXITCODE -ne 0 -or ($query -join ' ') -notmatch 'DEMAND_START') {
        throw "Windows service was not installed with manual startup: $query"
    }
    if (($query -join ' ') -notmatch 'LocalService') {
        throw 'SCM service account was not restricted to LocalService'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $install 'web\index.html'))) {
        throw 'SCM registration did not stage the expected files'
    }
    foreach ($relative in @(
        'tools\register-esterium-windows-service.ps1',
        'tools\provision-esterium-app-secrets.ps1',
        'tools\postgres-daily-backup.ps1',
        'tools\copy-verified-external-backup.ps1',
        'tools\check-esterium-backup-integrity.ps1',
        'tools\run-esterium-scheduled-backup.ps1',
        'tools\register-esterium-backup-task.ps1',
        'tools\check-esterium-windows-host.ps1',
        'tools\unregister-esterium-windows-service.ps1',
        'tools\check-esterium-local-dependencies.ps1',
        'tools\plan-esterium-windows-upgrade.ps1',
        'tools\plan-esterium-local-setup.ps1',
        'tools\install-esterium-local.ps1',
        'tools\apply-esterium-app-update.ps1',
        'tools\check-esterium-signed-release.ps1',
        'tools\check-esterium-offline-dependency-packages.ps1',
        'tools\provision-esterium-garnet-config.ps1',
        'tools\provision-esterium-postgres-optionfile.ps1',
        'tools\prepare-esterium-local-dependency-configs.ps1',
        'tools\check-esterium-postgres-tls-policy.ps1',
        'tools\\esterium-migrate.exe',
        'README_LOCAL.md',
        'bundle-manifest.json',
        'migrations\0001_ci_fixture.up.sql',
        'migrations\0001_ci_fixture.down.sql'
    )) {
        if (-not (Test-Path -LiteralPath (Join-Path $install $relative) -PathType Leaf)) {
            throw "Installed maintenance file is missing: $relative"
        }
    }
    & (Join-Path $install 'tools\verify-esterium-bundle.ps1') -BundleDir $install | Out-Null
    $regPath = 'HKLM:\SYSTEM\CurrentControlSet\Services\EsteriumERP'
    $values = @((Get-ItemProperty -Path $regPath -Name Environment).Environment)
    if (@($values | Where-Object { $_ -eq 'FISCAL_PROVIDER=disabled' }).Count -ne 1 -or
        @($values | Where-Object { $_ -eq 'HTTP_ADDR=127.0.0.1:8080' }).Count -ne 1 -or
        @($values | Where-Object { $_ -eq 'APP_ENV=prod' }).Count -ne 1 -or
        @($values | Where-Object { $_ -like 'DATABASE_URL_FILE=*' }).Count -ne 1 -or
        @($values | Where-Object { $_ -like 'ESTERIUM_MIGRATIONS_DIR=*' }).Count -ne 1) {
        throw 'SCM registry environment is missing restrictive production parameters.'
    }
    if (($values -join ';') -match 'synthetic-placeholder-') {
        throw 'SCM registry leaked a secret value.'
    }
    # Real Windows Task Scheduler smoke: no database, backup or network call.
    # Only create a disabled task on a GitHub-hosted disposable runner.
    $taskName = 'EsteriumERP-Backup'
    if (@(Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue).Count -ne 0) {
        throw 'Synthetic Task Scheduler smoke requires an absent backup task.'
    }
    $pgTools = Join-Path $root 'pgtools'
    [IO.Directory]::CreateDirectory($pgTools) | Out-Null
    foreach ($toolExe in @('pg_dump.exe', 'pg_restore.exe', 'psql.exe')) {
        [IO.File]::WriteAllText((Join-Path $pgTools $toolExe), 'synthetic-not-executable')
    }
    $backupPasswordFile = Join-Path $secrets 'postgres-backup.pgpass'
    $backupCA = Join-Path $secrets 'postgres-ca.crt'
    $configFile = Join-Path $secrets 'backup-config.json'
    [IO.File]::WriteAllText($backupPasswordFile, 'synthetic-only-credential')
    [IO.File]::WriteAllText($backupCA, 'synthetic-only-certificate')
    $backupConfig = @{
        PGHOST='127.0.0.1'; PGPORT='5432'; PGUSER='esterium_backup'
        PGDATABASE='esterium'; PGPASSFILE=$backupPasswordFile
        PGSSLROOTCERT=$backupCA; PostgresBin=$pgTools
        BackupDir=(Join-Path $root 'archives')
        ExternalBackupDir='\\other-ci-host\backups'; RetentionDays=14
    }
    [IO.File]::WriteAllText($configFile, ($backupConfig | ConvertTo-Json -Compress))
    foreach ($file in @($backupPasswordFile, $configFile, $backupCA)) {
        $aclResult = & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-20:R' '*S-1-5-18:F' '*S-1-5-32-544:F' 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Synthetic backup task ACL failed: $aclResult" }
    }
    $taskInstallAttempted = $true
    & (Join-Path $install 'tools\register-esterium-backup-task.ps1') -ConfigFile $configFile -InstallDir $install -DailyAt '02:31' -Apply -UnsignedDisposableCIFixture | Out-Null
    $installedTask = Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
    if ($installedTask.State -ne 'Disabled' -or
        $installedTask.Principal.UserId -notmatch '(?i)NETWORK SERVICE|S-1-5-20') {
        throw 'Backup task is not disabled or lacks restricted NetworkService identity.'
    }
    if ($installedTask.Actions[0].Arguments -notlike "*$configFile*" -or
        $installedTask.Actions[0].Arguments -match 'synthetic-only-credential|PGPASSWORD|PGPASSFILE=') {
        throw 'Backup scheduled task does not pass only a config file path.'
    }
    Write-Output 'PASS: Windows Scheduler registered real DISABLED NetworkService task with no credentials.'
    $unregister = Join-Path $install 'tools\unregister-esterium-windows-service.ps1'
    $dryRun = @(& $unregister -InstallDir $install) -join ' '
    if ($dryRun -notmatch 'no changes made') {
        throw 'Service-only removal plan did not remain read-only.'
    }
    $stillHere = & $sc query EsteriumERP 2>&1
    if ($LASTEXITCODE -ne 0) { throw 'Service was removed during the read-only unregistration plan.' }
    $global:LASTEXITCODE = 0
    $unsafeUnregister = $false
    try { & $unregister -InstallDir (Join-Path $env:ProgramFiles 'EsteriumERP-Other') -Apply | Out-Null }
    catch { $unsafeUnregister = $_.Exception.Message -match 'Service executable differs' }
    if (-not $unsafeUnregister) { throw 'Unregister allowed deleting a service owned by another path.' }
    # Production service removal must refuse an unsigned/unapproved admin
    # script, while the disposable SCM smoke can still check actual deletion.
    $deniedRelease = $false
    try { & $unregister -InstallDir $install -Apply | Out-Null }
    catch { $deniedRelease = $_.Exception.Message -match 'requires independently approved release digest' }
    if (-not $deniedRelease) {
        throw 'Real Windows service removal accepted missing release approval.'
    }
    & $unregister -InstallDir $install -Apply -UnsignedDisposableCIFixture | Out-Null
    $gone = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $queryResult = & $sc query EsteriumERP 2>$null
        $queryCode = $LASTEXITCODE
        $global:LASTEXITCODE = 0
        if ($queryCode -eq 1060) { $gone = $true; break }
        Start-Sleep -Milliseconds 250
    }
    if (-not $gone) { throw 'Service still registered after explicit service-only removal.' }
    foreach ($dataFile in @('esterium-server.exe','web\index.html','bundle-manifest.json')) {
        if (-not (Test-Path -LiteralPath (Join-Path $install $dataFile) -PathType Leaf)) {
            throw 'Service removal unexpectedly deleted installation files.'
        }
    }
    $backupTaskAfter = Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction Stop
    if ($backupTaskAfter.State -ne 'Disabled') { throw 'Service removal unexpectedly changed the backup task.' }
    Write-Output 'PASS: explicit service deregistration retained binaries and disabled backup task.'
    $created = $true
    Write-Output 'PASS: real SCM service was created with LocalService, manual startup and only credential paths.'
}
finally {
    if ($taskInstallAttempted) {
        # Only the task proven absent before this run can be deleted.
        $existingBackupTask = Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction SilentlyContinue
        if ($null -ne $existingBackupTask) {
            Unregister-ScheduledTask -TaskName 'EsteriumERP-Backup' -Confirm:$false -ErrorAction Stop
        }
    }
    # Only a fresh, explicitly absent service may be touched. It is never started.
    if ($installCalled) {
        $query = & $sc query EsteriumERP 2>$null
        $code = $LASTEXITCODE
        $global:LASTEXITCODE = 0
        if ($code -eq 0) {
            $deleted = & $sc delete EsteriumERP 2>&1
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "Manual CI cleanup needed for test-created service: $deleted"
            }
            $global:LASTEXITCODE = 0
        }
    }
    if (Test-Path -LiteralPath $install) {
        Remove-Item -LiteralPath $install -Force -Recurse
    }
    if (Test-Path -LiteralPath $root) {
        Remove-Item -LiteralPath $root -Force -Recurse
    }
}
