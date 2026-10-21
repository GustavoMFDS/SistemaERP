#requires -Version 5.1
<#
  Generate fresh, independent PostgreSQL bootstrap passwords and Garnet
  ACL/TLS inputs on one disposable Windows host. Never exposes secrets in
  args, logs, CI artifacts or the distribution bundle. This does NOT trust
  the self-signed TLS certificate or enable a production service.
#>
[CmdletBinding()]
param(
    [string]$DestinationDir=(Join-Path $env:ProgramData 'Esterium\DependencyBootstrap'),
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$UnsignedDisposableCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'A preparação de credenciais exige Windows.' }
if ($UnsignedDisposableCIFixture -and (
    $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_BOOTSTRAP_DISPOSABLE_CI -ne '1')) {
    throw 'Teste sem assinatura somente em Windows CI descartável.'
}
if ($Apply -and -not $UnsignedDisposableCIFixture) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Criar segredos reais exige editor aprovado de fonte independente.'
    }
    $signature=Get-AuthenticodeSignature -LiteralPath $PSCommandPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
           $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Criador de segredos não está assinado pelo editor aprovado.'
    }
}
if (-not [IO.Path]::IsPathRooted($DestinationDir) -or $DestinationDir.StartsWith('\\')) {
    throw 'Credenciais devem estar em disco local sob ProgramData\Esterium.'
}
$root=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$target=[IO.Path]::GetFullPath($DestinationDir).TrimEnd('\')
if (-not $target.StartsWith($root + '\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Pasta de credenciais deve estar exclusivamente sob ProgramData\Esterium.'
}
if (Test-Path -LiteralPath $target) {
    throw 'Credenciais existentes nunca são sobrescritas ou rotacionadas automaticamente.'
}
$parent=Split-Path -Parent $target
$cursor=$parent
while ($cursor) {
    if (Test-Path -LiteralPath $cursor) {
        if (((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Pasta de segredos não pode usar link ou junction.'
        }
    }
    $next=Split-Path -Parent $cursor
    if ($next -eq $cursor) { break }
    $cursor=$next
}
if (-not $Apply) {
    Write-Output 'PLAN: gerar senhas PostgreSQL independentes incluindo a conta SQL do ERP, uma URL rediss para a conta local do ERP e um certificado localhost TLS.'
    Write-Output 'PLAN: segredos protegidos, sem mostrar valores nem instalar confiança no certificado.'
    Write-Output 'PLAN: nenhuma escrita, serviço, conexão ou banco alterado.'
    return
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Criar credenciais locais requer Administrador.'
}
foreach ($cmd in @('New-SelfSignedCertificate','Export-PfxCertificate','Export-Certificate')) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        throw 'O Windows não possui a ferramenta de certificado necessária.'
    }
}
$icacls=Join-Path $env:SystemRoot 'System32\icacls.exe'
function Protect-Item([string]$path,[bool]$forNetworkService,[bool]$forLocalService=$false) {
    $rules=@('/inheritance:r','/grant:r','*S-1-5-18:F','*S-1-5-32-544:F')
    if ($forNetworkService) { $rules+= '*S-1-5-20:R' }
    if ($forLocalService) { $rules+= '*S-1-5-19:R' }
    & $icacls $path @rules | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Falha ao restringir ACL de segredos.' }
}
function New-Secret {
    $bytes=New-Object byte[] 48
    $rng=[Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) }
    finally { $rng.Dispose() }
    try { return [Convert]::ToBase64String($bytes).Replace('+','-').Replace('/','_').TrimEnd('=') }
    finally { [Array]::Clear($bytes,0,$bytes.Length) }
}
function Write-Secret([string]$path,[string]$value,[bool]$networkRead,[bool]$localRead=$false) {
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes($value+[Environment]::NewLine)
    try {
        $stream=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $stream.Write($bytes,0,$bytes.Length); $stream.Flush($true) }
        finally { $stream.Dispose() }
    } finally { [Array]::Clear($bytes,0,$bytes.Length) }
    Protect-Item $path $networkRead $localRead
}
[IO.Directory]::CreateDirectory($parent) | Out-Null
$stage=Join-Path $parent ('.esterium-bootstrap-'+[Guid]::NewGuid().ToString('N'))
$cert=$null
$certStorePath=''
$published=$false
$pgSuper=$null
$pgService=$null
$aclSecret=$null
$pfxSecret=$null
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    & $icacls $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:RX' '*S-1-5-19:RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Falha ao proteger pasta temporária de bootstrap.' }
    $pgSuper=New-Secret
    $pgService=New-Secret
    $pgApp=New-Secret
    $aclSecret=New-Secret
    $pfxSecret=New-Secret
    Write-Secret (Join-Path $stage 'pg-super.secret') $pgSuper $false
    Write-Secret (Join-Path $stage 'pg-service.secret') $pgService $false
    Write-Secret (Join-Path $stage 'pg-app.secret') $pgApp $false
    $aclText=('user default off' + [Environment]::NewLine +
        'user esteriumcache on >'+$aclSecret+' ~* +@all')
    Write-Secret (Join-Path $stage 'garnet.acl') $aclText $true
    Write-Secret (Join-Path $stage 'garnet-pfx.secret') $pfxSecret $true
    Write-Secret (Join-Path $stage 'redis-url.secret') ('rediss://esteriumcache:'+$aclSecret+'@localhost:'+$GarnetPort+'/0') $false $true
    # Only local loopback names. Do NOT place this cert into Trusted Root.
    $cert=New-SelfSignedCertificate -Type SSLServerAuthentication -Subject 'CN=localhost' -TextExtension @('2.5.29.17={text}DNS=localhost&IPAddress=127.0.0.1&IPAddress=::1') -KeyAlgorithm RSA -KeyLength 3072 -HashAlgorithm SHA256 -KeyExportPolicy Exportable -CertStoreLocation 'Cert:\LocalMachine\My' -NotAfter (Get-Date).AddMonths(12) -ErrorAction Stop
    if ($null -eq $cert -or -not $cert.HasPrivateKey) {
        throw 'Windows não conseguiu gerar um certificado TLS exportável.'
    }
    $certStorePath='Cert:\LocalMachine\My\'+$cert.Thumbprint
    $pfxPath=Join-Path $stage 'garnet.pfx'
    $pfxPassword=ConvertTo-SecureString $pfxSecret -AsPlainText -Force
    Export-PfxCertificate -Cert $cert -FilePath $pfxPath -Password $pfxPassword -ErrorAction Stop | Out-Null
    Protect-Item $pfxPath $true
    # Export only the public certificate for the LocalService ERP client.
    # Never add the self-signed leaf to global Windows trusted roots.
    $publicPath=Join-Path $stage 'garnet-server.cer'
    Export-Certificate -Cert $cert -FilePath $publicPath -Type CERT -ErrorAction Stop | Out-Null
    Protect-Item $publicPath $false $true
    if (-not (Test-Path -LiteralPath $publicPath -PathType Leaf) -or
        (Get-Item -LiteralPath $publicPath).Length -lt 128) {
        throw 'Certificado publico TLS do Garnet nao foi exportado.'
    }
    if (-not (Test-Path -LiteralPath $pfxPath -PathType Leaf)) {
        throw 'Arquivo PFX não foi criado.'
    }
    if (Test-Path -LiteralPath $target) { throw 'Outro processo criou as credenciais; sem sobrescrever.' }
    Move-Item -LiteralPath $stage -Destination $target -ErrorAction Stop
    $published=$true
    Write-Output 'PASS: credenciais PostgreSQL e Garnet, incluindo URL TLS do ERP, geradas numa pasta nova com ACL restrita.'
    Write-Output 'NOT READY: certificado autofirmado não é confiável para o cliente; configurar confiança TLS corretamente antes de iniciar ERP.'
    Write-Output 'NEXT: gerar configurações protegidas com prepare-esterium-local-dependency-configs.ps1.'
} finally {
    $pgSuper=$null; $pgService=$null; $pgApp=$null; $aclSecret=$null; $pfxSecret=$null
    if ($certStorePath -and (Test-Path -LiteralPath $certStorePath)) {
        Remove-Item -LiteralPath $certStorePath -Force -ErrorAction SilentlyContinue
    }
    if (-not $published -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    }
}
