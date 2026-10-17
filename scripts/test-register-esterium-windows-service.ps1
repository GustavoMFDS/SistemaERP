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
    $installer = Join-Path $PSScriptRoot 'register-esterium-windows-service.ps1'
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
    New-Item -ItemType Directory -Path $install -ErrorAction Stop | Out-Null
    $overwriteRejected = $false
    try { & $installer @args | Out-Null }
    catch { $overwriteRejected = $_.Exception.Message -match 'already exists' }
    if (-not $overwriteRejected) { throw 'Existing installation was not protected.' }
    Write-Output 'PASS: Windows service installation plan has no side effects and rejects missing secrets/overwrites.'
}
finally {
    if (Test-Path -LiteralPath $install) { Remove-Item -LiteralPath $install -Force -Recurse }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Force -Recurse }
}
