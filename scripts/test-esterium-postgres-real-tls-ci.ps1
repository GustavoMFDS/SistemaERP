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
$dbInit=Join-Path $env:ProgramData ('Esterium\PgAppCI-'+$id)
$appPwdFile=Join-Path $root 'pg-app.secret'
$appPassword='ci-local-erp-pg-app-'+$id
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
function Invoke-PgCtl([string[]]$argList,[string]$phase) {
    # pg_ctl launches postgres.exe. Avoid PowerShell pipelines that can
    # remain open if the service inherits their stdout or stderr handles.
    $info=[Diagnostics.ProcessStartInfo]::new()
    $info.FileName=$pgCtl
    $info.UseShellExecute=$false
    $info.CreateNoWindow=$true
    foreach ($value in $argList) { [void]$info.ArgumentList.Add($value) }
    $proc=[Diagnostics.Process]::Start($info)
    if ($null -eq $proc) { throw "pg_ctl did not start during $phase." }
    try {
        if (-not $proc.WaitForExit(60000)) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            throw "pg_ctl timed out after one minute during $phase."
        }
        if ($proc.ExitCode -ne 0) { throw "pg_ctl failed during $phase (exit $($proc.ExitCode))." }
    } finally { $proc.Dispose() }
}
function Run-Psql([string]$sql) {
    $out=@(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' $sql)
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL TLS smoke query failed.' }
    return (@($out) -join '').Trim()
}
function Must-Deny([string]$description) {
    $code=0
    try {
        & $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' 'SELECT 1' 2>$null | Out-Null
        $code=$LASTEXITCODE
    } catch {
        # Some PowerShell versions surface expected psql authentication/TLS
        # failures as terminating native-command errors.
        if ($LASTEXITCODE -eq 0) { throw }
        $code=$LASTEXITCODE
    }
    if ($code -eq 0) { throw "PostgreSQL accepted forbidden $description connection." }
}
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($passwordFile,$pass+[Environment]::NewLine)
    $runnerSID=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    & icacls.exe $passwordFile '/inheritance:r' '/grant:r' ('*'+$runnerSID+':R') '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict synthetic PostgreSQL test password.' }
    $certGen=Join-Path $PSScriptRoot 'esterium-pg-tls-certgen.exe'
    if (-not (Test-Path -LiteralPath $certGen -PathType Leaf)) {
        throw 'Test must compile offline TLS generator before running.'
    }
    & (Join-Path $PSScriptRoot 'provision-esterium-postgres-tls-policy.ps1') -DestinationDir $policy -PostgresPort $port -GeneratorExe $certGen -Apply -UnsignedDisposableCIFixture | Out-Null
    $policyConfig=Join-Path $policy 'postgresql.conf'
    $policyHba=Join-Path $policy 'pg_hba.conf'
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $policyConfig -PgHbaConf $policyHba | Out-Null
    Write-Host 'PostgreSQL CI: downloading pinned EDB Windows ZIP and validating its SHA-256.'
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
    Write-Host 'PostgreSQL CI: initializing new isolated database cluster.'
    & $initdb '-D' $data '--username=postgres' ('--pwfile='+$passwordFile) '--auth-host=scram-sha-256' '--auth-local=scram-sha-256' '--encoding=UTF8' '--no-instructions' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Real PostgreSQL initdb failed on isolated Windows host.' }
    # Exercise the exact first-install config installer, not a CI-only
    # Copy-Item shortcut. The vendor cluster is fresh and service is never
    # registered; only the hosted fixture bypasses SCM signature checks.
    $proof=Join-Path $root 'PostgresFresh-EsteriumPostgreSQL16.marker'
    [IO.File]::WriteAllText($proof,
        'ESTERIUM_FRESH_POSTGRES_16'+[Environment]::NewLine+
        'EsteriumPostgreSQL16'+[Environment]::NewLine+[IO.Path]::GetFullPath($data))
    $env:ESTERIUM_PG_ACTIVATE_DISPOSABLE_CI='1'
    & (Join-Path $PSScriptRoot 'stage-esterium-postgres-tls.ps1') -PostgresDataDir $data -TlsPolicyDir $policy -FreshClusterMarker $proof -PostgresPort $port -UnsignedDisposableCIFixture -Apply | Out-Null
    if (-not (Test-Path -LiteralPath (Join-Path $root 'OriginalPostgresql-EsteriumPostgreSQL16.conf') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $root 'OriginalPgHba-EsteriumPostgreSQL16.conf') -PathType Leaf)) {
        throw 'Real PostgreSQL startup test must retain original initdb configuration.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf (Join-Path $data 'postgresql.conf') -PgHbaConf (Join-Path $data 'pg_hba.conf') | Out-Null
    # The disposable runner runs postgres under its current Windows SID,
    # whereas real installation grants the freshly created service-account SID.
    # Grant the CI identity explicit key read without relaxing production ACLs.
    $tlsDir=Join-Path $policy 'certificates'
    foreach ($folder in @($policy,$tlsDir)) {
        & icacls.exe $folder '/grant:r' ('*'+$runnerSID+':RX') | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Unable to grant synthetic PostgreSQL directory traversal.' }
    }
    foreach ($name in @('server.crt','server.key')) {
        & icacls.exe (Join-Path $tlsDir $name) '/grant:r' ('*'+$runnerSID+':R') | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Unable to grant synthetic PostgreSQL service key read.' }
    }
    Write-Host 'PostgreSQL CI: starting TLS scratch cluster.'
    try {
        Invoke-PgCtl -argList @('-D',$data,'-l',$log,'-w','start') -phase 'TLS startup'
    } catch {
        if (Test-Path -LiteralPath $log) {
            Write-Host 'PostgreSQL CI startup log (last 35 lines, no credentials):'
            foreach ($line in @(Get-Content -LiteralPath $log -Tail 35)) { Write-Host $line }
        }
        throw
    }
    $started=$true
    Write-Host 'PostgreSQL CI: checking verify-full TLS sessions and SCRAM.'
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
    Write-Host 'PostgreSQL CI: testing reject paths for plaintext, CA, hostname and password.'
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
    Write-Host 'PostgreSQL CI: bootstrapping nonprivileged ERP SQL role and protected DATABASE_URL_FILE.'
    [IO.File]::WriteAllText($appPwdFile,$appPassword+[Environment]::NewLine)
    & icacls.exe $appPwdFile '/inheritance:r' '/grant:r' ('*'+$runnerSID+':R') '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not secure synthetic PostgreSQL application password.' }
    $initializer=Join-Path $PSScriptRoot 'initialize-esterium-postgres-app-db.ps1'
    $env:ESTERIUM_DB_BOOTSTRAP_DISPOSABLE_CI='1'
    $params=@{
        PsqlExe=$psql
        SuperPasswordFile=$passwordFile
        AppPasswordFile=$appPwdFile
        RootCertificateFile=(Join-Path $policy 'certificates\root.crt')
        DestinationDir=$dbInit
        PostgresPort=$port
        UnsignedDisposableCIFixture=$true
        Apply=$true
    }
    & $initializer @params | Out-Null
    $urlFile=Join-Path $dbInit 'database-url.secret'
    $url=[IO.File]::ReadAllText($urlFile).Trim()
    if ($url -notmatch '^postgres://esterium_app:[A-Za-z0-9_-]{48,}@localhost:64339/esterium\?sslmode=verify-full&sslrootcert=') {
        throw 'Initializer produced insecure database URL.'
    }
    $who=$env:PGUSER
    $whichDB=$env:PGDATABASE
    $env:PGUSER='esterium_app'
    $env:PGDATABASE='esterium'
    $env:PGPASSWORD=$appPassword
    if ((Run-Psql 'SELECT current_user;') -cne 'esterium_app') {
        throw 'New ERP application role does not authenticate through TLS.'
    }
    $permissions=Run-Psql 'SELECT rolsuper::int::text||rolcreatedb::int::text||rolcreaterole::int::text||rolreplication::int::text||rolbypassrls::int::text FROM pg_roles WHERE rolname=current_user;'
    if ($permissions -cne '00000') { throw 'ERP SQL role has dangerous cluster privileges.' }
    $env:PGUSER=$who
    $env:PGDATABASE=$whichDB
    $env:PGPASSWORD=$pass
    $replayAllowed=$false
    try { & $initializer @params | Out-Null; $replayAllowed=$true }
    catch {
        if ($_.Exception.Message -notmatch 'nao substituir credenciais') { throw }
    }
    if ($replayAllowed) { throw 'Database bootstrap unexpectedly allowed replay.' }
    Write-Host 'PostgreSQL CI: exercising pinned initial schema migrations under dedicated TLS app role.'
    $migrateTool=Join-Path $PSScriptRoot 'esterium-migrate.exe'
    $schemaTool=Join-Path $PSScriptRoot 'apply-esterium-initial-schema.ps1'
    $migrationFolder=Join-Path $PSScriptRoot '..\backend\migrations'
    $env:ESTERIUM_INITIAL_MIGRATIONS_DISPOSABLE_CI='1'
    $schemaParams=@{
        DatabaseURLFile=$urlFile
        PsqlExe=$psql
        MigrateExe=$migrateTool
        MigrationsDir=$migrationFolder
        UnsignedDisposableCIFixture=$true
    }
    # Verify both migration state and crash recovery in the ERP database,
    # never in the initial administrative maintenance database.
    $env:PGUSER='esterium_app'
    $env:PGDATABASE='esterium'
    $env:PGPASSWORD=$appPassword
    $schemaPlan=@(& $schemaTool @schemaParams) -join ' '
    if ($schemaPlan -notmatch 'PLAN: aplicar migracoes' -or
        (Run-Psql "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE';") -cne '0') {
        throw 'Initial SQL migrations plan changed a disposable database.'
    }
    $schemaParams.Apply=$true
    Write-Host 'PostgreSQL CI: invoking initial schema migration.'
    & $schemaTool @schemaParams | Out-Null
    Write-Host 'PostgreSQL CI: first schema migration command completed.'
    $version=Run-Psql "SELECT version::text||':'||dirty::text FROM public.schema_migrations;"
    if ($version -cne '51:false') { throw 'Full initial schema migrations did not reach expected version 51.' }
    $replayAllowed=$false
    Write-Host 'PostgreSQL CI: invoking expected replay rejection.'
    try { & $schemaTool @schemaParams | Out-Null; $replayAllowed=$true }
    catch { if ($_.Exception.Message -notmatch 'nao esta vazio') { throw } }
    if ($replayAllowed) { throw 'Initial schema tool unexpectedly permitted migration replay.' }
    Write-Host 'PostgreSQL CI: testing confirmed transaction durability.'
    $null=Run-Psql 'CREATE TABLE ci_durable(id integer PRIMARY KEY)'
    $null=Run-Psql 'INSERT INTO ci_durable VALUES(17)'
    Write-Host 'PostgreSQL CI: initiating immediate shutdown for WAL recovery test.'
    Invoke-PgCtl -argList @('-D',$data,'-m','immediate','-w','stop') -phase 'immediate stop'
    $started=$false
    Write-Host 'PostgreSQL CI: starting recovery from immediate shutdown.'
    Invoke-PgCtl -argList @('-D',$data,'-l',$log,'-w','start') -phase 'crash recovery'
    $started=$true
    if ((Run-Psql 'SELECT count(*) FROM ci_durable WHERE id=17') -cne '1') {
        throw 'PostgreSQL did not recover acknowledged transaction after forced stop.'
    }
    # Independent logical backup/restore on a SECOND disposable database.
    # This is separate from WAL crash replay; ensure the acknowledged row
    # and complete 51-step schema survive an actual pg_dump/pg_restore pair.
    Write-Host 'PostgreSQL CI: validating independent logical pg_dump/pg_restore after crash recovery.'
    $pgDump=Join-Path $bin 'pg_dump.exe'
    $pgRestore=Join-Path $bin 'pg_restore.exe'
    $backup=Join-Path $root 'esterium-ci-logical.backup'
    if (-not (Test-Path -LiteralPath $pgDump -PathType Leaf) -or
        -not (Test-Path -LiteralPath $pgRestore -PathType Leaf)) {
        throw 'Approved PostgreSQL Windows ZIP must include pg_dump and pg_restore.'
    }
    $env:PGUSER='esterium_app'
    $env:PGDATABASE='esterium'
    $env:PGPASSWORD=$appPassword
    & $pgDump '-h' 'localhost' '-p' ([string]$port) '-U' 'esterium_app' '-d' 'esterium' '--format=custom' '--no-owner' '--no-acl' ('--file='+$backup)
    if ($LASTEXITCODE -ne 0 -or
        -not (Test-Path -LiteralPath $backup -PathType Leaf) -or
        (Get-Item -LiteralPath $backup).Length -lt 4096) {
        throw 'PostgreSQL did not produce a nontrivial local logical backup.'
    }
    $backupHash=(Get-FileHash -LiteralPath $backup -Algorithm SHA256).Hash
    if ($backupHash -notmatch '^[A-F0-9]{64}$') { throw 'Logical backup SHA-256 is invalid.' }
    $env:PGUSER='postgres'
    $env:PGDATABASE='postgres'
    $env:PGPASSWORD=$pass
    $null=Run-Psql 'CREATE DATABASE esterium_restore_ci OWNER esterium_app TEMPLATE template0'
    $env:PGUSER='esterium_app'
    $env:PGDATABASE='esterium_restore_ci'
    $env:PGPASSWORD=$appPassword
    if ((Get-FileHash -LiteralPath $backup -Algorithm SHA256).Hash -cne $backupHash) {
        throw 'Logical backup content changed between export and recovery.'
    }
    & $pgRestore '--host=localhost' ('--port='+$port) '--username=esterium_app' '--dbname=esterium_restore_ci' '--no-owner' '--no-acl' '--exit-on-error' '--single-transaction' $backup
    if ($LASTEXITCODE -ne 0) {
        throw 'Independent PostgreSQL backup could not restore into a new database.'
    }
    if ((Run-Psql "SELECT version::text||':'||dirty::text FROM public.schema_migrations") -cne '51:false' -or
        (Run-Psql 'SELECT count(*) FROM public.ci_durable WHERE id=17') -cne '1') {
        throw 'Independently restored PostgreSQL database lost the schema version or confirmed test row.'
    }
    Write-Output 'PASS: independent PostgreSQL 16 custom-format pg_dump/pg_restore preserved 51 migrations and confirmed data in a second database.'
    Write-Output 'PASS: PostgreSQL 16 TLS verify-full, SCRAM, nonprivileged ERP role, protected URL, initial 51 migrations and crash recovery.'
} finally {
    if ($started -and $pgCtl -and (Test-Path -LiteralPath $pgCtl)) {
        try { Invoke-PgCtl -argList @('-D',$data,'-m','immediate','-w','stop') -phase 'cleanup' }
        catch { Write-Warning 'Disposable PostgreSQL server needs runner teardown.' }
    }
    foreach ($entry in $oldEnv.GetEnumerator()) {
        [Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')
    }
    Remove-Item Env:ESTERIUM_PG_ACTIVATE_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_PG_POLICY_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_DB_BOOTSTRAP_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_INITIAL_MIGRATIONS_DISPOSABLE_CI -ErrorAction SilentlyContinue
    foreach ($path in @($policy,$dbInit,$root)) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
