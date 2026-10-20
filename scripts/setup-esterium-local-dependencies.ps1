#requires -Version 5.1
<#
  Coordinator for fresh Windows/offline dependency setup. In the signed
  package it creates new secret files, generates vendor configs, runs EDB
  unattended and registers a stopped, restricted Garnet service.
  Does NOT apply SQL/schema, trust TLS certs, start services or enable ERP.
  Default PLAN makes no changes.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PostgresInstaller,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresPublisherThumbprint,
    [Parameter(Mandatory=$true)][string]$GarnetPackage,
    [Parameter(Mandatory=$true)][string]$ExpectedGarnetSHA256,
    [Parameter(Mandatory=$true)][string]$ServiceWrapperExe,
    [Parameter(Mandatory=$true)][string]$ExpectedServiceWrapperSHA256,
    [string]$BundleDir=(Split-Path -Parent $PSScriptRoot),
    [string]$BootstrapDir=(Join-Path $env:ProgramData 'Esterium\DependencyBootstrap'),
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$GarnetConfigDir=(Join-Path $env:ProgramData 'Esterium\GarnetConfig'),
    [string]$PostgresInstallDir=(Join-Path $env:ProgramFiles 'PostgreSQL\16'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$GarnetInstallDir=(Join-Path $env:ProgramFiles 'EsteriumGarnet'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [string]$GarnetServiceName='EsteriumGarnet',
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply,
    [switch]$SyntheticPlanCIFixture
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Instalador local exige Windows.' }
if ($SyntheticPlanCIFixture -and ($Apply -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SETUP_PLAN_DISPOSABLE_CI -ne '1')) {
    throw 'Teste sem assinatura restrito ao plano de Windows CI descartável.'
}
if ($PostgresPort -in @(8080,$GarnetPort) -or $GarnetPort -eq 8080) {
    throw 'Banco, cache e interface exigem portas locais diferentes.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($name -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or $name -eq 'EsteriumERP') {
        throw 'Nome de serviço inválido.'
    }
}
if ($PostgresServiceName -eq $GarnetServiceName) { throw 'Nomes de serviços devem ser distintos.' }
if ($PostgresServiceAccount -cnotmatch '^[a-zA-Z][a-zA-Z0-9_-]{2,30}$' -or
    $PostgresServiceAccount -in @('postgres','administrator','system','networkservice','localservice')) {
    throw 'Conta de servico PostgreSQL invalida.'
}
$existingAccounts=@(Get-CimInstance -ClassName Win32_UserAccount -Filter "Name='$PostgresServiceAccount' AND LocalAccount=TRUE" -ErrorAction Stop)
if ($existingAccounts.Count -ne 0) { throw 'Conta PostgreSQL existente: nao sobrescrever.' }
$pgBase=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$programBase=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
foreach ($candidate in @($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresDataDir)) {
    if (-not [IO.Path]::GetFullPath($candidate).StartsWith($pgBase+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Dados e configuracoes devem ficar sob ProgramData Esterium.'
    }
}
foreach ($candidate in @($PostgresInstallDir,$GarnetInstallDir)) {
    if (-not [IO.Path]::GetFullPath($candidate).StartsWith($programBase+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Executaveis devem ficar sob Program Files.'
    }
}
foreach ($dir in @($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresInstallDir,$PostgresDataDir,$GarnetInstallDir)) {
    if (-not [IO.Path]::IsPathRooted($dir) -or $dir.StartsWith('\\')) {
        throw 'Destino de instalacao deve usar disco local absoluto.'
    }
    if (Test-Path -LiteralPath $dir) {
        throw 'Ha arquivos anteriores de instalacao: nao sobrescrever.'
    }
}
$allPaths=@($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresInstallDir,$PostgresDataDir,$GarnetInstallDir) |
    ForEach-Object { [IO.Path]::GetFullPath($_).TrimEnd('\') }
for ($i=0;$i -lt $allPaths.Count;$i++) {
    for ($j=$i+1;$j -lt $allPaths.Count;$j++) {
        if ([string]::Equals($allPaths[$i],$allPaths[$j],[StringComparison]::OrdinalIgnoreCase) -or
            $allPaths[$i].StartsWith($allPaths[$j]+'\',[StringComparison]::OrdinalIgnoreCase) -or
            $allPaths[$j].StartsWith($allPaths[$i]+'\',[StringComparison]::OrdinalIgnoreCase)) {
            throw 'Instalador exige seis pastas novas e isoladas.'
        }
    }
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($null -ne (Get-Service -Name $name -ErrorAction SilentlyContinue)) {
        throw 'Serviço já registrado: não reinstalar automaticamente.'
    }
}
foreach ($port in @($PostgresPort,$GarnetPort)) {
    if (@(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw 'Porta em uso: escolha equipamento limpo ou rever o conflito.'
    }
}
$sourceGate=Join-Path $PSScriptRoot 'check-esterium-offline-dependency-packages.ps1'
$bootstrap=Join-Path $PSScriptRoot 'provision-esterium-dependency-bootstrap.ps1'
$prepare=Join-Path $PSScriptRoot 'prepare-esterium-local-dependency-configs.ps1'
$install=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
$releaseGate=Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
$trustedHelpers=@(
    'verify-esterium-bundle.ps1',
    'check-esterium-signed-release.ps1',
    'check-esterium-offline-dependency-packages.ps1',
    'provision-esterium-dependency-bootstrap.ps1',
    'prepare-esterium-local-dependency-configs.ps1',
    'provision-esterium-postgres-optionfile.ps1',
    'provision-esterium-garnet-config.ps1',
    'install-esterium-local-dependencies.ps1'
)
foreach ($filename in $trustedHelpers) {
    if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $filename) -PathType Leaf)) {
        throw 'Instalador está incompleto: falta componente confiável.'
    }
}
if ($Apply) {
if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$' -or
    $ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$') {
    throw 'Execução real exige certificado de editor e manifesto aprovados fora do ZIP.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Execução real exige Administrador.'
}
# The outer installer/OS must establish the launcher's trust before this code
# runs. Verify all delegated privileged scripts before the first write.
foreach ($signedPath in @($PSCommandPath) + @($trustedHelpers | ForEach-Object {
    Join-Path $PSScriptRoot $_
})) {
    $signature=Get-AuthenticodeSignature -LiteralPath $signedPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
           $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Instalador e componentes precisam de assinatura do editor previamente aprovado.'
    }
}
& $releaseGate -BundleDir $BundleDir -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null

}
$packageArgs=@{
    PostgresInstaller=$PostgresInstaller
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$GarnetPackage
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
}
if ($SyntheticPlanCIFixture) { $packageArgs.SyntheticUnsignedCIFixture=$true }
& $sourceGate @packageArgs | Out-Null
if ($ExpectedServiceWrapperSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
    -not [IO.Path]::IsPathRooted($ServiceWrapperExe) -or
    $ServiceWrapperExe.StartsWith('\\') -or
    -not (Test-Path -LiteralPath $ServiceWrapperExe -PathType Leaf) -or
    (Get-FileHash -LiteralPath $ServiceWrapperExe -Algorithm SHA256).Hash -cne
        $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
    throw 'Gerenciador de serviço NSSM não possui hash aprovado de fonte independente.'
}
$bootstrapArgs=@{DestinationDir=$BootstrapDir}
& $bootstrap @bootstrapArgs | Out-Null
if (-not $Apply) {
    Write-Output 'PLAN: pacotes aprovados, portas e pastas novas, geração de segredos e execução offline planejadas.'
    Write-Output 'NEXT: modo real assinado cria PostgreSQL manual/parado e Garnet manual/parado; não configura banco comercial.'
    Write-Output 'PLAN: nenhum segredo, serviço, pasta ou dado foi alterado.'
    return
}
# Confirm vendor files again after the release provenance check.
& $sourceGate @packageArgs | Out-Null
$bootstrapArgs.Apply=$true
$bootstrapArgs.TrustedPublisherThumbprint=$TrustedPublisherThumbprint
& $bootstrap @bootstrapArgs | Out-Null
$prepareArgs=@{
    PostgresSuperPasswordFile=(Join-Path $BootstrapDir 'pg-super.secret')
    PostgresServicePasswordFile=(Join-Path $BootstrapDir 'pg-service.secret')
    GarnetAclFile=(Join-Path $BootstrapDir 'garnet.acl')
    GarnetCertificatePfxFile=(Join-Path $BootstrapDir 'garnet.pfx')
    GarnetCertificatePasswordFile=(Join-Path $BootstrapDir 'garnet-pfx.secret')
    PostgresOptionsDir=$PostgresOptionsDir
    GarnetConfigDir=$GarnetConfigDir
    PostgresInstallDir=$PostgresInstallDir
    PostgresDataDir=$PostgresDataDir
    PostgresServiceName=$PostgresServiceName
    PostgresServiceAccount=$PostgresServiceAccount
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
    Apply=$true
}
& $prepare @prepareArgs | Out-Null
$installArgs=@{
    PostgresInstaller=$PostgresInstaller
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$GarnetPackage
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
    ServiceWrapperExe=$ServiceWrapperExe
    ExpectedServiceWrapperSHA256=$ExpectedServiceWrapperSHA256
    PostgresOptionsFile=(Join-Path $PostgresOptionsDir 'postgresql-unattended.options')
    GarnetConfigFile=(Join-Path $GarnetConfigDir 'garnet.conf')
    PostgresInstallDir=$PostgresInstallDir
    PostgresDataDir=$PostgresDataDir
    PostgresServiceName=$PostgresServiceName
    GarnetServiceName=$GarnetServiceName
    GarnetInstallDir=$GarnetInstallDir
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
    Apply=$true
}
& $install @installArgs | Out-Null
Write-Output 'PASS: dependências locais instaladas com um comando técnico; serviços permanecem parados.'
Write-Output 'NOT READY: confiança TLS/SQL/migrações e teste real de recuperação ainda necessários antes da primeira venda.'
 -or
    $PostgresServiceAccount -in @('postgres','administrator','system','networkservice','localservice')) {
    throw 'Conta de servico PostgreSQL invalida.'
}
$existingAccounts=@(Get-CimInstance -ClassName Win32_UserAccount -Filter "Name='$PostgresServiceAccount' AND LocalAccount=TRUE" -ErrorAction Stop)
if ($existingAccounts.Count -ne 0) { throw 'Conta PostgreSQL existente: nao sobrescrever.' }
$pgBase=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
$programBase=[IO.Path]::GetFullPath($env:ProgramFiles).TrimEnd('\')
foreach ($candidate in @($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresDataDir)) {
    if (-not [IO.Path]::GetFullPath($candidate).StartsWith($pgBase+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Dados e configuracoes devem ficar sob ProgramData Esterium.'
    }
}
foreach ($candidate in @($PostgresInstallDir,$GarnetInstallDir)) {
    if (-not [IO.Path]::GetFullPath($candidate).StartsWith($programBase+'\',[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Executaveis devem ficar sob Program Files.'
    }
}
foreach ($dir in @($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresInstallDir,$PostgresDataDir,$GarnetInstallDir)) {
    if (-not [IO.Path]::IsPathRooted($dir) -or $dir.StartsWith('\\')) {
        throw 'Destino de instalação deve usar disco local absoluto.'
    }
    if (Test-Path -LiteralPath $dir) {
        throw 'Há arquivos anteriores de instalação: não sobrescrever nem rotacionar.'
    }
}
$allPaths=@($BootstrapDir,$PostgresOptionsDir,$GarnetConfigDir,$PostgresInstallDir,$PostgresDataDir,$GarnetInstallDir) |
    ForEach-Object { [IO.Path]::GetFullPath($_).TrimEnd('\') }
for ($i=0;$i -lt $allPaths.Count;$i++) {
    for ($j=$i+1;$j -lt $allPaths.Count;$j++) {
        if ([string]::Equals($allPaths[$i],$allPaths[$j],[StringComparison]::OrdinalIgnoreCase) -or
            $allPaths[$i].StartsWith($allPaths[$j]+'\',[StringComparison]::OrdinalIgnoreCase) -or
            $allPaths[$j].StartsWith($allPaths[$i]+'\',[StringComparison]::OrdinalIgnoreCase)) {
            throw 'Instalador exige seis pastas novas e isoladas.'
        }
    }
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($null -ne (Get-Service -Name $name -ErrorAction SilentlyContinue)) {
        throw 'Serviço já registrado: não reinstalar automaticamente.'
    }
}
foreach ($port in @($PostgresPort,$GarnetPort)) {
    if (@(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw 'Porta em uso: escolha equipamento limpo ou rever o conflito.'
    }
}
$sourceGate=Join-Path $PSScriptRoot 'check-esterium-offline-dependency-packages.ps1'
$bootstrap=Join-Path $PSScriptRoot 'provision-esterium-dependency-bootstrap.ps1'
$prepare=Join-Path $PSScriptRoot 'prepare-esterium-local-dependency-configs.ps1'
$install=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
$releaseGate=Join-Path $PSScriptRoot 'check-esterium-signed-release.ps1'
$trustedHelpers=@(
    'verify-esterium-bundle.ps1',
    'check-esterium-signed-release.ps1',
    'check-esterium-offline-dependency-packages.ps1',
    'provision-esterium-dependency-bootstrap.ps1',
    'prepare-esterium-local-dependency-configs.ps1',
    'provision-esterium-postgres-optionfile.ps1',
    'provision-esterium-garnet-config.ps1',
    'install-esterium-local-dependencies.ps1'
)
foreach ($filename in $trustedHelpers) {
    if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $filename) -PathType Leaf)) {
        throw 'Instalador está incompleto: falta componente confiável.'
    }
}
if ($Apply) {
if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$' -or
    $ApprovedManifestSHA256 -cnotmatch '^[a-fA-F0-9]{64}$') {
    throw 'Execução real exige certificado de editor e manifesto aprovados fora do ZIP.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Execução real exige Administrador.'
}
# The outer installer/OS must establish the launcher's trust before this code
# runs. Verify all delegated privileged scripts before the first write.
foreach ($signedPath in @($PSCommandPath) + @($trustedHelpers | ForEach-Object {
    Join-Path $PSScriptRoot $_
})) {
    $signature=Get-AuthenticodeSignature -LiteralPath $signedPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
        $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
           $TrustedPublisherThumbprint.ToUpperInvariant()) {
        throw 'Instalador e componentes precisam de assinatura do editor previamente aprovado.'
    }
}
& $releaseGate -BundleDir $BundleDir -ExpectedManifestSHA256 $ApprovedManifestSHA256 -ExpectedPublisherThumbprint $TrustedPublisherThumbprint | Out-Null

}
$packageArgs=@{
    PostgresInstaller=$PostgresInstaller
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$GarnetPackage
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
}
if ($SyntheticPlanCIFixture) { $packageArgs.SyntheticUnsignedCIFixture=$true }
& $sourceGate @packageArgs | Out-Null
if ($ExpectedServiceWrapperSHA256 -cnotmatch '^[a-fA-F0-9]{64}$' -or
    -not [IO.Path]::IsPathRooted($ServiceWrapperExe) -or
    $ServiceWrapperExe.StartsWith('\\') -or
    -not (Test-Path -LiteralPath $ServiceWrapperExe -PathType Leaf) -or
    (Get-FileHash -LiteralPath $ServiceWrapperExe -Algorithm SHA256).Hash -cne
        $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
    throw 'Gerenciador de serviço NSSM não possui hash aprovado de fonte independente.'
}
$bootstrapArgs=@{DestinationDir=$BootstrapDir}
& $bootstrap @bootstrapArgs | Out-Null
if (-not $Apply) {
    Write-Output 'PLAN: pacotes aprovados, portas e pastas novas, geração de segredos e execução offline planejadas.'
    Write-Output 'NEXT: modo real assinado cria PostgreSQL manual/parado e Garnet manual/parado; não configura banco comercial.'
    Write-Output 'PLAN: nenhum segredo, serviço, pasta ou dado foi alterado.'
    return
}
# Confirm vendor files again after the release provenance check.
& $sourceGate @packageArgs | Out-Null
$bootstrapArgs.Apply=$true
$bootstrapArgs.TrustedPublisherThumbprint=$TrustedPublisherThumbprint
& $bootstrap @bootstrapArgs | Out-Null
$prepareArgs=@{
    PostgresSuperPasswordFile=(Join-Path $BootstrapDir 'pg-super.secret')
    PostgresServicePasswordFile=(Join-Path $BootstrapDir 'pg-service.secret')
    GarnetAclFile=(Join-Path $BootstrapDir 'garnet.acl')
    GarnetCertificatePfxFile=(Join-Path $BootstrapDir 'garnet.pfx')
    GarnetCertificatePasswordFile=(Join-Path $BootstrapDir 'garnet-pfx.secret')
    PostgresOptionsDir=$PostgresOptionsDir
    GarnetConfigDir=$GarnetConfigDir
    PostgresInstallDir=$PostgresInstallDir
    PostgresDataDir=$PostgresDataDir
    PostgresServiceName=$PostgresServiceName
    PostgresServiceAccount=$PostgresServiceAccount
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
    Apply=$true
}
& $prepare @prepareArgs | Out-Null
$installArgs=@{
    PostgresInstaller=$PostgresInstaller
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$GarnetPackage
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
    ServiceWrapperExe=$ServiceWrapperExe
    ExpectedServiceWrapperSHA256=$ExpectedServiceWrapperSHA256
    PostgresOptionsFile=(Join-Path $PostgresOptionsDir 'postgresql-unattended.options')
    GarnetConfigFile=(Join-Path $GarnetConfigDir 'garnet.conf')
    PostgresInstallDir=$PostgresInstallDir
    PostgresDataDir=$PostgresDataDir
    PostgresServiceName=$PostgresServiceName
    GarnetServiceName=$GarnetServiceName
    GarnetInstallDir=$GarnetInstallDir
    PostgresPort=$PostgresPort
    GarnetPort=$GarnetPort
    TrustedPublisherThumbprint=$TrustedPublisherThumbprint
    Apply=$true
}
& $install @installArgs | Out-Null
Write-Output 'PASS: dependências locais instaladas com um comando técnico; serviços permanecem parados.'
Write-Output 'NOT READY: confiança TLS/SQL/migrações e teste real de recuperação ainda necessários antes da primeira venda.'
