#requires -Version 5.1
# Disposable GitHub-hosted Windows preflight. No DB/Redis/SEFAZ connections.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:OS -ne 'Windows_NT') {
    throw 'Production config preflight test requires a disposable hosted Windows CI runner.'
}
$root = Join-Path $env:RUNNER_TEMP ('esterium-preflight-' + [Guid]::NewGuid().ToString('N'))
$bundle = Join-Path $root 'web'
$secrets = Join-Path $root 'secrets'
$binary = Join-Path $root 'esterium-server.exe'
try {
    New-Item -ItemType Directory -Path $bundle, $secrets -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $bundle 'index.html'), '<html>synthetic</html>')
    $jwtPath = Join-Path $secrets 'jwt.secret'
    $metricsPath = Join-Path $secrets 'metrics.secret'
    $dbPath = Join-Path $secrets 'database.secret'
    $redisPath = Join-Path $secrets 'redis.secret'
    [IO.File]::WriteAllText($jwtPath, 'ci-Windows-preflight-JWT-secret-strong-enough-for-validation')
    [IO.File]::WriteAllText($metricsPath, 'ci-monitoring-token-synthetic-only')
    [IO.File]::WriteAllText($dbPath, 'postgres://esterium:synthetic-windows-db-secret@127.0.0.1:55439/esterium?sslmode=verify-full')
    [IO.File]::WriteAllText($redisPath, 'rediss://:synthetic-windows-redis-secret@127.0.0.1:56379/0')
    $env:APP_ENV = 'prod'
    $env:FISCAL_PROVIDER = 'disabled'
    $env:NFCE_SEFAZ_PRODUCTION_ENABLED = 'false'
    $env:NFCE_SEFAZ_HOMOLOGATION_ENABLED = 'false'
    $env:OTEL_ENABLED = 'false'
    $env:HTTP_ADDR = '127.0.0.1:8080'
    $env:ESTERIUM_WEB_DIR = $bundle
    $env:CORS_ALLOWED_ORIGINS = 'https://esterium.ci.invalid:8443'
    $env:APP_PUBLIC_URL = 'https://esterium.ci.invalid:8443'
    $env:JWT_SECRET_FILE = $jwtPath
    $env:METRICS_BEARER_TOKEN_FILE = $metricsPath
    $env:DATABASE_URL_FILE = $dbPath
    $env:REDIS_URL_FILE = $redisPath
    $env:JWT_SECRET = ''
    $env:METRICS_BEARER_TOKEN = ''
    $env:DATABASE_URL = ''
    $env:REDIS_URL = ''
    $env:ESTERIUM_TLS_CERT_FILE = ''
    $env:ESTERIUM_TLS_KEY_FILE = ''
    $env:DISABLE_REDIS = 'false'
    $env:ALLOW_DEMO_SEED = 'false'

    Push-Location (Join-Path $PSScriptRoot '..\backend')
    try {
        & go build -o $binary ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw 'Go Windows binary build failed.' }
    } finally {
        Pop-Location
    }

    function Invoke-Preflight([string[]]$StartupArgs, [int]$ExpectedStatus) {
        $out = Join-Path $root ([Guid]::NewGuid().ToString('N') + '.out')
        $err = "$out.err"
        $process = Start-Process -FilePath $binary -ArgumentList $StartupArgs -PassThru -Wait -RedirectStandardOutput $out -RedirectStandardError $err
        try {
            if ($process.ExitCode -ne $ExpectedStatus) {
                $lines = Get-Content -LiteralPath $out -ErrorAction SilentlyContinue
                throw "Preflight exit=$($process.ExitCode), expected=$ExpectedStatus. Output: $($lines -join ' ')"
            }
            $stdout = Get-Content -LiteralPath $out -Raw -ErrorAction SilentlyContinue
            if ($null -eq $stdout) { $stdout = '' }
            if ($ExpectedStatus -eq 0 -and $stdout -notmatch 'configuration preflight passed') {
                throw 'Valid preflight did not print the approved status.'
            }
            if ($stdout -match 'synthetic-windows-db-secret|synthetic-windows-redis-secret') {
                throw 'Preflight leaked a database or Redis password.'
            }
        } finally {
            Remove-Item -LiteralPath $out, $err -Force -ErrorAction SilentlyContinue
        }
    }

    Invoke-Preflight -StartupArgs @('--check-config') -ExpectedStatus 0
    $env:ESTERIUM_WEB_DIR = Join-Path $root 'missing-web'
    Invoke-Preflight -StartupArgs @('--check-config') -ExpectedStatus 1
    $env:ESTERIUM_WEB_DIR = $bundle
    [IO.File]::WriteAllText($dbPath, 'postgres://esterium:synthetic-windows-db-secret@127.0.0.1:55439/esterium?sslmode=disable')
    Invoke-Preflight -StartupArgs @('--check-config') -ExpectedStatus 1
    [IO.File]::WriteAllText($dbPath, 'postgres://esterium:synthetic-windows-db-secret@127.0.0.1:55439/esterium?sslmode=verify-full')
    Invoke-Preflight -StartupArgs @('--unknown-flag') -ExpectedStatus 2
    Write-Output 'PASS: production preflight validates URLs, SPA and unknown flags without starting external services.'
} finally {
    if (Test-Path -LiteralPath $root) {
        Remove-Item -LiteralPath $root -Force -Recurse
    }
}
