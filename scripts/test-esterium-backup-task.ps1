#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:OS -ne 'Windows_NT') {
    throw 'Synthetic backup scheduler test requires a disposable hosted Windows runner.'
}
$root = Join-Path $env:RUNNER_TEMP ('esterium-task-' + [Guid]::NewGuid().ToString('N'))
$configPath = Join-Path $root 'backup-config.json'
$passPath = Join-Path $root 'pgpass.secret'
$cert = Join-Path $root 'ca.crt'
$bin = Join-Path $root 'pgtools'
$local = Join-Path $root 'archives'
$runner = Join-Path $PSScriptRoot 'run-esterium-scheduled-backup.ps1'
$registration = Join-Path $PSScriptRoot 'register-esterium-backup-task.ps1'
try {
    [IO.Directory]::CreateDirectory($bin) | Out-Null
    [IO.Directory]::CreateDirectory($local) | Out-Null
    foreach ($name in @('pg_dump.exe', 'pg_restore.exe', 'psql.exe')) {
        [IO.File]::WriteAllText((Join-Path $bin $name), 'synthetic-not-executable')
    }
    [IO.File]::WriteAllText($passPath, 'fake:credential:test:data:synthetic')
    [IO.File]::WriteAllText($cert, 'fake-test-CA')
    $config = @{
        PGHOST='127.0.0.1'; PGPORT='5432'; PGUSER='esterium_backup'
        PGDATABASE='esterium'; PGPASSFILE=$passPath; PGSSLROOTCERT=$cert
        BackupDir=$local; ExternalBackupDir='\\other-computer\protected-backups'
        PostgresBin=$bin; RetentionDays=14
    }
    function Write-Config {
        [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Compress))
    }
    Write-Config
    foreach ($file in @($configPath, $passPath)) {
        $result = & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-20:R' '*S-1-5-18:F' '*S-1-5-32-544:F' 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Synthetic ACL setup failed: $result" }
    }
    $accepted = @(& $runner -ConfigFile $configPath -CheckOnly) -join ' '
    if ($accepted -notmatch 'PASS: protected backup task configuration validated') {
        throw 'Valid protected backup config was rejected.'
    }
    $plan = @(& $registration -ConfigFile $configPath -DailyAt '02:00') -join ' '
    if ($plan -notmatch 'no task, data folder, network connection' -or
        @(Get-ScheduledTask -TaskName 'EsteriumERP-Backup' -ErrorAction SilentlyContinue).Count -ne 0) {
        throw 'Backup plan mode unexpectedly installed or enabled a scheduled task.'
    }
    function Assert-Reject([string]$reason) {
        $rejected = $false
        try { & $runner -ConfigFile $configPath -CheckOnly | Out-Null }
        catch { $rejected = $true }
        if (-not $rejected) { throw "Backup preflight accepted $reason" }
    }
    $config['PGPASSWORD'] = 'this-is-not-a-real-secret'
    Write-Config
    Assert-Reject 'an inline password property'
    $config.Remove('PGPASSWORD')
    $config.ExternalBackupDir = $local
    Write-Config
    Assert-Reject 'a local-only destination'
    $config.ExternalBackupDir = '\\localhost\protected-backups'
    Write-Config
    Assert-Reject 'an SMB share on the same machine'
    $config.ExternalBackupDir = '\\other-computer\protected-backups'
    $config.PGHOST = '192.168.0.5'
    Write-Config
    Assert-Reject 'a remote database server'
    $config.PGHOST = '127.0.0.1'
    Write-Config
    $aclOutput = & icacls.exe $passPath '/grant' '*S-1-5-32-545:R' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Synthetic insecure ACL setup failed: $aclOutput" }
    Assert-Reject 'PostgreSQL password file readable by Users'
    $aclOutput = & icacls.exe $passPath '/remove:g' '*S-1-5-32-545' 2>&1
    if ($LASTEXITCODE -ne 0) { throw "ACL cleanup failed: $aclOutput" }
    $badTime = $false
    try { & $registration -ConfigFile $configPath -DailyAt '25:90' | Out-Null }
    catch { $badTime = $true }
    if (-not $badTime) { throw 'Invalid scheduling time was accepted.' }
    Write-Output 'PASS: backup scheduler plan is read-only and rejects insecure credentials, disk and configuration.'
} finally {
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
