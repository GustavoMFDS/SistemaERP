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
    Copy-Item -LiteralPath $policyConfig -Destination (Join-Path $data 'postgresql.conf') -Force
    Copy-Item -LiteralPath $policyHba -Destination (Join-Path $data 'pg_hba.conf') -Force
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
    Write-Output 'PASS: PostgreSQL 16 official Windows ZIP, strict TLS verify-full, SCRAM, HBA denials and crash recovery.'
} finally {
    if ($started -and $pgCtl -and (Test-Path -LiteralPath $pgCtl)) {
        try { Invoke-PgCtl -argList @('-D',$data,'-m','immediate','-w','stop') -phase 'cleanup' }
        catch { Write-Warning 'Disposable PostgreSQL server needs runner teardown.' }
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
