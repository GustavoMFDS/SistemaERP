#requires -Version 5.1
[CmdletBinding()]
param([switch]$FromIntegratedStage)
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
# A fixed high port may be in a Windows excluded/reserved port range and
# cause WSAEACCES, even on a disposable hosted runner. Let the OS select
# an available loopback port before generating the TLS policy.
$portListener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
try {
    $portListener.Start()
    $port=([Net.IPEndPoint]$portListener.LocalEndpoint).Port
} finally {
    $portListener.Stop()
}
if ($port -lt 1024 -or $port -gt 65535) {
    throw 'Windows failed to select a valid temporary PostgreSQL port.'
}
$pass='ci-postgres-secret-'+$id
$pgCtl=$null
$psql=$null
$started=$false
$secondStarted=$false
$secondData=Join-Path $root 'recovery-independent-cluster'
$secondLog=Join-Path $root 'recovery-independent.log'
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
    if($FromIntegratedStage) {
        # Use the actual files created and verified by the MIT setup EXE.
        # The TLS test is still CI-only and cannot execute on a user's PC.
        if($env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1' -or
           $env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1') {
            throw 'Integrated TLS/schema CI requires both ephemeral Windows CI guards.'
        }
        $bin=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin\pgsql\bin'
        $vendorZip=Join-Path $env:ProgramFiles 'EsteriumOpenSourceVendorStage\postgresql-binaries.zip'
        $erp=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage\bundle-manifest.json'
        if(-not (Test-Path -LiteralPath $erp -PathType Leaf) -or
           -not (Test-Path -LiteralPath $vendorZip -PathType Leaf) -or
           (Get-FileHash -LiteralPath $vendorZip -Algorithm SHA256).Hash -cne
                '43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281' -or
           -not (Test-Path -LiteralPath (Join-Path $bin 'initdb.exe') -PathType Leaf)) {
            throw 'Native MIT ZIP stage or independently vendor-pinned SHA missing for TLS test.'
        }
        $initdb=Join-Path $bin 'initdb.exe'
        Write-Output 'PASS: TLS/schema CI now uses the PostgreSQL binaries extracted by the native MIT EXE.'
    } else {
        Write-Host 'PostgreSQL CI: downloading pinned EDB Windows ZIP and validating its SHA-256.'
        # Hash independently published by EnterpriseDB for the exact 16.15-5 ZIP:
        # https://github.com/EnterpriseDB/edb-installers/issues/731 .
        $url='https://get.enterprisedb.com/postgresql/postgresql-16.15-5-windows-x64-binaries.zip'
        Invoke-WebRequest -Uri $url -OutFile $zip -ErrorAction Stop
        $hash=(Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($hash -cne '43bb45f173a6f08cf1d29a97a6d8deb119e8e8093a24c00d2d1001a0ccaa8281') {
            throw 'Vendor PostgreSQL Windows ZIP hash mismatch: no third-party binary can execute.'
        }
        Expand-Archive -LiteralPath $zip -DestinationPath $unpacked -ErrorAction Stop
        $initdbMatches=@(Get-ChildItem -LiteralPath $unpacked -Recurse -Filter 'initdb.exe' -File)
        if ($initdbMatches.Count -ne 1) { throw 'Expected exactly one PostgreSQL initdb.exe in approved ZIP.' }
        $bin=$initdbMatches[0].DirectoryName
        $initdb=$initdbMatches[0].FullName
    }
    $pgCtl=Join-Path $bin 'pg_ctl.exe'
    $psql=Join-Path $bin 'psql.exe'
    if (-not (Test-Path -LiteralPath $pgCtl -PathType Leaf) -or
        -not (Test-Path -LiteralPath $psql -PathType Leaf)) {
        throw 'Approved PostgreSQL binary ZIP lacks pg_ctl or psql.'
    }
    $env:PATH=$bin+';'+$oldEnv['PATH']
    Write-Host 'PostgreSQL CI: initializing new isolated database cluster.'
    # Capture only bounded, redacted diagnostic output on test failure.
    # The password travels exclusively through --pwfile, NEVER argv/logs.
    $initInfo=[Diagnostics.ProcessStartInfo]::new()
    $initInfo.FileName=$initdb
    $initInfo.UseShellExecute=$false
    $initInfo.CreateNoWindow=$true
    $initInfo.RedirectStandardOutput=$true
    $initInfo.RedirectStandardError=$true
    foreach($value in @('-D',$data,'--username=postgres',('--pwfile='+$passwordFile),
        '--auth-host=scram-sha-256','--auth-local=scram-sha-256',
        '--encoding=UTF8','--no-instructions')){
        [void]$initInfo.ArgumentList.Add($value)
    }
    $initProc=[Diagnostics.Process]::Start($initInfo)
    if($null -eq $initProc){throw 'Could not start PostgreSQL initdb process.'}
    try {
        $initOut=$initProc.StandardOutput.ReadToEnd()
        $initErr=$initProc.StandardError.ReadToEnd()
        if(-not $initProc.WaitForExit(120000)){
            try{$initProc.Kill()}catch{}
            throw 'PostgreSQL initdb timed out on ephemeral CI.'
        }
        if($initProc.ExitCode -ne 0){
            $diagnostic=($initErr+[Environment]::NewLine+$initOut).
                Replace($pass,'[REDACTED]').
                Replace($appPassword,'[REDACTED]')
            if($diagnostic.Length -gt 2400){$diagnostic=$diagnostic.Substring(0,2400)}
            Write-Warning ('CI-only PostgreSQL initdb diagnostic, redacted: '+$diagnostic)
            throw ('PostgreSQL initdb failed on isolated Windows CI, exit '+$initProc.ExitCode)
        }
    }finally{$initProc.Dispose()}
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
    $expectedURLPattern='^postgres://esterium_app:[A-Za-z0-9_-]{48,}@localhost:'+([string]$port)+'/esterium\?sslmode=verify-full&sslrootcert='
    if ($url -notmatch $expectedURLPattern) {
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
    # The release schema is versioned by its bundled migration files, not by
    # a frozen count; this keeps fresh-install smoke in sync with new versions.
    $latestMigration=Get-ChildItem -LiteralPath $migrationFolder -Filter '*.up.sql' -File |
        Sort-Object Name | Select-Object -Last 1
    if ($null -eq $latestMigration -or $latestMigration.Name -notmatch '^(\d{4})_') {
        throw 'No valid numbered SQL migrations in Windows CI bundle.'
    }
    $expectedVersion=[int]$Matches[1]
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
    if ($version -cne "${expectedVersion}:false") {
        throw "Full initial schema migrations did not reach expected version $expectedVersion."
    }
    $replayAllowed=$false
    Write-Host 'PostgreSQL CI: invoking expected replay rejection.'
    try { & $schemaTool @schemaParams | Out-Null; $replayAllowed=$true }
    catch { if ($_.Exception.Message -notmatch 'nao esta vazio') { throw } }
    if ($replayAllowed) { throw 'Initial schema tool unexpectedly permitted migration replay.' }
    Write-Host 'PostgreSQL CI: testing confirmed transaction durability.'
    $null=Run-Psql 'CREATE TABLE ci_durable(id integer PRIMARY KEY, value text NOT NULL)'
    $null=Run-Psql "INSERT INTO ci_durable VALUES(17,'acknowledged-before-crash'),(42,'independent-restore-proof')"
    # Product images live inside PostgreSQL (bytea), not an external web/
    # folder. Prove the actual image and thumbnail byte payloads survive both
    # a forced PostgreSQL stop and a full restore into another Windows cluster.
    # The deliberately synthetic bytes never represent a customer photo.
    $null=Run-Psql @'
WITH company AS (
  INSERT INTO companies(legal_name, trade_name, cnpj, ie, crt)
  VALUES ('CI Recovery Photos LTDA', 'CI Recovery Photos', '98989898000198', 'ISENTO', '1')
  RETURNING id
), product AS (
  INSERT INTO products(tenant_id, sku, name, unit, cost_price, price_cash, min_stock, active)
  SELECT id, 'CI-RESTORE-PHOTO', 'Foto de teste descartavel', 'UN', 1.00, 3.00, 0, true
  FROM company RETURNING tenant_id, id
)
INSERT INTO product_images(tenant_id, product_id, upload_key, content_sha256,
                           image_data, thumb_data, caption)
SELECT tenant_id, id, gen_random_uuid(),
       decode('f46e8d0056321744073c78d41d11d020ffabd11d595b347e5ba24b363de48016','hex'),
       decode('ffd8ffe000104a46494600ffd9','hex'),
       decode('ffd8ffe000104a464946ffd9','hex'),
       'CI foto/miniatura apos restauracao'
FROM product;
'@
    if ((Run-Psql "SELECT count(*) FROM product_images WHERE caption='CI foto/miniatura apos restauracao'") -cne '1') {
        throw 'Disposable recovery source did not persist the product-image fixture.'
    }
    Write-Host 'PostgreSQL CI: initiating immediate shutdown for WAL recovery test.'
    Invoke-PgCtl -argList @('-D',$data,'-m','immediate','-w','stop') -phase 'immediate stop'
    $started=$false
    Write-Host 'PostgreSQL CI: starting recovery from immediate shutdown.'
    Invoke-PgCtl -argList @('-D',$data,'-l',$log,'-w','start') -phase 'crash recovery'
    $started=$true
    if ((Run-Psql 'SELECT count(*) FROM ci_durable WHERE id=17') -cne '1') {
        throw 'PostgreSQL did not recover acknowledged transaction after forced stop.'
    }
    # A real PostgreSQL 16 custom-format archive must be inspectable by the
    # Windows independent recovery preflight WITHOUT connecting to any DB.
    # The running cluster, data and backup live only on this disposable runner.
    $recoveryArchive = Join-Path $root ('esterium-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-aabbccdd.dump')
    $pgDump = Join-Path $bin 'pg_dump.exe'
    $pgRestore = Join-Path $bin 'pg_restore.exe'
    if (-not (Test-Path -LiteralPath $pgDump) -or -not (Test-Path -LiteralPath $pgRestore)) {
        throw 'Vendor PostgreSQL 16 package lacks pg_dump or pg_restore.'
    }
    & $pgDump '--format=custom' '--no-owner' '--no-privileges' ('--file='+$recoveryArchive)
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $recoveryArchive)) {
        throw 'Disposable Windows PostgreSQL recovery archive creation failed.'
    }
    $archiveDigest = (Get-FileHash -LiteralPath $recoveryArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText(($recoveryArchive+'.sha256'),
        ($archiveDigest+'  '+[IO.Path]::GetFileName($recoveryArchive)+[Environment]::NewLine),
        [Text.UTF8Encoding]::new($false))
    $pgRestoreDigest = (Get-FileHash -LiteralPath $pgRestore -Algorithm SHA256).Hash.ToLowerInvariant()
    $preflight = Join-Path $PSScriptRoot 'check-esterium-restore-archive.ps1'
    $env:ESTERIUM_RESTORE_DISPOSABLE_CI = '1'
    $verifyArgs = @{
        ArchivePath=$recoveryArchive
        ExpectedArchiveSHA256=$archiveDigest
        PgRestoreExe=$pgRestore
        ExpectedPgRestoreSHA256=$pgRestoreDigest
        SyntheticCIFixture=$true
    }
    $inspection = @(& $preflight @verifyArgs) -join ' '
    if ($inspection -notmatch 'PASS: independently approved PostgreSQL 16 archive') {
        throw 'Real Windows PostgreSQL restore preflight did not inspect the archive.'
    }
    # An archive with all CREATE TABLE statements but no TABLE DATA records
    # must never be mistaken for a full, recoverable database backup.
    $schemaOnlyArchive = Join-Path $root ('esterium-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-eeff0011.dump')
    & $pgDump '--format=custom' '--schema-only' '--no-owner' '--no-privileges' ('--file='+$schemaOnlyArchive)
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $schemaOnlyArchive)) {
        throw 'Cannot create synthetic schema-only negative recovery fixture.'
    }
    $schemaDigest = (Get-FileHash -LiteralPath $schemaOnlyArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText(($schemaOnlyArchive+'.sha256'),
        ($schemaDigest+'  '+[IO.Path]::GetFileName($schemaOnlyArchive)+[Environment]::NewLine),
        [Text.UTF8Encoding]::new($false))
    $schemaArgs = $verifyArgs.Clone()
    $schemaArgs.ArchivePath = $schemaOnlyArchive
    $schemaArgs.ExpectedArchiveSHA256 = $schemaDigest
    $schemaOnlyRejected = $false
    try { & $preflight @schemaArgs | Out-Null }
    catch { $schemaOnlyRejected = $_.Exception.Message -match 'schema-only archives are not complete backups' }
    if (-not $schemaOnlyRejected) { throw 'Recovery preflight accepted a schema-only archive without table data.' }
    $badTool = $verifyArgs.Clone()
    $badTool.ExpectedPgRestoreSHA256 = '0' * 64
    $badToolRejected = $false
    try { & $preflight @badTool | Out-Null }
    catch { $badToolRejected = $_.Exception.Message -match 'pg_restore executable differs' }
    if (-not $badToolRejected) { throw 'Restore preflight accepted a nonapproved pg_restore binary.' }
    $badArchive = $verifyArgs.Clone()
    $badArchive.ExpectedArchiveSHA256 = '0' * 64
    $badArchiveRejected = $false
    try { & $preflight @badArchive | Out-Null }
    catch { $badArchiveRejected = $_.Exception.Message -match 'checksum sidecar differs' }
    if (-not $badArchiveRejected) { throw 'Restore preflight accepted an unapproved archive hash.' }
    # Stronger than an archive-directory check: restore the ENTIRE archive
    # into a second, completely fresh PostgreSQL cluster on this disposable
    # Windows runner. No volume, data directory, roles or server process is
    # shared with the source. This is NOT a second physical PC.
    Write-Host 'PostgreSQL CI: restoring archive into a second isolated Windows cluster.'
    $sourceSql = @'
SELECT
    (SELECT version::text || ':' || dirty::text FROM schema_migrations)
    || '|' || (SELECT md5(string_agg(id::text || ':' || value, '|' ORDER BY id)) FROM ci_durable)
    || '|' || (SELECT count(*)::text FROM companies)
    || '|' || (SELECT count(*)::text FROM users)
    || '|' || (SELECT count(*)::text FROM products)
    || '|' || (SELECT count(*)::text FROM product_images)
    || '|' || (SELECT md5(string_agg(
          encode(image_data,'hex') || ':' || encode(thumb_data,'hex') || ':' || caption,
          '|' ORDER BY id)) FROM product_images)
    || '|' || (SELECT count(*)::text FROM sales)
    || '|' || (SELECT count(*)::text FROM audit_logs)
    || '|' || (SELECT count(*)::text FROM sale_fiscal_intents)
    || '|' || (SELECT count(*)::text FROM fiscal_document_sequences);
'@
    $sourceDigest = Run-Psql $sourceSql
    if ($sourceDigest -notmatch "^${expectedVersion}:false\|[a-f0-9]{32}\|") {
        throw 'Disposable source migration or sample content fingerprint is invalid.'
    }
    $portProbe=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
    try {
        $portProbe.Start()
        $secondPort=([Net.IPEndPoint]$portProbe.LocalEndpoint).Port
    } finally {
        $portProbe.Stop()
    }
    if ($secondPort -lt 1024 -or $secondPort -eq $port) {
        throw 'Cannot allocate a distinct loopback-only restore server port.'
    }
    # Fresh cluster: never use an existing database directory or drop a DB.
    if (Test-Path -LiteralPath $secondData) { throw 'Second restore cluster directory already exists.' }
    & $initdb '-D' $secondData '--username=postgres' ('--pwfile='+$passwordFile) '--auth-host=scram-sha-256' '--auth-local=scram-sha-256' '--encoding=UTF8' '--no-instructions' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Second disposable Windows PostgreSQL initdb failed.' }
    [IO.File]::AppendAllText((Join-Path $secondData 'postgresql.conf'),
        [Environment]::NewLine + "listen_addresses = '127.0.0.1'" + [Environment]::NewLine +
        "port = $secondPort" + [Environment]::NewLine)
    Invoke-PgCtl -argList @('-D',$secondData,'-l',$secondLog,'-w','start') -phase 'second recovery cluster startup'
    $secondStarted=$true
    # Keep the original source connection to verify it is not changed.
    $preRestoreKeys = @{}
    foreach ($key in @('PGHOST','PGPORT','PGUSER','PGDATABASE','PGPASSWORD','PGSSLMODE','PGSSLROOTCERT','PGHOSTADDR')) {
        $preRestoreKeys[$key]=[Environment]::GetEnvironmentVariable($key,'Process')
    }
    try {
        $env:PGHOST='127.0.0.1'
        $env:PGPORT=[string]$secondPort
        $env:PGUSER='postgres'
        $env:PGDATABASE='postgres'
        $env:PGPASSWORD=$pass
        $env:PGSSLMODE='disable' # localhost-only disposable cluster; NO production exception
        Remove-Item Env:PGSSLROOTCERT,Env:PGHOSTADDR -ErrorAction SilentlyContinue
        $isolation = @(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-c' 'SHOW listen_addresses;') -join ''
        if ($LASTEXITCODE -ne 0 -or $isolation.Trim() -cne '127.0.0.1') {
            throw 'Second disposable PostgreSQL cluster is not bound only to loopback.'
        }
        & $psql '-X' '-q' '-v' 'ON_ERROR_STOP=1' '-c' 'CREATE DATABASE esterium_recovery_ci;' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Cannot create a NEW database in the independent Windows cluster.' }
        & $pgRestore '--exit-on-error' '--no-owner' '--no-privileges' '--dbname=esterium_recovery_ci' $recoveryArchive | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Independent Windows PostgreSQL pg_restore failed.' }
        $restored = @(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-d' 'esterium_recovery_ci' '-c' $sourceSql) -join ''
        if ($LASTEXITCODE -ne 0 -or $restored.Trim() -cne $sourceDigest) {
            throw 'Independent Windows restore differs in migration, sample payload or critical business/fiscal counts.'
        }
        $tablesSql = "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('companies','users','products','product_images','sales','sale_items','payments','audit_logs','nfce_configs','fiscal_document_sequences','sale_fiscal_intents','schema_migrations');"
        $tables = @(& $psql '-X' '-q' '-t' '-A' '-v' 'ON_ERROR_STOP=1' '-d' 'esterium_recovery_ci' '-c' $tablesSql) -join ''
        if ($LASTEXITCODE -ne 0 -or $tables.Trim() -cne '12') {
            throw 'Independent Windows recovery is missing critical ERP tables.'
        }
    } finally {
        foreach ($entry in $preRestoreKeys.GetEnumerator()) {
            [Environment]::SetEnvironmentVariable($entry.Key,$entry.Value,'Process')
        }
    }
    if ((Run-Psql $sourceSql) -cne $sourceDigest) {
        throw 'Independent Windows restore unexpectedly changed the original PostgreSQL source.'
    }
    Write-Output 'PASS: full archive restored into independent Windows PostgreSQL 16 cluster; migration, content and critical tables match.'

    [IO.File]::AppendAllText($recoveryArchive, 'tampered-on-disposable-CI')
    $changedArchiveRejected = $false
    try { & $preflight @verifyArgs | Out-Null }
    catch { $changedArchiveRejected = $_.Exception.Message -match 'archive bytes disagree' }
    if (-not $changedArchiveRejected) { throw 'Restore preflight accepted tampered backup content.' }
    Write-Output 'PASS: Windows archive preflight inspected real PostgreSQL backup and rejected bad archive or binary hashes.'

    Write-Output "PASS: PostgreSQL 16 TLS verify-full, SCRAM, nonprivileged ERP role, protected URL, initial schema version $expectedVersion and crash recovery."
} finally {
    if ($secondStarted -and $pgCtl -and (Test-Path -LiteralPath $pgCtl)) {
        try { Invoke-PgCtl -argList @('-D',$secondData,'-m','immediate','-w','stop') -phase 'second cluster cleanup' }
        catch { Write-Warning 'Disposable recovery cluster needs runner teardown.' }
    }
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
