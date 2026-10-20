#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Synthetic PostgreSQL option file gate requires disposable hosted Windows CI.'
}
$id=[Guid]::NewGuid().ToString('N')
$short=$id.Substring(0,10)
$root=Join-Path $env:RUNNER_TEMP ("esterium-postgres-options-fixture-" + $id)
$target=Join-Path $env:ProgramData ("Esterium\PostgresOptionsCI-" + $short)
$data=Join-Path $env:ProgramData ("Esterium\PostgresDataCI-" + $short)
$binary=Join-Path $env:ProgramFiles ("PostgreSQL\16-ci-" + $short)
$adminSecret=Join-Path $root 'super.secret'
$serviceSecret=Join-Path $root 'service.secret'
$checker=Join-Path $PSScriptRoot 'provision-esterium-postgres-optionfile.ps1'
$adminPassword='CI2026SuperSecretOnly00112233'
$servicePassword='CI2026ServiceSecretOnly334455'
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($adminSecret,$adminPassword)
    [IO.File]::WriteAllText($serviceSecret,$servicePassword)
    foreach ($file in @($adminSecret,$serviceSecret)) {
        & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Could not secure synthetic PostgreSQL password file.' }
    }
    $p=@{
        SuperPasswordFile=$adminSecret
        ServicePasswordFile=$serviceSecret
        DestinationDir=$target
        PostgresInstallDir=$binary
        DataDir=$data
        ServiceName=('EsteriumPGCI-' + $short)
        ServiceAccount=('epg_ci' + $short)
        Port=64322
    }
    $plan=@(& $checker @p) -join ' '
    if ($plan -notmatch 'PLAN: generate an ACL-protected offline PostgreSQL 16' -or
        (Test-Path -LiteralPath $target)) {
        throw 'PostgreSQL option-file plan wrote data or failed.'
    }
    function Assert-Reject([string]$label,[string]$expected) {
        $actual='(accepted)'
        try { & $checker @p | Out-Null } catch { $actual=$_.Exception.Message }
        if ($actual -notmatch $expected) { throw "PostgreSQL options accepted $label or failed unexpectedly: $actual" }
    }
    $p.Port=8080
    Assert-Reject 'PostgreSQL on the ERP web port' 'may not take the Esterium web'
    $p.Port=6379
    Assert-Reject 'PostgreSQL on the Garnet cache port' 'may not take the Esterium web'
    $p.ServiceAccount='administrator'
    Assert-Reject 'Windows administrator service identity' 'Invalid or unsafe'
    $p.ServiceAccount=('epg_ci' + $short)
    $p.Port=64322
    & icacls.exe $adminSecret '/grant' '*S-1-5-32-545:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot weaken synthetic PostgreSQL password ACL.' }
    Assert-Reject 'shared credential reader' 'may be read only by SYSTEM and Administrators'
    & icacls.exe $adminSecret '/remove' '*S-1-5-32-545' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Cannot restore synthetic credential ACL.' }
    $p.ServicePasswordFile=$adminSecret
    Assert-Reject 'reusing one file for both accounts' 'distinct files'
    $p.ServicePasswordFile=$serviceSecret
    $unapprovedDenied=$false
    try { & $checker @p -Apply | Out-Null }
    catch { $unapprovedDenied=$_.Exception.Message -match 'independently approved publisher identity' }
    if (-not $unapprovedDenied) { throw 'Direct PostgreSQL option helper accepted unsigned real Apply.' }
    $env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI='1'
    $result=@(& $checker @p -Apply -UnsignedDisposableCIFixture) -join ' '
    if ($result -notmatch 'PASS: new protected PostgreSQL 16 unattended installer option file') {
        throw 'Synthetic PostgreSQL option file was not created.'
    }
    $out=Join-Path $target 'postgresql-unattended.options'
    if (-not (Test-Path -LiteralPath $out -PathType Leaf)) {
        throw 'PostgreSQL option file missing after Apply.'
    }
    $content=[IO.File]::ReadAllText($out)
    if (-not $content.Contains('mode=unattended') -or
        -not $content.Contains('enable-components=server,commandlinetools') -or
        -not $content.Contains('superpassword=' + $adminPassword) -or
        -not $content.Contains('servicepassword=' + $servicePassword) -or
        -not $content.Contains('datadir=' + $data)) {
        throw 'PostgreSQL unattended option file does not contain expected isolated settings.'
    }
    $acl=Get-Acl -LiteralPath $out
    foreach ($entry in $acl.Access) {
        if ($entry.AccessControlType -ne 'Allow') { continue }
        $sid=$entry.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if (($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0 -and
            $sid -notin @('S-1-5-18','S-1-5-32-544')) {
            throw 'Generated PostgreSQL installer option file can be read by an unsafe account.'
        }
    }
    Assert-Reject 'reusing a previously generated option file' 'Existing PostgreSQL'
    if (Test-Path -LiteralPath $data) { throw 'PostgreSQL setup unexpectedly initialized database data.' }
    Write-Output 'PASS: synthetic protected PostgreSQL 16 installer option file and fail-closed checks.'
} finally {
    Remove-Item Env:ESTERIUM_DEPENDENCY_CONFIG_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
