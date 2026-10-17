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
    [IO.File]::WriteAllText((Join-Path $web 'index.html'), 'synthetic-html')
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
        'verify-esterium-bundle.ps1',
        'postgres-daily-backup.ps1',
        'copy-verified-external-backup.ps1'
    )) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $tool) -Destination (Join-Path $bundleTools $tool)
    }
    [IO.File]::WriteAllText((Join-Path $bundle 'README_LOCAL.md'), 'Synthetic setup directions')
    & (Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1') -BundleDir $bundle | Out-Null
    & (Join-Path $PSScriptRoot 'verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
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
    $args.AllowedOrigin = 'https://esterium.loja.example'
    $args.ListenAddress = '0.0.0.0:8443'
    $lanRejected = $false
    try { & $installer @args | Out-Null }
    catch { $lanRejected = $_.Exception.Message -match 'LAN listeners require' }
    if (-not $lanRejected) { throw 'Plaintext LAN listener was not rejected.' }
    $args.TLSCertFile = Join-Path $secrets 'lan-cert.pem'
    $args.TLSKeyFile = Join-Path $secrets 'lan-key.pem'
    [IO.File]::WriteAllText($args.TLSCertFile, 'synthetic certificate test marker')
    $halfPairRejected = $false
    try { & $installer @args | Out-Null }
    catch { $halfPairRejected = $_.Exception.Message -match 'must both be supplied' }
    if (-not $halfPairRejected) { throw 'Incomplete TLS pair was not rejected.' }
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
    Write-Output 'PASS: Windows service installation plan has no side effects and rejects missing secrets/overwrites.'
}
finally {
    if (Test-Path -LiteralPath $install) { Remove-Item -LiteralPath $install -Force -Recurse }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Force -Recurse }
}
