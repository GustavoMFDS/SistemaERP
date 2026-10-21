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
    Write-Output 'PASS: local PostgreSQL TLS/SCRAM templates, newly minted root/server keys and ACL isolation deny overwrite.'
} finally {
    Remove-Item Env:ESTERIUM_PG_POLICY_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
