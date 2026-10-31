#requires -Version 5.1
<#
  Technical first-install entry point. One invocation plans or, for an
  independently approved signed release, runs the two fresh-install phases:
  vendor PostgreSQL/Garnet and the protected ERP schema/service readiness.
  This is not a customer-facing EXE or permission to start taking sales.
  No Go/Docker installation is required on the customer's computer.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$SourceBundleDir,
    [Parameter(Mandatory=$true)][string]$AllowedOrigin,
    [Parameter(Mandatory=$true)][string]$PostgresInstaller,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresPublisherThumbprint,
    [Parameter(Mandatory=$true)][string]$GarnetPackage,
    [Parameter(Mandatory=$true)][string]$ExpectedGarnetSHA256,
    [Parameter(Mandatory=$true)][string]$ServiceWrapperExe,
    [Parameter(Mandatory=$true)][string]$ExpectedServiceWrapperSHA256,
    [string]$ListenAddress='0.0.0.0:8443',
    [string]$TLSCertFile='',
    [string]$TLSKeyFile='',
    [string]$TrustedRemoteAddress='',
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [switch]$Apply,
    # Absolutely no executable vendor/ERP installation is allowed with this
    # test-only option. It verifies ONLY the real two-stage PLAN path in CI.
    [switch]$SyntheticPlanCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'A instalacao completa exige Windows.' }
if ($SyntheticPlanCIFixture -and ($Apply -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SETUP_PLAN_DISPOSABLE_CI -ne '1')) {
    throw 'Plano sintetico restrito a CI Windows descartavel, proibido em Apply.'
}
if ($PostgresPort -in @($GarnetPort,8080) -or $GarnetPort -eq 8080) {
    throw 'Portas da aplicacao, PostgreSQL e Garnet precisam ser distintas.'
}
$listenerParts=$ListenAddress -split ':',2
if ($listenerParts.Count -ne 2 -or $listenerParts[1] -notmatch '^\d{1,5}$' -or
    $listenerParts[0] -notin @('0.0.0.0','127.0.0.1') -or
    [int]$listenerParts[1] -in @($PostgresPort,$GarnetPort)) {
    throw 'ListenAddress deve ser loopback/LAN HTTPS em porta distinta das dependencias.'
}
$origin=$null
if (-not [Uri]::TryCreate($AllowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
    $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or
    [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or
    $origin.Fragment -ne '') {
    throw 'Instalacao exige origem HTTPS aprovada para o navegador da loja.'
}
$originPort=if ($origin.IsDefaultPort) { 443 } else { $origin.Port }
if ($originPort -ne [int]$listenerParts[1]) { throw 'AllowedOrigin port must match the ERP HTTPS listener.' }
$originIsLoopback=$origin.Host -in @('localhost','127.0.0.1','::1')
if ($originIsLoopback -and $listenerParts[0] -ne '127.0.0.1') {
    throw 'AllowedOrigin localhost so pode ser usado com ListenAddress loopback.'
}
if (-not [IO.Path]::IsPathRooted($SourceBundleDir) -or $SourceBundleDir.StartsWith('\\')) {
    throw 'O pacote aprovado precisa estar em disco local, nao em rede.'
}
$source=[IO.Path]::GetFullPath($SourceBundleDir).TrimEnd('\')
if (-not (Test-Path -LiteralPath $source -PathType Container)) {
    throw 'Pacote local aprovado nao encontrado.'
}
$setup=Join-Path $PSScriptRoot 'setup-esterium-local-dependencies.ps1'
$finish=Join-Path $PSScriptRoot 'finish-esterium-local-first-install.ps1'
$activation=Join-Path $PSScriptRoot 'activate-esterium-local-first-install.ps1'
foreach ($helper in @($setup,$finish,$activation)) {
    if (-not (Test-Path -LiteralPath $helper -PathType Leaf)) {
        throw 'Instalador incompleto: falta etapa obrigatoria.'
    }
}
if ($Apply) {
    if ($ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
        $TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Execucao real exige manifesto e editor aprovados independentemente.'
    }
    if (-not [string]::Equals(
        [IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\'),
        (Join-Path $source 'tools'),
        [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Execucao real somente a partir das ferramentas do pacote assinado.'
    }
    # Authorize this coordinator and both privileged delegates before PLAN
    # or execution. Each delegate will independently recheck its own helpers.
    foreach ($path in @($PSCommandPath,$setup,$finish,$activation)) {
        $signature=Get-AuthenticodeSignature -LiteralPath $path
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
              $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Instalador completo sem assinatura do editor aprovado.'
        }
    }
    if ([string]::IsNullOrWhiteSpace($TLSCertFile) -or [string]::IsNullOrWhiteSpace($TLSKeyFile)) {
        throw 'Apply exige certificado HTTPS confiavel e chave privada para o servico da loja.'
    }
    if ($listenerParts[0] -eq '0.0.0.0' -and [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)) {
        throw 'Acesso LAN exige um CIDR privado explicito para a regra de firewall.'
    }
    if ($listenerParts[0] -eq '127.0.0.1' -and -not [string]::IsNullOrWhiteSpace($TrustedRemoteAddress)) {
        throw 'Acesso loopback nao deve abrir uma regra de firewall LAN.'
    }
}
$setupArgs=@{
    PostgresInstaller=$PostgresInstaller
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$GarnetPackage
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
    ServiceWrapperExe=$ServiceWrapperExe
    ExpectedServiceWrapperSHA256=$ExpectedServiceWrapperSHA256
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    BundleDir=$source
    ApprovedManifestSHA256=$ApprovedManifestSHA256
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
}
if ($SyntheticPlanCIFixture) { $setupArgs.SyntheticPlanCIFixture=$true }
$finishArgs=@{
    SourceBundleDir=$source
    AllowedOrigin=$AllowedOrigin
    ListenAddress=$ListenAddress
    TLSCertFile=$TLSCertFile
    TLSKeyFile=$TLSKeyFile
    TrustedRemoteAddress=$TrustedRemoteAddress
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    ApprovedManifestSHA256=$ApprovedManifestSHA256
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
}
# Read-only verification of BOTH stages before any component is installed.
# The delegates also verify fresh paths, service identities, vendor hashes,
# release integrity and TLS origin. Never disable checks in this wrapper.
& $setup @setupArgs | Out-Null
& $finish @finishArgs | Out-Null
if (-not $Apply) {
    Write-Output 'PLAN: PostgreSQL 16, Garnet e ERP em um procedimento tecnico unico.'
    Write-Output 'PLAN: dependencias locais, TLS, banco novo e prova sem habilitar venda.'
    Write-Output 'PLAN: nenhuma pasta, senha, banco ou servico foi alterado.'
    return
}
$setupArgs.Apply=$true
$finishArgs.Apply=$true
# Phase 1 may have created secrets or a service before an error. Do not
# automatically retry or undo a partial PostgreSQL cluster.
try {
    & $setup @setupArgs | Out-Null
} catch {
    throw 'Instalacao das dependencias interrompida; preserve evidencias e faca avaliacao manual. Nao repetir Apply automaticamente.'
}
# Phase 2 may leave an initialized schema. It has its own always-stop guards.
try {
    & $finish @finishArgs | Out-Null
} catch {
    if ($env:ESTERIUM_FULL_INSTALL_DISPOSABLE_CI -eq '1' -and
        $env:GITHUB_ACTIONS -eq 'true' -and
        $env:RUNNER_ENVIRONMENT -eq 'github-hosted') {
        $failure=$_
        $failureFile='unknown'
        if (-not [string]::IsNullOrWhiteSpace($failure.InvocationInfo.ScriptName)) {
            $failureFile=[IO.Path]::GetFileName($failure.InvocationInfo.ScriptName)
        }
        $failureLine=[int]$failure.InvocationInfo.ScriptLineNumber
        $failureType=$failure.Exception.GetType().FullName
        throw ('Instalacao inicial do ERP interrompida; preserve o banco e nao repetir Apply. [CI diagnostic: {0} at {1}:{2}]' -f $failureType,$failureFile,$failureLine)
    }
    throw 'Instalacao inicial do ERP interrompida; nao apagar o banco, nao repetir Apply, nao iniciar a loja.'
}
Write-Output 'PASS: instalacao inicial assinada; migracoes/admin/HTTPS/servicos/atalho e health foram ativados.'
Write-Output 'FISCAL permanece desativado; backup externo, EDB EXE, terminais fisicos e piloto comercial continuam independentes.'
