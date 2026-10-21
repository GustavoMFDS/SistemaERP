#requires -Version 5.1
<#
  Real PostgreSQL 16 smoke, only on disposable GitHub-hosted Windows runner.
  Downloads vendor-published SHA-256-pinned community ZIP (not EDB installer);
  starts a temporary scratch cluster, verifies local TLS/SCRAM and recovery.
  NEVER touches installed services, customer databases or the main branch.
#>
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Real PostgreSQL TLS verification requires disposable hosted Windows CI.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP ('esterium-postgres-real-'+$id)
$policy=Join-Path $env:ProgramData ('Esterium\PgTlsRealCI-'+$id)
$data=Join-Path $root 'data'
$unpacked=Join-Path $root 'binaries'
$zip=Join-Path $root 'vendor.zip'
$wrongRoot=Join-Path $root 'wrong-cert'
$log=Join-Path $root 'postgres.log'
$passwordFile=Join-Path $root 'pg-password.secret'
$port=64339
$pass='ci-postgres-secret-'+$id
$pgCtl=$null
$psql=$null
$started=$false
$oldEnv=@{}
$env:ESTERIUM_PG_POLICY_DISPOSABLE_CI='1'
foreach ($key in @('PGPASSWORD','PGHOST','PGPORT','PGUSER','PGDATABASE','PGSSLROOTCERT','PGSSLMODE','PGHOSTADDR','PATH')) {
    $oldEnv[$key]=[Environment]::GetEnvironmentVariable($key,'Process')
}
function Run-Psql([string]$sql) {
    $out=@(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' $sql)
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL TLS smoke query failed.' }
    return (@($out) -join '').Trim()
}
function Must-Deny([string]$description) {
    $null=& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' 'SELECT 1' 2>&1
    if ($LASTEXITCODE -eq 0) { throw "PostgreSQL accepted forbidden $description connection." }
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($passwordFile,$pass+[Environment]::NewLine)
    & icacls.exe $passwordFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict synthetic PostgreSQL test password.' }
    $certGen=Join-Path $PSScriptRoot 'esterium-pg-tls-certgen.exe'
    if (-not (Test-Path -LiteralPath $certGen -PathType Leaf)) {
        throw 'Test must compile offline TLS generator before running.'
    }
    & (Join-Path $PSScriptRoot 'provision-esterium-postgres-tls-policy.ps1') -DestinationDir $policy -PostgresPort $port -GeneratorExe $certGen -Apply -UnsignedDisposableCIFixture | Out-Null
    $policyConfig=Join-Path $policy 'postgresql.conf'
    $policyHba=Join-Path $policy 'pg_hba.conf'
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $policyConfig -PgHbaConf $policyHba | Out-Null
    # Hash independently published by EnterpriseDB for the exact 16.15-1 ZIP.
    $url='https://get.enterprisedb.com/postgresql/postgresql-16.15-1-windows-x64-binaries.zip'
    Invoke-WebRequest -Uri $url -OutFile $zip -ErrorAction Stop
    $hash=(Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -cne '25e6fcdfb8caec38691bf461125e7564508760666f7b8e5dc6a5f0818f58f81e') {
        throw 'Vendor PostgreSQL Windows ZIP hash mismatch: no third-party binary can execute.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $unpacked -ErrorAction Stop
    $initdbMatches=@(Get-ChildItem -LiteralPath $unpacked -Recurse -Filter 'initdb.exe' -File)
    if ($initdbMatches.Count -ne 1) { throw 'Expected exactly one PostgreSQL initdb.exe in approved ZIP.' }
    $bin=$initdbMatches[0].DirectoryName
    $initdb=$initdbMatches[0].FullName
    $pgCtl=Join-Path $bin 'pg_ctl.exe'
    $psql=Join-Path $bin 'psql.exe'
    if (-not (Test-Path -LiteralPath $pgCtl -PathType Leaf) -or
        -not (Test-Path -LiteralPath $psql -PathType Leaf)) {
        throw 'Approved PostgreSQL binary ZIP lacks pg_ctl or psql.'
    }
    $env:PATH=$bin+';'+$oldEnv['PATH']
    & $initdb '-D' $data '--username=postgres' ('--pwfile='+$passwordFile) '--auth-host=scram-sha-256' '--encoding=UTF8' '--no-instructions' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Real PostgreSQL initdb failed on isolated Windows host.' }
    Copy-Item -LiteralPath $policyConfig -Destination (Join-Path $data 'postgresql.conf') -Force
    Copy-Item -LiteralPath $policyHba -Destination (Join-Path $data 'pg_hba.conf') -Force
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf (Join-Path $data 'postgresql.conf') -PgHbaConf (Join-Path $data 'pg_hba.conf') | Out-Null
    & $pgCtl '-D' $data '-l' $log '-w' 'start' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Real PostgreSQL refused prepared TLS/SCRAM config.' }
    $started=$true
    $env:PGHOST='localhost'
    $env:PGPORT=[string]$port
    $env:PGUSER='postgres'
    $env:PGDATABASE='postgres'
    $env:PGPASSWORD=$pass
    $env:PGSSLROOTCERT=Join-Path $policy 'certificates\root.crt'
    $env:PGSSLMODE='verify-full'
    if ((Run-Psql 'SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid()') -cne 't') {
        throw 'PostgreSQL did not negotiate TLS under verify-full.'
    }
    if ((Run-Psql "SHOW password_encryption") -cne 'scram-sha-256') {
        throw 'PostgreSQL session does not enforce SCRAM for new passwords.'
    }
    if ((Run-Psql "SHOW listen_addresses") -cne 'localhost') {
        throw 'PostgreSQL server exposed an unexpected listen interface.'
    }
    if ((Run-Psql 'SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL') -cne '0') {
        throw 'PostgreSQL reported rejected or malformed HBA rules.'
    }
    $env:PGSSLMODE='disable'
    Must-Deny 'plaintext'
    $env:PGSSLMODE='verify-full'
    [IO.Directory]::CreateDirectory($wrongRoot) | Out-Null
    & $certGen '-out-dir' $wrongRoot | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not create independent fake CA for negative verification.' }
    $env:PGSSLROOTCERT=Join-Path $wrongRoot 'root.crt'
    Must-Deny 'untrusted TLS root'
    $env:PGSSLROOTCERT=Join-Path $policy 'certificates\root.crt'
    $env:PGHOST='invalid.example'
    $env:PGHOSTADDR='127.0.0.1'
    Must-Deny 'TLS hostname mismatch'
    $env:PGHOST='localhost'
    Remove-Item Env:PGHOSTADDR -ErrorAction SilentlyContinue
    $env:PGPASSWORD='synthetic-incorrect'
    Must-Deny 'incorrect SCRAM password'
    $env:PGPASSWORD=$pass
    $null=Run-Psql 'CREATE TABLE ci_durable(id integer PRIMARY KEY)'
    $null=Run-Psql 'INSERT INTO ci_durable VALUES(17)'
    & $pgCtl '-D' $data '-m' 'immediate' '-w' 'stop' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL test failed to force an immediate shutdown.' }
    $started=$false
    & $pgCtl '-D' $data '-l' $log '-w' 'start' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL TLS cluster did not recover after immediate shutdown.' }
    $started=$true
    if ((Run-Psql 'SELECT count(*) FROM ci_durable WHERE id=17') -cne '1') {
        throw 'PostgreSQL did not recover acknowledged transaction after forced stop.'
    }
    Write-Output 'PASS: PostgreSQL 16 official Windows ZIP, strict TLS verify-full, SCRAM, HBA denials and crash recovery.'
} finally {
    if ($started -and $pgCtl -and (Test-Path -LiteralPath $pgCtl)) {
        & $pgCtl '-D' $data '-m' 'immediate' '-w' 'stop' | Out-Null
    }
    foreach ($entry in $oldEnv.GetEnumerator()) {
        [Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')
    }
    Remove-Item Env:ESTERIUM_PG_POLICY_DISPOSABLE_CI -ErrorAction SilentlyContinue
    foreach ($path in @($policy,$root)) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
