#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Unified dependency config test requires disposable hosted Windows CI.'
}
$id=[Guid]::NewGuid().ToString('N')
$short=$id.Substring(0,9)
$root=Join-Path $env:RUNNER_TEMP ('esterium-local-configs-' + $id)
$pgOutput=Join-Path $env:ProgramData ('Esterium\PGConfigCI-' + $short)
$garnetOutput=Join-Path $env:ProgramData ('Esterium\GarnetConfigCI-' + $short)
$pgData=Join-Path $env:ProgramData ('Esterium\PGDataCI-' + $short)
$pgBin=Join-Path $env:ProgramFiles ('PostgreSQL\16-prep-ci-' + $short)
$pgSuper=Join-Path $root 'pg-super.secret'
$pgService=Join-Path $root 'pg-service.secret'
$garnetAcl=Join-Path $root 'garnet.acl'
$pfx=Join-Path $root 'garnet.pfx'
$pfxPasswordFile=Join-Path $root 'pfx.secret'
$tool=Join-Path $PSScriptRoot 'prepare-esterium-local-dependency-configs.ps1'
$certThumbprint=''
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($pgSuper,'SyntheticPostgresSecret0000000001')
    [IO.File]::WriteAllText($pgService,'SyntheticServiceSecret0000000002')
    [IO.File]::WriteAllText($pfxPasswordFile,'synthetic-pfx-password-only')
    [IO.File]::WriteAllText($garnetAcl,('user default off' + [Environment]::NewLine + 'user synthetic on >synthetic-only ~* +@all'))
    $cert=New-SelfSignedCertificate -DnsName localhost -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddDays(2)
    $certThumbprint=$cert.Thumbprint
    $passwordSecure=ConvertTo-SecureString 'synthetic-pfx-password-only' -AsPlainText -Force
    Export-PfxCertificate -Cert $cert -FilePath $pfx -Password $passwordSecure | Out-Null
    foreach ($path in @($pgSuper,$pgService)) {
        & icacls.exe $path '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Cannot set fixture PostgreSQL secret ACL.' }
    }
    foreach ($path in @($garnetAcl,$pfx,$pfxPasswordFile)) {
        & icacls.exe $path '/inheritance:r' '/grant:r' '*S-1-5-20:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Cannot set fixture Garnet secret ACL.' }
    }
    $values=@{
        PostgresSuperPasswordFile=$pgSuper
        PostgresServicePasswordFile=$pgService
        GarnetAclFile=$garnetAcl
        GarnetCertificatePfxFile=$pfx
        GarnetCertificatePasswordFile=$pfxPasswordFile
        PostgresOptionsDir=$pgOutput
        GarnetConfigDir=$garnetOutput
        PostgresInstallDir=$pgBin
        PostgresDataDir=$pgData
        PostgresServiceName=('PGCI' + $short)
        PostgresServiceAccount=('pgci' + $short)
        PostgresPort=64323
        GarnetPort=64324
    }
    $result=@(& $tool @values) -join ' '
    if ($result -notmatch 'PLAN: both prerequisite configuration checks passed' -or
        (Test-Path -LiteralPath $pgOutput) -or (Test-Path -LiteralPath $garnetOutput)) {
        throw 'Unified plan unexpectedly wrote configuration files.'
    }
    function Assert-Reject([string]$label,[string]$expected) {
        $errorText='(accepted)'
        try { & $tool @values | Out-Null } catch { $errorText=$_.Exception.Message }
        if ($errorText -notmatch $expected) {
            throw "Unified dependency tool accepted $label or returned unexpected error: $errorText"
        }
    }
    $values.GarnetPort=64323
    Assert-Reject 'shared PostgreSQL/Garnet port' 'distinct local ports'
    $values.GarnetPort=64324
    $values.GarnetConfigDir=$pgOutput
    Assert-Reject 'shared config path' 'directories must be separate'
    $values.GarnetConfigDir=$garnetOutput
    $values.Apply=$true
    Assert-Reject 'real Apply without approved publisher' 'independently approved publisher thumbprint'
    $values.Remove('Apply')
    $env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI='1'
    $values.UnsignedDisposableCIFixture=$true
    $values.Apply=$true
    $applied=@(& $tool @values) -join ' '
    if ($applied -notmatch 'PASS: new PostgreSQL installer options and Garnet ACL/TLS/Lua') {
        throw 'Unified synthetic Apply failed.'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $pgOutput 'postgresql-unattended.options')) -or
        -not (Test-Path -LiteralPath (Join-Path $garnetOutput 'garnet.conf'))) {
        throw 'One or both protected configuration files are missing.'
    }
    if (Test-Path -LiteralPath $pgData) {
        throw 'Unified setup changed database data or installed PostgreSQL.'
    }
    $values.Remove('UnsignedDisposableCIFixture')
    $values.Remove('Apply')
    Assert-Reject 'reusing existing protected directories' 'Existing PostgreSQL'
    Write-Output 'PASS: combined synthetic dependency configuration is read-only by default and safely creates only new protected settings.'
} finally {
    Remove-Item Env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if ($certThumbprint) {
        Remove-Item -LiteralPath ('Cert:\CurrentUser\My\' + $certThumbprint) -Force -ErrorAction SilentlyContinue
    }
    foreach ($path in @($pgOutput,$garnetOutput,$root)) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Recurse -Force }
    }
}
