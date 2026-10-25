#requires -Version 5.1
# Synthetic bootstrap only: creates credentials/cert on ephemeral Windows runner.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Credential bootstrap fixtures only run on disposable Windows CI.'
}
$tool=Join-Path $PSScriptRoot 'provision-esterium-dependency-bootstrap.ps1'
$parseTokens=$null
$parseErrors=$null
[Management.Automation.Language.Parser]::ParseFile($tool,[ref]$parseTokens,[ref]$parseErrors) | Out-Null
if (@($parseErrors).Count -ne 0) {
    throw ('Bootstrap script does not parse: '+(@($parseErrors) | ForEach-Object { $_.Message } | Select-Object -First 1))
}
$id=[Guid]::NewGuid().ToString('N')
$target=Join-Path $env:ProgramData ('Esterium\BootstrapCI-'+$id)
$outside=Join-Path $env:RUNNER_TEMP ('BootstrapCI-'+$id)
$env:ESTERIUM_BOOTSTRAP_DISPOSABLE_CI='1'
try {
    $args=@{ DestinationDir=$target }
    $plan=@(& $tool @args) -join ' '
    if ($plan -notmatch 'PLAN: gerar senhas PostgreSQL' -or (Test-Path $target)) {
        throw 'Read-only credential plan wrote files.'
    }
    $err=''
    try { & $tool @args -Apply | Out-Null } catch { $err=$_.Exception.Message }
    if ($err -notmatch 'editor aprovado') { throw 'Real unsigned bootstrap unexpectedly allowed.' }
    $args.DestinationDir=$outside
    $err=''
    try { & $tool @args | Out-Null } catch { $err=$_.Exception.Message }
    if ($err -notmatch 'exclusivamente') { throw 'Bootstrap outside ProgramData was not rejected.' }
    $args.DestinationDir=$target
    $args.UnsignedDisposableCIFixture=$true
    $args.Apply=$true
    $result=@(& $tool @args) -join ' '
    if ($result -notmatch 'PASS: credenciais PostgreSQL e Garnet, incluindo URL TLS do ERP, geradas') {
        throw 'Synthetic bootstrap generation did not finish.'
    }
    foreach ($name in @('pg-super.secret','pg-service.secret','pg-app.secret','garnet.acl','garnet-pfx.secret','garnet.pfx','garnet-server.cer','redis-url.secret')) {
        if (-not (Test-Path -LiteralPath (Join-Path $target $name) -PathType Leaf)) {
            throw 'Synthetic bootstrap omitted a protected file.'
        }
    }
    $pgSuper=[IO.File]::ReadAllText((Join-Path $target 'pg-super.secret')).Trim()
    $pgService=[IO.File]::ReadAllText((Join-Path $target 'pg-service.secret')).Trim()
    $pgApp=[IO.File]::ReadAllText((Join-Path $target 'pg-app.secret')).Trim()
    if ($pgSuper -ceq $pgService -or $pgSuper -ceq $pgApp -or
        $pgService -ceq $pgApp -or $pgSuper.Length -lt 48 -or
        $pgService.Length -lt 48 -or $pgApp.Length -lt 48) {
        throw 'Database secret generation did not produce independent strong tokens.'
    }
    foreach ($password in @($pgSuper,$pgService,$pgApp)) {
        if ($password -cnotmatch '^[A-Za-z0-9_-]{48,128}$' -or
            $password -cnotmatch '[A-Z]' -or $password -cnotmatch '[a-z]' -or
            $password -cnotmatch '[0-9]' -or $password -cnotmatch '[_-]') {
            throw 'PostgreSQL passwords must satisfy Windows service account complexity reliably.'
        }
    }
    $acl=[IO.File]::ReadAllText((Join-Path $target 'garnet.acl'))
    if ($acl -notmatch '(?m)^user default off\r?$' -or $acl -match 'nopass' -or
        $acl -notmatch '(?m)^user esteriumcache on >[a-zA-Z0-9_-]{48,}') {
        throw 'Garnet ACL credentials are missing or unsafe.'
    }
    foreach ($name in @('pg-super.secret','pg-service.secret','pg-app.secret','garnet.acl','garnet-pfx.secret','garnet.pfx','garnet-server.cer','redis-url.secret')) {
        $allowNetwork=$name -notlike 'pg-*' -and $name -notin @('redis-url.secret','garnet-server.cer')
        $allowLocal=$name -in @('redis-url.secret','garnet-server.cer')
        $hasNetwork=$false
        $hasLocal=$false
        foreach ($ace in (Get-Acl -LiteralPath (Join-Path $target $name)).Access) {
            if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
            $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
            if (($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0) {
                if ($sid -notin @('S-1-5-18','S-1-5-32-544','S-1-5-20','S-1-5-19')) {
                    throw 'Bootstrap file allows unrelated readers.'
                }
                if ($sid -eq 'S-1-5-20') { $hasNetwork=$true }
                if ($sid -eq 'S-1-5-19') { $hasLocal=$true }
            }
        }
        if ($hasNetwork -ne $allowNetwork -or $hasLocal -ne $allowLocal) { throw 'Bootstrap service identity ACL differs from policy.' }
    }
    $url=[IO.File]::ReadAllText((Join-Path $target 'redis-url.secret')).Trim()
    if ($url -notmatch '^rediss://esteriumcache:[a-zA-Z0-9_-]{48,}@localhost:6379/0$') {
        throw 'Garnet client URL must require localhost TLS and a strong independent password.'
    }
    $secret=ConvertTo-SecureString ([IO.File]::ReadAllText((Join-Path $target 'garnet-pfx.secret')).Trim()) -AsPlainText -Force
    $cert=Get-PfxCertificate -FilePath (Join-Path $target 'garnet.pfx') -Password $secret
    $public=[Security.Cryptography.X509Certificates.X509Certificate2]::new((Join-Path $target 'garnet-server.cer'))
    if ($null -eq $public -or $public.Thumbprint -cne $cert.Thumbprint -or $public.HasPrivateKey) {
        throw 'Garnet public trust certificate does not match its TLS service identity.'
    }
    if ($null -eq $cert -or $cert.NotAfter -le (Get-Date).AddMonths(11)) {
        throw 'Short-lived or invalid synthetic TLS PFX.'
    }
    if (Test-Path -LiteralPath ('Cert:\LocalMachine\Root\'+$cert.Thumbprint)) {
        throw 'Bootstrap incorrectly trusted its own TLS certificate.'
    }
    $err=''
    try { & $tool @args | Out-Null } catch { $err=$_.Exception.Message }
    if ($err -notmatch 'nunca são sobrescritas') {
        throw 'Bootstrap unexpectedly modified existing credentials.'
    }
    Write-Output 'PASS: bootstrap plan, crypto-random independent passwords, Garnet ACL, PFX and restrictive Windows ACL verified.'
} finally {
    Remove-Item Env:ESTERIUM_BOOTSTRAP_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
}
