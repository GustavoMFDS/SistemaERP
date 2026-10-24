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
$origin=$null
if (-not [Uri]::TryCreate($AllowedOrigin,[UriKind]::Absolute,[ref]$origin) -or
    $origin.Scheme -cne 'https' -or $origin.UserInfo -ne '' -or
    [string]::IsNullOrWhiteSpace($origin.Host) -or
    $origin.AbsolutePath -ne '/' -or $origin.Query -ne '' -or
    $origin.Fragment -ne '' -or $origin.Host -in @('localhost','127.0.0.1','::1')) {
    throw 'Instalacao exige origem HTTPS aprovada para o navegador da loja.'
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
foreach ($helper in @($setup,$finish)) {
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
    foreach ($path in @($PSCommandPath,$setup,$finish)) {
        $signature=Get-AuthenticodeSignature -LiteralPath $path
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
              $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Instalador completo sem assinatura do editor aprovado.'
        }
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
    throw 'Instalacao inicial do ERP interrompida; nao apagar o banco, nao repetir Apply, nao iniciar a loja.'
}
Write-Output 'PASS: instalacao inicial assinada e verificada, com dependencias/ERP ainda bloqueados para venda.'
Write-Output 'NOT READY: exige teste real EDB/NSSM, HTTPS dos terminais, backup restaurado, piloto fisico e autorizacao comercial.'
