#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Synthetic Garnet config tests require disposable hosted Windows CI.'
}
$id = [Guid]::NewGuid().ToString('N')
$root = Join-Path $env:RUNNER_TEMP "esterium-garnet-config-$id"
$target = Join-Path $env:ProgramData "Esterium\GarnetConfigCI-$id"
$aclFile = Join-Path $root 'redis-acl.txt'
$pfxFile = Join-Path $root 'server.pfx'
$passwordFile = Join-Path $root 'pfx-password.secret'
$checker = Join-Path $PSScriptRoot 'provision-esterium-garnet-config.ps1'
$thumbprint = ''
$fixturePassword = 'synthetic-pfx-fixture-only'
try {
    [IO.Directory]::CreateDirectory($root) | Out-Null
    [IO.File]::WriteAllText($aclFile, ('user default off' + [Environment]::NewLine + 'user synthetic on >synthetic-only ~* +@all'))
    [IO.File]::WriteAllText($passwordFile, $fixturePassword)
    $certificate = New-SelfSignedCertificate -DnsName 'localhost' -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddDays(2) -ErrorAction Stop
    $thumbprint = $certificate.Thumbprint
    $secure = ConvertTo-SecureString $fixturePassword -AsPlainText -Force
    Export-PfxCertificate -Cert $certificate -FilePath $pfxFile -Password $secure | Out-Null
    foreach ($file in @($aclFile,$passwordFile,$pfxFile)) {
        $aclResult = & icacls.exe $file '/inheritance:r' '/grant:r' '*S-1-5-20:R' '*S-1-5-18:F' '*S-1-5-32-544:F' 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Failed to secure synthetic Garnet input: $aclResult" }
    }
    $parameters = @{
        AclFile=$aclFile
        CertificatePfxFile=$pfxFile
        CertificatePasswordFile=$passwordFile
        DestinationDir=$target
        Port=63913
    }
    $plan = @(& $checker @parameters) -join ' '
    if ($plan -notmatch 'PLAN: new protected Garnet JSON' -or (Test-Path -LiteralPath $target)) {
        throw 'Garnet dry run modified Windows.'
    }
    function Assert-Reject([string]$label, [string]$match) {
        $actual = '(accepted)'
        try { & $checker @parameters | Out-Null }
        catch { $actual = $_.Exception.Message }
        if ($actual -notmatch $match) { throw "Garnet gate accepted $label or failed for another reason: $actual" }
    }
    $parameters.Port = 8080
    Assert-Reject 'app port collision' 'must not use the Esterium web port'
    $parameters.Port = 63913
    & icacls.exe $aclFile '/grant' '*S-1-5-32-545:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to weaken fixture ACL.' }
    Assert-Reject 'generic Users reader' 'may not be read by other Windows accounts'
    & icacls.exe $aclFile '/remove' '*S-1-5-32-545' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to restore fixture ACL.' }
    & icacls.exe $aclFile '/grant:r' '*S-1-5-20:M' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to grant synthetic NETWORK SERVICE write.' }
    Assert-Reject 'NETWORK SERVICE write' 'must not be able to change'
    & icacls.exe $aclFile '/grant:r' '*S-1-5-20:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Failed to restore NETWORK SERVICE read-only.' }
    $result = @(& $checker @parameters -Apply) -join ' '
    if ($result -notmatch 'PASS: new protected Garnet configuration') {
        throw 'Synthetic Garnet config provisioning failed.'
    }
    $configPath = Join-Path $target 'garnet.conf'
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    if ($config.Address -ne '127.0.0.1' -or $config.AuthenticationMode -ne 'ACL' -or
        -not $config.EnableTLS -or -not $config.EnableLua -or
        -not $config.LuaTransactionMode -or $config.Port -ne 63913 -or
        $config.CertPassword -ne $fixturePassword) {
        throw 'Generated Garnet config has unsafe or incomplete settings.'
    }
    $perms = Get-Acl -LiteralPath $configPath
    $unexpected = @($perms.Access | Where-Object {
        $_.AccessControlType -eq 'Allow' -and
        $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -eq 'S-1-5-32-545'
    })
    if ($unexpected.Count -ne 0) { throw 'Config is readable by ordinary Users.' }
    Assert-Reject 'existing Garnet config destination' 'Target exists'
    Write-Output 'PASS: synthetic Garnet config plan, ACL protection, TLS/Lua and fail-closed validations.'
} finally {
    if ($thumbprint) {
        Remove-Item -LiteralPath ("Cert:\CurrentUser\My\" + $thumbprint) -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
