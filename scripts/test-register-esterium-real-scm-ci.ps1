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
    'register-esterium-backup-task.ps1'
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
    }
    $installCalled = $true
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
    $created = $true
    Write-Output 'PASS: real SCM service was created with LocalService, manual startup and only credential paths.'
}
finally {
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
