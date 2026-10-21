#requires -Version 5.1
# Test never changes a PostgreSQL cluster. Synthetic only.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'PostgreSQL TLS generator test requires disposable Windows CI.'
}
$root=Join-Path $env:ProgramData ('Esterium\PgTlsPolicyCI-'+[Guid]::NewGuid().ToString('N'))
$outside=Join-Path $env:RUNNER_TEMP ('PgTlsPolicyCI-'+[Guid]::NewGuid().ToString('N'))
$tool=Join-Path $PSScriptRoot 'provision-esterium-postgres-tls-policy.ps1'
$env:ESTERIUM_PG_POLICY_DISPOSABLE_CI='1'
try {
    $args=@{DestinationDir=$root;PostgresPort=64333}
    $plan=@(& $tool @args) -join ' '
    if ($plan -notmatch 'PLAN: new immutable PostgreSQL' -or (Test-Path $root)) {
        throw 'PostgreSQL policy read-only plan changed host.'
    }
    $args.DestinationDir=$outside
    $denied=''
    try { & $tool @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'ProgramData') { throw 'Policy accepted untrusted destination.' }
    $args.DestinationDir=$root
    $args.PostgresPort=8080
    $denied=''
    try { & $tool @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'Reserved') { throw 'Policy accepted ERP web port.' }
    $args.PostgresPort=64333
    $denied=''
    try { & $tool @args -Apply | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'externally approved publisher') {
        throw 'Unsigned policy generator incorrectly allowed production Apply.'
    }
    $args.Apply=$true
    $args.UnsignedDisposableCIFixture=$true
    $result=@(& $tool @args) -join ' '
    if ($result -notmatch 'PASS: new PostgreSQL TLS/SCRAM policy, private server key') {
        throw 'Synthetic protected PostgreSQL policy provisioning failed.'
    }
    $pg=Join-Path $root 'postgresql.conf'
    $hba=Join-Path $root 'pg_hba.conf'
    $certFolder=Join-Path $root 'certificates'
    foreach ($name in @('server.crt','server.key','root.crt')) {
        if (-not (Test-Path -LiteralPath (Join-Path $certFolder $name) -PathType Leaf)) {
            throw 'Offline certificate generator omitted a PostgreSQL TLS file.'
        }
        $readSids=@((Get-Acl -LiteralPath (Join-Path $certFolder $name)).Access | Where-Object {
            $_.AccessControlType -eq 'Allow' -and
            ($_.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0
        } | ForEach-Object { $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value })
        $localCanRead='S-1-5-19' -in $readSids
        if (($name -eq 'root.crt') -ne $localCanRead) {
            throw 'LocalService access to PostgreSQL TLS root/private key does not follow isolation policy.'
        }
    }

    if (-not (Test-Path -LiteralPath $pg -PathType Leaf) -or
        -not (Test-Path -LiteralPath $hba -PathType Leaf)) {
        throw 'Missing newly created PostgreSQL policy templates.'
    }
    $text=[IO.File]::ReadAllText($pg)
    if ($text -notmatch "(?m)^port = 64333\r?$" -or $text -notmatch "ssl = on" -or
        $text -notmatch [regex]::Escape('certificates/server.crt') -or
        $text -notmatch "password_encryption = 'scram-sha-256'") {
        throw 'PostgreSQL policy contains unexpected listen/transport/auth settings.'
    }
    & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $pg -PgHbaConf $hba | Out-Null
    $denied=''
    try { & $tool @args | Out-Null } catch { $denied=$_.Exception.Message }
    if ($denied -notmatch 'must not be overwritten') {
        throw 'Policy generator overwrote an existing trusted destination.'
    }
    # NEW cluster activation: only a synthetic disabled/stopped service fixture.
    $newData=Join-Path $env:RUNNER_TEMP ('EsteriumFreshPg-'+[Guid]::NewGuid().ToString('N'))
    $proofDir=Join-Path $env:RUNNER_TEMP ('EsteriumProofPg-'+[Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($newData) | Out-Null
    [IO.Directory]::CreateDirectory($proofDir) | Out-Null
    try {
        $marker=Join-Path $proofDir 'PostgresFresh-EsteriumPostgreSQL16.marker'
        [IO.File]::WriteAllText((Join-Path $newData 'PG_VERSION'),"16"+[Environment]::NewLine)
        $oldConf="# Vendor first-install config"+[Environment]::NewLine+"max_connections = 100"+[Environment]::NewLine
        $oldHba="# Vendor first-install HBA"+[Environment]::NewLine+"host all all 127.0.0.1/32 scram-sha-256"+[Environment]::NewLine
        [IO.File]::WriteAllText((Join-Path $newData 'postgresql.conf'),$oldConf)
        [IO.File]::WriteAllText((Join-Path $newData 'pg_hba.conf'),$oldHba)
        [IO.File]::WriteAllText($marker,
            'ESTERIUM_FRESH_POSTGRES_16'+[Environment]::NewLine+
            'EsteriumPostgreSQL16'+[Environment]::NewLine+[IO.Path]::GetFullPath($newData))
        $stage=Join-Path $PSScriptRoot 'stage-esterium-postgres-tls.ps1'
        $env:ESTERIUM_PG_ACTIVATE_DISPOSABLE_CI='1'
        $stageArgs=@{
            PostgresDataDir=$newData
            TlsPolicyDir=$root
            FreshClusterMarker=$marker
            PostgresPort=64333
            UnsignedDisposableCIFixture=$true
        }
        $plan=@(& $stage @stageArgs) -join ' '
        if ($plan -notmatch 'PLAN: validate new disabled/stopped PostgreSQL' -or
            [IO.File]::ReadAllText((Join-Path $newData 'postgresql.conf')) -cne $oldConf) {
            throw 'PostgreSQL TLS staging PLAN touched the new database.'
        }
        $unsafe=$stageArgs.Clone()
        $unsafe.PostgresPort=64334
        $denied=''
        try { & $stage @unsafe | Out-Null } catch { $denied=$_.Exception.Message }
        if ($denied -notmatch 'port') { throw 'Wrong PostgreSQL TLS port was accepted.' }
        $stageArgs.Apply=$true
        $stageArgs.Remove('UnsignedDisposableCIFixture')
        $denied=''
        try { & $stage @stageArgs | Out-Null } catch { $denied=$_.Exception.Message }
        if ($denied -notmatch 'Publisher certificate') {
            throw 'Unsigned PostgreSQL config activation unexpectedly allowed outside disposable mode.'
        }
        $stageArgs.UnsignedDisposableCIFixture=$true
        & $stage @stageArgs | Out-Null
        $liveConfig=Join-Path $newData 'postgresql.conf'
        $liveHba=Join-Path $newData 'pg_hba.conf'
        & (Join-Path $PSScriptRoot 'check-esterium-postgres-tls-policy.ps1') -PostgresqlConf $liveConfig -PgHbaConf $liveHba | Out-Null
        if ([IO.File]::ReadAllText((Join-Path $proofDir 'OriginalPostgresql-EsteriumPostgreSQL16.conf')) -cne $oldConf -or
            [IO.File]::ReadAllText((Join-Path $proofDir 'OriginalPgHba-EsteriumPostgreSQL16.conf')) -cne $oldHba -or
            -not (Test-Path -LiteralPath (Join-Path $proofDir 'PostgresTlsApplied-EsteriumPostgreSQL16.receipt'))) {
            throw 'PostgreSQL TLS config activation lost the original vendor defaults or completion receipt.'
        }
        $denied=''
        try { & $stage @stageArgs | Out-Null } catch { $denied=$_.Exception.Message }
        if ($denied -notmatch 'already staged or an interrupted first install') {
            throw 'A second PostgreSQL TLS activation replaced an existing config.'
        }
        Write-Output 'PASS: first-install TLS config staged atomically, original vendor config preserved, unsigned/port/replay refused.'
    } finally {
        Remove-Item Env:ESTERIUM_PG_ACTIVATE_DISPOSABLE_CI -ErrorAction SilentlyContinue
        foreach ($dir in @($newData,$proofDir)) {
            if (Test-Path -LiteralPath $dir) { Remove-Item -LiteralPath $dir -Force -Recurse }
        }
    }
    Write-Output 'PASS: local PostgreSQL TLS/SCRAM templates, newly minted root/server keys and ACL isolation deny overwrite.'
} finally {
    Remove-Item Env:ESTERIUM_PG_POLICY_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
