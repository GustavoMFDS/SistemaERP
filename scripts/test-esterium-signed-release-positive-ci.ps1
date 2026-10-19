#requires -Version 5.1
<# Purely synthetic code signing test: disposable GitHub-hosted Windows ONLY.
   Never uses a publisher certificate or distributes a signed production ZIP. #>
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Signing fixture requires disposable hosted Windows CI.'
}
$id=[Guid]::NewGuid().ToString('N')
$root=Join-Path $env:RUNNER_TEMP "esterium-signed-fixture-$id"
$bundle=Join-Path $root 'bundle'
$toolsDir=Join-Path $bundle 'tools'
$certThumb=''
try {
    [IO.Directory]::CreateDirectory($toolsDir) | Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $bundle 'web')) | Out-Null
    [IO.Directory]::CreateDirectory((Join-Path $bundle 'migrations')) | Out-Null
    [IO.File]::WriteAllText((Join-Path $bundle 'web\index.html'),'<html>synthetic signed fixture</html>')
    [IO.File]::WriteAllText((Join-Path $bundle 'README_LOCAL.md'),'Synthetic disposable CI-only release.')
    [IO.File]::WriteAllText((Join-Path $bundle 'migrations\0001_fixture.up.sql'),'-- synthetic only')
    [IO.File]::WriteAllText((Join-Path $bundle 'migrations\0001_fixture.down.sql'),'-- synthetic only')
    $csc=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    if (-not (Test-Path -LiteralPath $csc -PathType Leaf)) { throw 'Windows C# compiler missing for synthetic PE creation.' }
    $source=Join-Path $root 'fixture.cs'
    [IO.File]::WriteAllText($source,'public class Program { public static void Main() {} }')
    $server=Join-Path $bundle 'esterium-server.exe'
    & $csc '/nologo' '/target:exe' (('/out:' + $server)) $source | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not build synthetic Windows server fixture.' }
    Copy-Item -LiteralPath $server -Destination (Join-Path $toolsDir 'esterium-migrate.exe')
    # Match the manifest's exhaustive curator, but never copy package secrets.
    $required=@(
        'register-esterium-windows-service.ps1',
        'provision-esterium-app-secrets.ps1',
        'verify-esterium-bundle.ps1',
        'postgres-daily-backup.ps1',
        'copy-verified-external-backup.ps1',
        'check-esterium-backup-integrity.ps1',
        'run-esterium-scheduled-backup.ps1',
        'register-esterium-backup-task.ps1',
        'check-esterium-windows-host.ps1',
        'unregister-esterium-windows-service.ps1',
        'check-esterium-local-dependencies.ps1',
        'plan-esterium-windows-upgrade.ps1',
        'plan-esterium-local-setup.ps1',
        'install-esterium-local.ps1',
        'apply-esterium-app-update.ps1',
        'check-esterium-signed-release.ps1',
        'check-esterium-offline-dependency-packages.ps1',
        'provision-esterium-garnet-config.ps1',
        'provision-esterium-postgres-optionfile.ps1',
        'prepare-esterium-local-dependency-configs.ps1',
        'check-esterium-postgres-tls-policy.ps1'
    )
    foreach ($tool in $required) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $tool) -Destination (Join-Path $toolsDir $tool)
    }
    Write-Output 'STEP: generating disposable Windows code-signing certificate.'
    # CertificateRequest avoids depending on New-SelfSignedCertificate's
    # Windows key-provider enrollment path, which stalled hosted CI for >15m.
    # This certificate is exclusively a synthetic runner-local fixture.
    $rsa=[Security.Cryptography.RSA]::Create(2048)
    try {
        $request=[Security.Cryptography.X509Certificates.CertificateRequest]::new(
            ("CN=Esterium Disposable Test " + $id),
            $rsa,
            [Security.Cryptography.HashAlgorithmName]::SHA256,
            [Security.Cryptography.RSASignaturePadding]::Pkcs1)
        $request.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new(
                [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature, $true))
        $uses=[Security.Cryptography.OidCollection]::new()
        [void]$uses.Add([Security.Cryptography.Oid]::new('1.3.6.1.5.5.7.3.3'))
        $request.CertificateExtensions.Add(
            [Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]::new($uses,$false))
        $temporaryCert=$request.CreateSelfSigned(
            [DateTimeOffset]::UtcNow.AddMinutes(-5),[DateTimeOffset]::UtcNow.AddHours(3))
        $password=[Guid]::NewGuid().ToString('N')
        $pfx=Join-Path $root 'ephemeral-disposable-code-signing.pfx'
        [IO.File]::WriteAllBytes($pfx,$temporaryCert.Export(
            [Security.Cryptography.X509Certificates.X509ContentType]::Pfx,$password))
        $cert=Import-PfxCertificate -FilePath $pfx -CertStoreLocation 'Cert:\CurrentUser\My' `
            -Password (ConvertTo-SecureString $password -AsPlainText -Force) `
            -ErrorAction Stop
        Remove-Item -LiteralPath $pfx -Force
        $temporaryCert.Dispose()
    } finally {
        $rsa.Dispose()
    }
    if ($null -eq $cert -or -not $cert.HasPrivateKey) {
        throw 'Synthetic signing certificate missing the private signing key.'
    }
    $certThumb=$cert.Thumbprint
    Write-Output 'STEP: ephemeral certificate minted and imported; preparing local-only trust.'
    $cer=Join-Path $root 'synthetic-only.cer'
    Export-Certificate -Cert $cert -FilePath $cer | Out-Null
    Import-Certificate -FilePath $cer -CertStoreLocation 'Cert:\CurrentUser\Root' | Out-Null
    Import-Certificate -FilePath $cer -CertStoreLocation 'Cert:\CurrentUser\TrustedPublisher' | Out-Null
    Write-Output 'STEP: disposable trust stores prepared; signing fixtures.'
    Write-Output 'STEP: signing disposable executables and all packaged scripts.'
    $signTargets=@($server,(Join-Path $toolsDir 'esterium-migrate.exe'))
    $signTargets+=@(Get-ChildItem -LiteralPath $toolsDir -File -Filter '*.ps1' | ForEach-Object { $_.FullName })
    foreach ($path in $signTargets) {
        $result=Set-AuthenticodeSignature -FilePath $path -Certificate $cert -HashAlgorithm SHA256 -ErrorAction Stop
        if ($result.Status -ne [Management.Automation.SignatureStatus]::Valid) {
            throw 'Synthetic code signing did not produce a trusted local fixture.'
        }
    }
    Write-Output 'STEP: verifying full signed package and publisher identity.'
    $writer=Join-Path $PSScriptRoot 'write-esterium-bundle-manifest.ps1'
    & $writer -BundleDir $bundle | Out-Null
    $hash=(Get-FileHash -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Algorithm SHA256).Hash
    $gate=Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
    $checks=@{
        BundleDir=$bundle
        ExpectedManifestSHA256=$hash
        ExpectedPublisherThumbprint=$certThumb
    }
    $passed=@(& $gate @checks) -join ' '
    if ($passed -notmatch 'PASS: approved manifest hash and code-signing identity') {
        throw 'Approved synthetic signed bundle was rejected.'
    }
    $checks.ExpectedPublisherThumbprint='0' * 40
    $wrongSigner=$false
    try { & $gate @checks | Out-Null }
    catch { $wrongSigner=$_.Exception.Message -match 'lacks approved Authenticode signature' }
    if (-not $wrongSigner) { throw 'Signing gate accepted an unapproved publisher identity.' }
    $checks.ExpectedPublisherThumbprint=$certThumb
    [IO.File]::AppendAllText((Join-Path $toolsDir 'install-esterium-local.ps1'),'# synthetic tamper')
    Remove-Item -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Force
    & $writer -BundleDir $bundle | Out-Null
    $checks.ExpectedManifestSHA256=(Get-FileHash -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Algorithm SHA256).Hash
    $unsignedScript=$false
    try { & $gate @checks | Out-Null }
    catch { $unsignedScript=$_.Exception.Message -match 'lacks approved Authenticode signature' }
    if (-not $unsignedScript) { throw 'Signing gate accepted a tampered but rehashed privileged script.' }
    Write-Output 'PASS: ephemeral code-signed Windows bundle approved; wrong signer and rehashed tampering rejected.'
} finally {
    if ($certThumb) {
        foreach ($store in @('My','Root','TrustedPublisher')) {
            Remove-Item -LiteralPath ("Cert:\CurrentUser\" + $store + "\" + $certThumb) -Force -ErrorAction SilentlyContinue
        }
    }
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
