#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = Join-Path ([IO.Path]::GetTempPath()) ('esterium-plan-' + [Guid]::NewGuid().ToString('N'))
$bundle = Join-Path $root 'technical-bundle'
$web = Join-Path $bundle 'web'
$secrets = Join-Path $root 'secrets'
$install = Join-Path $env:ProgramFiles ('EsteriumERP-Plan-' + [Guid]::NewGuid().ToString('N'))
try {
    [IO.Directory]::CreateDirectory($web) | Out-Null
    [IO.Directory]::CreateDirectory($secrets) | Out-Null
    [IO.File]::WriteAllText((Join-Path $bundle 'esterium-server.exe'), 'synthetic-not-executable')
    [IO.Directory]::CreateDirectory((Join-Path $bundle 'tools')) | Out-Null
    [IO.File]::WriteAllText((Join-Path $bundle 'tools\\esterium-migrate.exe'), 'synthetic-CLI-never-executed')
    [IO.File]::WriteAllText((Join-Path $web 'index.html'), 'synthetic-html')
    $migrationDir = Join-Path $bundle 'migrations'
    [IO.Directory]::CreateDirectory($migrationDir) | Out-Null
    [IO.File]::WriteAllText((Join-Path $migrationDir '0001_ci_fixture.up.sql'), '-- Synthetic migration for bundle integrity only')
    [IO.File]::WriteAllText((Join-Path $migrationDir '0001_ci_fixture.down.sql'), '-- Synthetic rollback file')

    $names = @('database', 'redis', 'jwt', 'metrics')
    $paths = @{}
    foreach ($name in $names) {
        $paths[$name] = Join-Path $secrets "$name.secret"
        [IO.File]::WriteAllText($paths[$name], 'synthetic-credential-placeholder')
    }
    $bundleTools = Join-Path $bundle 'tools'
    [IO.Directory]::CreateDirectory($bundleTools) | Out-Null
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
    'unregister-esterium-windows-service.ps1'
    )) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $tool) -Destination (Join-Path $bundleTools $tool)
    }
    [IO.File]::WriteAllText((Join-Path $bundle 'README_LOCAL.md'), 'Synthetic setup directions')
    & (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $bundle | Out-Null
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
    $hostCheck = Join-Path $bundleTools 'check-esterium-windows-host.ps1'
    $hostArgs = @{
        BundleDir = $bundle; InstallDir = $install
        ListenPort = 65399; MinimumFreeGiB = 1
    }
    $hostResult = @(& $hostCheck @hostArgs 3>$null) -join ' '
    if ($hostResult -notmatch 'PASS: Windows x64 host' -or
        (Test-Path -LiteralPath $install)) {
        throw 'Host technical preflight failed or modified the installation.'
    }
    $unsignedRejected = $false
    try { & $hostCheck @hostArgs -RequireSignedServer | Out-Null }
    catch { $unsignedRejected = $_.Exception.Message -match 'signature check failed' }
    if (-not $unsignedRejected) {
        throw 'Host gate accepted an unsigned synthetic server as a signed release.'
    }
    $hostArgs.MinimumFreeGiB = 512
    $spaceRejected = $false
    try { & $hostCheck @hostArgs 3>$null | Out-Null }
    catch { $spaceRejected = $_.Exception.Message -match 'Not enough free disk' }
    if (-not $spaceRejected) { throw 'Host gate accepted an impossible disk threshold.' }
    $hostArgs.MinimumFreeGiB = 1
    $hostArgs.InstallDir = Join-Path $root 'outside-program-files'
    $outsideRejected = $false
    try { & $hostCheck @hostArgs 3>$null | Out-Null }
    catch { $outsideRejected = $_.Exception.Message -match 'dedicated subdirectory of Program Files' }
    if (-not $outsideRejected) { throw 'Host gate accepted an unsafe installation path.' }
    $hostArgs.InstallDir = $install
    Write-Output 'PASS: technical host preflight is read-only and rejects unsigned, low-disk and unsafe installation requests.'
    # The migration executable is mandatory and protected by the manifest.
    $migratorPath = Join-Path $bundle 'tools\\esterium-migrate.exe'
    $outside = Join-Path $root 'temporary-migrator-test-only'
    Move-Item -LiteralPath $migratorPath -Destination $outside
    try {
        $missingMigratorRejected = $false
        try { & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null }
        catch { $missingMigratorRejected = $_.Exception.Message -match 'Missing bundle file' }
        if (-not $missingMigratorRejected) { throw 'Bundle accepted absent PostgreSQL migrator binary.' }
    } finally {
        Move-Item -LiteralPath $outside -Destination $migratorPath
    }
    $installer = Join-Path $bundleTools 'register-esterium-windows-service.ps1'
    $args = @{
        BundleDir = $bundle
        DatabaseURLFile = $paths.database
        RedisURLFile = $paths.redis
        JWTSecretFile = $paths.jwt
        MetricsBearerTokenFile = $paths.metrics
        AllowedOrigin = 'https://esterium.loja.example'
        InstallDir = $install
        PlanOnly = $true
    }
    $result = @(& $installer @args) -join [Environment]::NewLine
    if ($result -notmatch 'no changes made' -or (Test-Path -LiteralPath $install)) {
        throw 'Plan mode wrote data or did not report its safe scope.'
    }
    $missingRejected = $false
    try {
        $args.RedisURLFile = Join-Path $root 'missing.secret'
        & $installer @args | Out-Null
    } catch {
        $missingRejected = $_.Exception.Message -match 'Missing, empty or non-absolute'
    }
    if (-not $missingRejected) { throw 'Missing credential file was not rejected.' }
    $args.RedisURLFile = $paths.redis
    $args.AllowedOrigin = 'http://esterium.local'
    $insecureOriginRejected = $false
    try { & $installer @args | Out-Null }
    catch { $insecureOriginRejected = $_.Exception.Message -match 'HTTPS origin' }
    if (-not $insecureOriginRejected) { throw 'Plaintext origin was not rejected.' }
    $args.AllowedOrigin = 'https://user:pass@esterium.loja.example'
    $userinfoRejected = $false
    try { & $installer @args | Out-Null }
    catch { $userinfoRejected = $_.Exception.Message -match 'HTTPS origin' }
    if (-not $userinfoRejected) { throw 'Origin userinfo was not rejected.' }
    $args.AllowedOrigin = 'https://esterium.loja.example'
    $args.ListenAddress = 'user@0.0.0.0:8443'
    $listenerUserinfoRejected = $false
    try { & $installer @args | Out-Null }
    catch { $listenerUserinfoRejected = $_.Exception.Message -match 'ListenAddress' }
    if (-not $listenerUserinfoRejected) { throw 'Listener userinfo was not rejected.' }
    $args.ListenAddress = '0.0.0.0:8443'
    $lanRejected = $false
    try { & $installer @args | Out-Null }
    catch { $lanRejected = $_.Exception.Message -match 'LAN listeners require' }
    if (-not $lanRejected) { throw 'Plaintext LAN listener was not rejected.' }
    $args.TLSCertFile = Join-Path $secrets 'lan-cert.pem'
    [IO.File]::WriteAllText($args.TLSCertFile, 'synthetic certificate test marker')
    $halfPairRejected = $false
    try { & $installer @args | Out-Null }
    catch { $halfPairRejected = $_.Exception.Message -match 'must both be supplied' }
    if (-not $halfPairRejected) { throw 'Incomplete TLS pair was not rejected.' }
    $args.TLSKeyFile = Join-Path $secrets 'lan-key.pem'
    [IO.File]::WriteAllText($args.TLSKeyFile, 'synthetic key test marker')
    $lanPlan = @(& $installer @args) -join [Environment]::NewLine
    if ($lanPlan -notmatch 'HTTPS listener' -or (Test-Path -LiteralPath $install)) {
        throw 'LAN HTTPS plan failed or created installation files.'
    }
    $args.ListenAddress = '127.0.0.1:8080'
    $args.Remove('TLSCertFile')
    $args.Remove('TLSKeyFile')
    New-Item -ItemType Directory -Path $install -ErrorAction Stop | Out-Null
    $overwriteRejected = $false
    try { & $installer @args | Out-Null }
    catch { $overwriteRejected = $_.Exception.Message -match 'already exists' }
    if (-not $overwriteRejected) { throw 'Existing installation was not protected.' }
    [IO.File]::AppendAllText((Join-Path $web 'index.html'), 'tampered')
    $tamperRejected = $false
    try { & $installer @args | Out-Null }
    catch { $tamperRejected = $_.Exception.Message -match 'SHA-256 mismatch' }
    if (-not $tamperRejected) { throw 'Tampered bundle was not rejected.' }
    # Manifest writer must fail before publishing ambiguous or gapped SQL versions.
    # This operates only on the disposable synthetic bundle created above.
    Remove-Item -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Force
    foreach ($direction in @('up', 'down')) {
        [IO.File]::WriteAllText(
            (Join-Path $migrationDir "0001_conflict.$direction.sql"),
            '-- synthetic duplicate version'
        )
    }
    $duplicateRejected = $false
    try {
        & (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $bundle | Out-Null
    } catch {
        $duplicateRejected = $_.Exception.Message -match 'Duplicate SQL migration version'
    }
    if (-not $duplicateRejected) { throw 'Manifest accepted two SQL pairs numbered 0001.' }
    foreach ($direction in @('up', 'down')) {
        Remove-Item -LiteralPath (Join-Path $migrationDir "0001_conflict.$direction.sql") -Force
        [IO.File]::WriteAllText(
            (Join-Path $migrationDir "0003_gap.$direction.sql"),
            '-- synthetic migration version gap'
        )
    }
    $gapRejected = $false
    try {
        & (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $bundle | Out-Null
    } catch {
        $gapRejected = $_.Exception.Message -match 'Missing or non-contiguous SQL migration version'
    }
    if (-not $gapRejected) { throw 'Manifest accepted a SQL gap from version 0001 to 0003.' }
    Write-Output 'PASS: synthetic manifest rejects duplicate and gapped SQL versions.'
    Write-Output 'PASS: Windows service installation plan has no side effects and rejects missing secrets/overwrites.'
}
finally {
    if (Test-Path -LiteralPath $install) { Remove-Item -LiteralPath $install -Force -Recurse }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Force -Recurse }
}
