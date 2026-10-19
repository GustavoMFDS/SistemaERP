#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Static PostgreSQL policy fixture requires disposable hosted Windows CI.'
}
$root=Join-Path $env:RUNNER_TEMP ('esterium-postgres-policy-' + [Guid]::NewGuid().ToString('N'))
$pg=Join-Path $root 'postgresql.conf'
$hba=Join-Path $root 'pg_hba.conf'
$gate=Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1'
$healthyPg=@'
listen_addresses = 'localhost'
ssl = on
ssl_min_protocol_version = 'TLSv1.2'
ssl_cert_file = 'C:/certs/postgres.crt'
ssl_key_file = 'C:/certs/postgres.key'
password_encryption = 'scram-sha-256'
shared_buffers = 128MB
'@
$healthyHba=@'
# Only encrypted loopback access, reject non-TLS connections
hostnossl all all 0.0.0.0/0 reject
hostnossl all all ::/0 reject
hostssl all all 127.0.0.1/32 scram-sha-256
hostssl all all ::1/128 scram-sha-256
'@
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($pg,$healthyPg)
    [IO.File]::WriteAllText($hba,$healthyHba)
    $params=@{PostgresqlConf=$pg;PgHbaConf=$hba}
    $valid=@(& $gate @params) -join ' '
    if ($valid -notmatch 'PASS: static PostgreSQL config files') {
        throw 'Safe synthetic PostgreSQL policy was not accepted.'
    }
    function Must-Reject([string]$label,[string]$pattern) {
        $msg='(accepted)'
        try { & $gate @params | Out-Null } catch { $msg=$_.Exception.Message }
        if ($msg -notmatch $pattern) { throw "Policy accepted $label or rejected for another reason: $msg" }
    }
    [IO.File]::WriteAllText($pg, $healthyPg.Replace("'localhost'","'*'"))
    Must-Reject 'all network listeners' 'listen only on loopback'
    [IO.File]::WriteAllText($pg, $healthyPg.Replace('ssl = on','ssl = off'))
    Must-Reject 'TLS disabled' 'require TLS transport'
    [IO.File]::WriteAllText($pg, $healthyPg.Replace("'scram-sha-256'","'md5'"))
    Must-Reject 'weak password scheme' 'SCRAM-SHA-256'
    [IO.File]::WriteAllText($pg, $healthyPg + [Environment]::NewLine + "include_dir = 'conf.d'")
    Must-Reject 'unverified include directory' 'overridden or included'
    [IO.File]::WriteAllText($pg, $healthyPg + [Environment]::NewLine + 'ssl = off')
    Must-Reject 'duplicated contradictory SSL directives' 'declared more than once'
    [IO.File]::WriteAllText($pg,$healthyPg)
    [IO.File]::WriteAllText($hba,$healthyHba.Replace('hostssl all all 127.0.0.1/32','host all all 127.0.0.1/32'))
    Must-Reject 'non-TLS successful login' 'Non-TLS'
    [IO.File]::WriteAllText($hba,$healthyHba.Replace('127.0.0.1/32 scram-sha-256','0.0.0.0/0 scram-sha-256'))
    Must-Reject 'LAN authentication' 'Only loopback'
    [IO.File]::WriteAllText($hba,$healthyHba.Replace('::1/128 scram-sha-256','::1/128 trust'))
    Must-Reject 'trust authentication' 'Only loopback'
    [IO.File]::WriteAllText($hba,$healthyHba + [Environment]::NewLine + "include 'weak.conf'")
    Must-Reject 'unreviewed HBA includes' 'included files'
    [IO.File]::WriteAllText($hba,$healthyHba)
    $final=@(& $gate @params) -join ' '
    if ($final -notmatch 'NOT VERIFIED: actual running PostgreSQL settings') {
        throw 'Static policy checker failed to disclose runtime validation limits.'
    }
    Write-Output 'PASS: synthetic PostgreSQL policy denies LAN, non-TLS, trust, MD5, includes and contradictory settings.'
} finally {
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
