#requires -Version 5.1
<#
  Initialize a real PostgreSQL 16 test cluster as a new non-administrator
  Windows identity. GitHub-hosted disposable Windows CI ONLY, no service,
  network listener, customer PC or production credential.
#>
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$VerifiedPgRoot)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1') {
    throw 'PostgreSQL initdb may run ONLY on disposable GitHub-hosted CI.'
}
$temp=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\')+'\'
$root=[IO.Path]::GetFullPath($VerifiedPgRoot).TrimEnd('\')
if (-not $root.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase) -or
    -not (Test-Path -LiteralPath (Join-Path $root 'bin\initdb.exe') -PathType Leaf)) {
    throw 'Verified PostgreSQL binary directory must be under hosted CI temporary files.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Only CI Administrator can create temporary low-privilege test account.'
}
$account='epg'+[Guid]::NewGuid().ToString('N').Substring(0,12)
$work=Join-Path $env:RUNNER_TEMP ('esterium-pg-initdb-'+[Guid]::NewGuid().ToString('N'))
$data=Join-Path $work 'cluster'
$pwFile=Join-Path $work 'password.secret'
$stdout=Join-Path $work 'initdb.out'
$stderr=Join-Path $work 'initdb.err'
$created=$false
$proc=$null
$bytes=New-Object byte[] 48
$rng=[Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $rng.GetBytes($bytes)
    $password='Q9_z'+[Convert]::ToBase64String($bytes).Replace('+','-').Replace('/','_').TrimEnd('=')
} finally { $rng.Dispose();[Array]::Clear($bytes,0,$bytes.Length) }
$secure=ConvertTo-SecureString $password -AsPlainText -Force
try {
    $local=New-LocalUser -Name $account -Password $secure -Description 'Ephemeral PostgreSQL initdb test'
    if ($null -eq $local -or $null -eq $local.SID) {throw 'Failed to create CI low-privilege user.'}
    $created=$true
    $sid=[string]$local.SID.Value
    $administrators=@(Get-LocalGroupMember -Group 'Administrators' -ErrorAction Stop)
    if (@($administrators | Where-Object { $_.SID.Value -eq $sid }).Count -ne 0) {
        throw 'CI PostgreSQL user unexpectedly belongs to Administrators.'
    }
    [IO.Directory]::CreateDirectory($work) | Out-Null
    & icacls.exe $work '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' ('*'+$sid+':(OI)(CI)M') | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Cannot set ephemeral cluster directory ACL.'}
    & icacls.exe $root '/grant' ('*'+$sid+':(OI)(CI)RX') | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'CI account cannot read verified ZIP binaries.'}
    [IO.File]::WriteAllText($pwFile,$password+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    & icacls.exe $pwFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' ('*'+$sid+':R') | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Cannot protect ephemeral database secret.'}
    $password=$null
    $credential=[PSCredential]::new(($env:COMPUTERNAME+'\'+$account),$secure)
    $argLine='-D "'+$data+'" -U postgres --auth-local=scram-sha-256 --auth-host=scram-sha-256 --encoding=UTF8 --pwfile "'+$pwFile+'" --no-instructions'
    $run=@{FilePath=(Join-Path $root 'bin\initdb.exe');Credential=$credential;ArgumentList=$argLine;
        Wait=$true;PassThru=$true;RedirectStandardOutput=$stdout;RedirectStandardError=$stderr}
    $proc=Start-Process @run
    if ($null -eq $proc -or $proc.ExitCode -ne 0) {
        throw 'Non-admin initdb failed in disposable CI. No database server was started.'
    }
    if (-not(Test-Path -LiteralPath (Join-Path $data 'PG_VERSION') -PathType Leaf) -or
        ([IO.File]::ReadAllText((Join-Path $data 'PG_VERSION'))).Trim() -cne '16' -or
        -not(Test-Path -LiteralPath (Join-Path $data 'postgresql.conf') -PathType Leaf) -or
        -not(Test-Path -LiteralPath (Join-Path $data 'pg_hba.conf') -PathType Leaf)) {
        throw 'Unprivileged PostgreSQL 16 cluster is incomplete.'
    }
    $hba=[IO.File]::ReadAllText((Join-Path $data 'pg_hba.conf'))
    if ($hba -notmatch 'scram-sha-256') {throw 'New test cluster did not configure SCRAM.'}
    foreach ($service in @('EsteriumPostgreSQL16','EsteriumGarnet','EsteriumERP')) {
        if ($null -ne (Get-Service -Name $service -ErrorAction SilentlyContinue)) {
            throw 'Unprivileged initdb unexpectedly registered a service.'
        }
    }
    # Real Windows SCM step, still disabled/stopped. User credentials stay in
    # PSCredential SecureString and never appear in any process argument.
    & (Join-Path $PSScriptRoot 'test-postgres-oss-service-ci.ps1') -VerifiedPgRoot $root -DataDir $data -ServiceCredential $credential -ExerciseLoopback
    Write-Output 'PASS: PostgreSQL 16 initdb generated a real cluster as new NON-ADMIN CI Windows user.'
    Write-Output 'PASS: PG_VERSION=16 and pg_hba SCRAM, no Windows service started or registered.'
} finally {
    $password=$null
    $secure=$null
    if ($null -ne $proc) {$proc.Dispose()}
    if ($created) {Remove-LocalUser -Name $account -ErrorAction Stop}
}
