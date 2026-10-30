#requires -Version 5.1
<#
  Instalação técnica offline das dependências em Windows novo.
  Pré-requisitos: opções EDB e configuração Garnet já geradas por
  prepare-esterium-local-dependency-configs.ps1, ambas com ACL restrita.
  Padrão: somente plano, sem executar programas. -Apply exige assinatura.
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
    [Parameter(Mandatory=$true)][string]$PostgresOptionsFile,
    [Parameter(Mandatory=$true)][string]$GarnetConfigFile,
    [string]$PostgresInstallDir=(Join-Path $env:ProgramFiles 'PostgreSQL\16'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [string]$GarnetServiceName='EsteriumGarnet',
    [string]$GarnetInstallDir=(Join-Path $env:ProgramFiles 'EsteriumGarnet'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$TrustedPublisherThumbprint='',
    [switch]$SyntheticUnsignedCIFixture,
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Instalador exige Windows.' }
if ($SyntheticUnsignedCIFixture -and ($Apply -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_DEPENDENCY_INSTALL_DISPOSABLE_CI -ne '1')) {
    throw 'Fixture sem assinatura somente em planejamento no runner Windows descartável.'
}
if ($PostgresPort -in @(8080,$GarnetPort) -or $GarnetPort -eq 8080) {
    throw 'Portas da aplicação, banco e cache devem ser distintas.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($name -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or $name -eq 'EsteriumERP') {
        throw 'Nome inválido de serviço Windows.'
    }
}
if ($PostgresServiceName -eq $GarnetServiceName) { throw 'Serviços não podem compartilhar nome.' }
if ($PostgresServiceAccount -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,30}$' -or
    $PostgresServiceAccount -in @('postgres','administrator','system','networkservice','localservice')) {
    throw 'Conta Windows PostgreSQL invalida ou privilegiada.'
}
if (@(Get-CimInstance -ClassName Win32_UserAccount -Filter "Name='$PostgresServiceAccount' AND LocalAccount=TRUE" -ErrorAction Stop).Count -gt 0) {
    throw 'Conta PostgreSQL ja existente: nao trocar senha nem reutilizar conta.'
}
foreach ($digest in @($ExpectedPostgresSHA256,$ExpectedGarnetSHA256,$ExpectedServiceWrapperSHA256)) {
    if ($digest -cnotmatch '^[a-fA-F0-9]{64}$') { throw 'SHA-256 aprovado é obrigatório para cada pacote.' }
}
function Get-SafeFile([string]$value) {
    if (-not [IO.Path]::IsPathRooted($value) -or $value.StartsWith('\\')) {
        throw 'Somente caminhos absolutos em disco local são aceitos.'
    }
    $p=(Resolve-Path -LiteralPath $value -ErrorAction Stop).Path
    $item=Get-Item -LiteralPath $p -Force
    if ($item.PSIsContainer -or $item.Length -le 0) { throw 'Arquivo offline ausente ou vazio.' }
    $cursor=$p
    while ($cursor) {
        $candidate=Get-Item -LiteralPath $cursor -Force
        if (($candidate.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Link/junção não permitido em arquivos do instalador.'
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    return $p
}
function Assert-NewDestination([string]$value,[string]$root) {
    if (-not [IO.Path]::IsPathRooted($value) -or $value.StartsWith('\\')) {
        throw 'Destino deve ser absoluto e local.'
    }
    $p=[IO.Path]::GetFullPath($value).TrimEnd('\')
    $prefix=[IO.Path]::GetFullPath($root).TrimEnd('\') + '\'
    if (-not $p.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)) {
        throw 'Destino fora dos diretórios de instalação permitidos.'
    }
    if (Test-Path -LiteralPath $p) { throw 'Instalação existente não será sobrescrita.' }
    $cursor=Split-Path -Parent $p
    while ($cursor) {
        if (Test-Path -LiteralPath $cursor) {
            if (((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw 'Destino contém link/junção não permitido.'
            }
        }
        $parent=Split-Path -Parent $cursor
        if ($parent -eq $cursor) { break }
        $cursor=$parent
    }
    return $p
}
$pg=Get-SafeFile $PostgresInstaller
$zip=Get-SafeFile $GarnetPackage
$wrapper=Get-SafeFile $ServiceWrapperExe
$options=Get-SafeFile $PostgresOptionsFile
$configPath=Get-SafeFile $GarnetConfigFile
if ([IO.Path]::GetExtension($pg).ToLowerInvariant() -ne '.exe' -or
    [IO.Path]::GetExtension($zip).ToLowerInvariant() -ne '.zip' -or
    [IO.Path]::GetExtension($wrapper).ToLowerInvariant() -ne '.exe') {
    throw 'Exigidos PostgreSQL .exe, Garnet .zip e wrapper Windows .exe.'
}
$pgDir=Assert-NewDestination $PostgresInstallDir $env:ProgramFiles
$pgData=Assert-NewDestination $PostgresDataDir (Join-Path $env:ProgramData 'Esterium')
$garnetDir=Assert-NewDestination $GarnetInstallDir $env:ProgramFiles
if ($pgDir.StartsWith($garnetDir+'\',[StringComparison]::OrdinalIgnoreCase) -or
    $garnetDir.StartsWith($pgDir+'\',[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Binários de PostgreSQL e Garnet devem ficar em pastas separadas.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($null -ne (Get-Service -Name $name -ErrorAction SilentlyContinue)) {
        throw 'Serviço já existente: não alterar.'
    }
}
foreach ($port in @($PostgresPort,$GarnetPort)) {
    if (@(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw 'Porta já ocupada: não instalar.'
    }
}
if ((Get-FileHash -LiteralPath $wrapper -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
    throw 'Wrapper de serviço não corresponde ao hash aprovado.'
}
$checker=Join-Path $PSScriptRoot 'check-esterium-offline-dependency-packages.ps1'
if (-not (Test-Path -LiteralPath $checker -PathType Leaf)) {
    throw 'Verificador de procedência offline ausente.'
}
if ($Apply) {
    if ($TrustedPublisherThumbprint -cnotmatch '^[a-fA-F0-9]{40}$') {
        throw 'Editor assinado aprovado independentemente é obrigatório.'
    }
    foreach ($scriptPath in @($PSCommandPath,$checker)) {
        $signature=Get-AuthenticodeSignature -LiteralPath $scriptPath
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Thumbprint.ToUpperInvariant() -cne
              $TrustedPublisherThumbprint.ToUpperInvariant()) {
            throw 'Instalador ou verificador não é assinado pelo editor aprovado.'
        }
    }
    $principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Instalação real requer administrador.'
    }
    $dotnetRuntimeSource=Join-Path $PSScriptRoot 'dotnet'
    $dotnetSourceExe=Join-Path $dotnetRuntimeSource 'dotnet.exe'
    if (-not (Test-Path -LiteralPath $dotnetRuntimeSource -PathType Container) -or
        ((Get-Item -LiteralPath $dotnetRuntimeSource -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Pacote assinado nao contem uma pasta de runtime .NET local segura para Garnet.'
    }
    foreach ($entry in @(Get-ChildItem -LiteralPath $dotnetRuntimeSource -Recurse -Force)) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Runtime .NET local contem link ou juncao proibido.'
        }
    }
    foreach ($relative in @('LICENSE.txt','ThirdPartyNotices.txt',
        'host\fxr\10.0.14\hostfxr.dll',
        'shared\Microsoft.NETCore.App\10.0.14\System.Private.CoreLib.dll')) {
        $runtimeFile=Join-Path $dotnetRuntimeSource $relative
        if (-not (Test-Path -LiteralPath $runtimeFile -PathType Leaf) -or
            ((Get-Item -LiteralPath $runtimeFile -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Pacote assinado nao contem o runtime .NET local esperado para Garnet.'
        }
    }
    if (-not (Test-Path -LiteralPath $dotnetSourceExe -PathType Leaf) -or
        ((Get-Item -LiteralPath $dotnetSourceExe -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Pacote assinado nao contem dotnet.exe local para Garnet.'
    }
    $runtimeList=@(& $dotnetSourceExe --list-runtimes)
    if ($LASTEXITCODE -ne 0 -or -not ($runtimeList -match 'Microsoft\.NETCore\.App 10\.0\.14')) {
        throw 'Runtime .NET 10.0.14 privado ausente ou invalido para Garnet.'
    }
}
$packageArgs=@{
    PostgresInstaller=$pg
    ExpectedPostgresSHA256=$ExpectedPostgresSHA256
    ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint
    GarnetPackage=$zip
    ExpectedGarnetSHA256=$ExpectedGarnetSHA256
}
if ($SyntheticUnsignedCIFixture) { $packageArgs.SyntheticUnsignedCIFixture=$true }
& $checker @packageArgs | Out-Null
$allowedKeys=@('mode','prefix','datadir','serverport','servicename','serviceaccount',
    'superaccount','superpassword','servicepassword','enable-components','enable_acledit','create_shortcuts','unattendedmodeui')
$lines=@([IO.File]::ReadAllLines($options))
$seen=@{}
foreach ($line in $lines) {
    if ($line -notmatch '^([a-z_-]+)=(.*)$') { throw 'Opções EDB inválidas.' }
    $key=$matches[1]
    if ($key -notin $allowedKeys -or $seen.ContainsKey($key)) {
        throw 'Opções EDB duplicadas ou extras.'
    }
    $seen[$key]=$true
}
if ($seen.Count -ne $allowedKeys.Count) { throw 'Opções EDB incompletas.' }
$required=@{
    mode='unattended';prefix=$pgDir;datadir=$pgData
    servicename=$PostgresServiceName;serverport=[string]$PostgresPort
    serviceaccount=$PostgresServiceAccount
    'enable-components'='server,commandlinetools';enable_acledit='1';'create_shortcuts'='0'
    unattendedmodeui='minimal';superaccount='postgres'
}
foreach ($entry in $required.GetEnumerator()) {
    if (@($lines | Where-Object { $_ -ceq ($entry.Key+'='+$entry.Value) }).Count -ne 1) {
        throw 'Opções EDB não correspondem aos destinos e portas selecionados.'
    }
}
function Assert-Readers([string]$p,[string[]]$allowed,[string]$mustRead) {
    $found=$false
    foreach ($ace in (Get-Acl -LiteralPath $p).Access) {
        if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if (($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -ne 0) {
            if ($sid -notin $allowed) { throw 'Configuração possui permissões de leitura excessivas.' }
            if ($sid -eq $mustRead) { $found=$true }
        }
        if ($sid -eq 'S-1-5-20' -and
            ($ace.FileSystemRights -band ([Security.AccessControl.FileSystemRights]::WriteData -bor
              [Security.AccessControl.FileSystemRights]::Delete -bor
              [Security.AccessControl.FileSystemRights]::ChangePermissions -bor
              [Security.AccessControl.FileSystemRights]::TakeOwnership)) -ne 0) {
            throw 'Garnet não pode alterar seu arquivo de configuração.'
        }
    }
    if (-not $found) { throw 'Arquivo protegido não possui acesso esperado.' }
}
Assert-Readers $options @('S-1-5-18','S-1-5-32-544') 'S-1-5-18'
Assert-Readers $configPath @('S-1-5-18','S-1-5-32-544','S-1-5-20') 'S-1-5-20'
$config=Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
if ($config.Address -cne '127.0.0.1' -or [int]$config.Port -ne $GarnetPort -or
    $config.EnableTLS -cne $true -or $config.AuthenticationMode -cne 'ACL' -or
    $config.EnableLua -cne $true -or $config.LuaTransactionMode -cne $true) {
    throw 'Garnet exige loopback, TLS, ACL e operações Lua.'
}
# The vendor JSON is untrusted until every sensitive path, policy and ACL
# is checked. In particular, a forged config could point at a different ACL
# or PFX, or turn off scripting limits, without changing the basic flags.
$keys=@('Address','Port','EnableCluster','AuthenticationMode','AclFile',
    'EnableTLS','ClientCertificateRequired','CertFileName','CertPassword','EnableLua','LuaTransactionMode',
    'LuaMemoryManagementMode','LuaScriptMemoryLimit','LuaScriptTimeoutMs',
    'EnableAOF','WaitForCommit','CommitFrequencyMs','Recover',
    'FailOnRecoveryError','UseAofNullDevice','CheckpointDir')
$names=@($config.PSObject.Properties | ForEach-Object { $_.Name })
if ($names.Count -ne $keys.Count -or @($names | Where-Object { $_ -notin $keys }).Count -ne 0 -or
    $config.EnableCluster -cne $false -or
    $config.ClientCertificateRequired -cne $false -or
    $config.LuaMemoryManagementMode -cne 'Tracked' -or
    $config.LuaScriptMemoryLimit -cne '8m' -or
    [int]$config.LuaScriptTimeoutMs -ne 5000 -or
    $config.EnableAOF -cne $true -or $config.WaitForCommit -cne $true -or
    [int]$config.CommitFrequencyMs -ne 0 -or
    $config.Recover -cne $true -or $config.FailOnRecoveryError -cne $true -or
    $config.UseAofNullDevice -cne $false) {
    throw 'Configuração Garnet não corresponde ao perfil protegido do Esterium.'
}
# Mutable AOF state must be a separate directory below the protected
# configuration parent; NETWORK SERVICE may write state but not settings.
$stateRoot=Join-Path (Split-Path -Parent $configPath) 'state'
$expectedCheckpoint=[IO.Path]::GetFullPath((Join-Path $stateRoot 'checkpoints'))
if (-not [string]::Equals([string]$config.CheckpointDir,$expectedCheckpoint,[StringComparison]::OrdinalIgnoreCase)) {
    throw 'Pasta de checkpoints AOF Garnet deve ser isolada sob state.'
}
foreach ($dir in @($stateRoot,$expectedCheckpoint)) {
    if (-not (Test-Path -LiteralPath $dir -PathType Container) -or
        ((Get-Item -LiteralPath $dir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'Armazenamento de durabilidade Garnet ausente ou link proibido.'
    }
    $hasWritableService=$false
    foreach ($ace in (Get-Acl -LiteralPath $dir).Access) {
        if ($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { continue }
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $writes=[Security.AccessControl.FileSystemRights]::WriteData -bor
            [Security.AccessControl.FileSystemRights]::AppendData -bor
            [Security.AccessControl.FileSystemRights]::Delete -bor
            [Security.AccessControl.FileSystemRights]::ChangePermissions
        if (($ace.FileSystemRights -band $writes) -ne 0) {
            if ($sid -notin @('S-1-5-18','S-1-5-32-544','S-1-5-20')) {
                throw 'Armazenamento Garnet permite gravacao por usuario nao aprovado.'
            }
            if ($sid -eq 'S-1-5-20') { $hasWritableService=$true }
        }
    }
    if (-not $hasWritableService) { throw 'Servico Garnet nao pode gravar AOF/checkpoint.' }
}
$aclPath=Get-SafeFile ([string]$config.AclFile)
$pfxPath=Get-SafeFile ([string]$config.CertFileName)
if ($aclPath -eq $pfxPath -or [IO.Path]::GetExtension($pfxPath).ToLowerInvariant() -ne '.pfx') {
    throw 'Arquivo de certificado e ACL Garnet devem ser separados e conter PFX válido.'
}
Assert-Readers $aclPath @('S-1-5-18','S-1-5-32-544','S-1-5-20') 'S-1-5-20'
Assert-Readers $pfxPath @('S-1-5-18','S-1-5-32-544','S-1-5-20') 'S-1-5-20'
$garnetAclText=[IO.File]::ReadAllText($aclPath)
if ($garnetAclText.Length -gt 131072 -or
    $garnetAclText -notmatch '(?im)^\s*user\s+default\s+off(?:\s|$)' -or
    $garnetAclText -match '(?i)\bnopass\b' -or
    $garnetAclText -notmatch '(?im)^\s*user\s+(?!default\b)\S+\s+on\b[^\r\n]*[>#]\S+') {
    throw 'ACL Garnet deve proibir usuário padrão e senhas ausentes.'
}
if ([string]::IsNullOrWhiteSpace([string]$config.CertPassword) -or
    ([string]$config.CertPassword).Length -gt 4096) {
    throw 'Senha PFX Garnet não configurada ou longa demais.'
}
if ($Apply) {
    # Validate certificate+private key locally; no network or third-party code.
    $certificate=$null
    try {
        $certificate=[Security.Cryptography.X509Certificates.X509Certificate2]::new(
            $pfxPath, [string]$config.CertPassword,
            [Security.Cryptography.X509Certificates.X509KeyStorageFlags]::EphemeralKeySet)
        if (-not $certificate.HasPrivateKey -or
            $certificate.NotBefore.ToUniversalTime() -gt [DateTime]::UtcNow -or
            $certificate.NotAfter.ToUniversalTime() -le [DateTime]::UtcNow) {
            throw 'Certificado Garnet está sem chave privada ou fora de validade.'
        }
    } catch {
        throw 'Arquivo PFX Garnet não foi validado: revisar certificado e senha.'
    } finally {
        if ($null -ne $certificate) { $certificate.Dispose() }
    }
}
$freshMarker=Join-Path ([IO.Path]::GetDirectoryName($options)) ('PostgresFresh-'+$PostgresServiceName+'.marker')
if (Test-Path -LiteralPath $freshMarker) {
    throw 'Existing PostgreSQL fresh-cluster attestation: never reinstall automatically.'
}
if (-not $Apply) {
    Write-Output 'PLAN: pacotes, fontes, portas e configurações validados sem alteração no PC.'
    Write-Output 'NEXT: -Apply instalará EDB silenciosamente e registrará Garnet parado (ainda sem banco/schema TLS comercial).'
    return
}
# Revalidate trusted third-party artifacts immediately before execution.
# A previous PLAN approval cannot authorize a different EXE at Apply time.
& $checker @packageArgs | Out-Null
if ((Get-FileHash -LiteralPath $wrapper -Algorithm SHA256).Hash -cne
    $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
    throw 'Wrapper de serviço foi alterado antes da instalação.'
}
Write-Output '1/3: instalando PostgreSQL 16 sem interface...'
$proc=$null
try {
    # Passwords stay in the restricted EDB option file, not shell arguments.
    $proc=Start-Process -FilePath $pg -ArgumentList @('--mode','unattended','--unattendedmodeui','none','--optionfile',('"' + $options + '"')) -Wait -PassThru -ErrorAction Stop
} finally {
    # Even a partially failed EDB installer may have registered an *automatic*
    # database service. Disable/stop this newly selected service before any
    # filesystem/version checks, including when Start-Process itself throws.
    try {
        if (Test-Path -LiteralPath $options) {
            Remove-Item -LiteralPath $options -Force -ErrorAction Stop
        }
    } finally {
        $newService=Get-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue
        if ($null -ne $newService) {
            try {
                Set-Service -Name $PostgresServiceName -StartupType Disabled -ErrorAction Stop
                if ($newService.Status -ne 'Stopped') {
                    Stop-Service -Name $PostgresServiceName -ErrorAction Stop
                    (Get-Service -Name $PostgresServiceName -ErrorAction Stop).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,
                        [TimeSpan]::FromSeconds(45))
                }
            } catch {
                throw 'Falha ao proteger PostgreSQL parcial: verifique serviço e rede antes de prosseguir.'
            }
        }
    }
}
if ($null -eq $proc -or $proc.ExitCode -ne 0) {
    throw 'Instalador EDB não concluiu: conferir serviço/dados parciais; não reinstalar automaticamente.'
}
if (-not (Test-Path -LiteralPath (Join-Path $pgDir 'bin\pg_isready.exe')) -or
    -not (Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION')) -or
    ([IO.File]::ReadAllText((Join-Path $pgData 'PG_VERSION'))).Trim() -cne '16' -or
    $null -eq (Get-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue)) {
    throw 'PostgreSQL não confirmou serviço, binários e cluster versão 16. Serviço permanece parado.'
}
$pgService=@(Get-CimInstance -ClassName Win32_Service -Filter "Name='$PostgresServiceName'" -ErrorAction Stop)
if ($pgService.Count -ne 1 -or $pgService[0].StartMode -cne 'Disabled' -or
    $pgService[0].State -cne 'Stopped' -or [int]$pgService[0].ProcessId -ne 0) {
    throw 'PostgreSQL deve estar desabilitado e parado antes da configuracao TLS.'
}
$pgIdentity=[string]$pgService[0].StartName
$localAccounts=@($PostgresServiceAccount,('.\'+$PostgresServiceAccount),($env:COMPUTERNAME+'\'+$PostgresServiceAccount))
if ($pgIdentity -notin $localAccounts) {
    throw 'Servico PostgreSQL nao pertence a conta Windows dedicada aprovada.'
}
# A listener should not survive shutdown; reject a port still occupied
# instead of inferring that the new service is ready for production.
if (@(Get-NetTCPConnection -LocalPort $PostgresPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
    throw 'A porta PostgreSQL ainda está ocupada após o desligamento: revisar identidade do processo.'
}
# Seal the new-cluster proof in the already protected install-options folder.
# Never write a proof for existing services or before disabled/stopped SCM
# identity, PG_VERSION and zero-listener guards have passed.
$markerBody='ESTERIUM_FRESH_POSTGRES_16'+[Environment]::NewLine+
    $PostgresServiceName+[Environment]::NewLine+
    [IO.Path]::GetFullPath($pgData)
$markerBytes=[Text.UTF8Encoding]::new($false).GetBytes($markerBody)
try {
    $markerStream=[IO.File]::Open($freshMarker,[IO.FileMode]::CreateNew,
        [IO.FileAccess]::Write,[IO.FileShare]::None)
    try {
        $markerStream.Write($markerBytes,0,$markerBytes.Length)
        $markerStream.Flush($true)
    } finally { $markerStream.Dispose() }
} finally { [Array]::Clear($markerBytes,0,$markerBytes.Length) }
& icacls.exe $freshMarker '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Fresh PostgreSQL attestation ACL could not be protected.' }
Write-Output 'PASS 1/3: PostgreSQL 16 instalado, serviço desabilitado e parado para configuração de TLS/SCRAM.'
Write-Output '2/3: instalando binários e serviço Garnet...'
$stage=$garnetDir+'.staging-'+[Guid]::NewGuid().ToString('N')
$published=$false
$created=$false
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    # Restrict the empty staging directory BEFORE extracting or copying any
    # untrusted vendor bytes. All children inherit the safe ACL at creation.
    & icacls.exe $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Não foi possível proteger pasta temporária Garnet.' }
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash -cne $ExpectedGarnetSHA256.ToUpperInvariant()) {
        throw 'Arquivo Garnet mudou após a validação.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $stage -ErrorAction Stop
    foreach ($entry in @(Get-ChildItem -LiteralPath $stage -Recurse -Force)) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Garnet extraído contém link/junção proibido.'
        }
    }
    $servers=@(Get-ChildItem -LiteralPath $stage -Recurse -Filter 'GarnetServer.exe' -File)
    if ($servers.Count -ne 1) { throw 'Pacote exige exatamente um GarnetServer.exe.' }
    $relative=$servers[0].FullName.Substring($stage.Length).TrimStart('\')
    $stagedWrapper=Join-Path $stage 'esterium-service-wrapper.exe'
    Copy-Item -LiteralPath $wrapper -Destination $stagedWrapper -ErrorAction Stop
    $dotnetStage=Join-Path $stage 'dotnet'
    if (Test-Path -LiteralPath $dotnetStage) {
        throw 'Pacote Garnet conflita com o destino reservado ao runtime .NET privado.'
    }
    Copy-Item -LiteralPath $dotnetRuntimeSource -Destination $dotnetStage -Recurse -ErrorAction Stop
    $stagedDotnet=Join-Path $dotnetStage 'dotnet.exe'
    $sourceRuntimeFiles=@(Get-ChildItem -LiteralPath $dotnetRuntimeSource -Recurse -File -Force)
    $stagedRuntimeFiles=@(Get-ChildItem -LiteralPath $dotnetStage -Recurse -File -Force)
    if ($sourceRuntimeFiles.Count -lt 5 -or $sourceRuntimeFiles.Count -ne $stagedRuntimeFiles.Count) {
        throw 'Runtime .NET privado nao foi copiado integralmente para Garnet.'
    }
    foreach ($sourceFile in $sourceRuntimeFiles) {
        $relativeRuntime=$sourceFile.FullName.Substring($dotnetRuntimeSource.Length).TrimStart('\\')
        $stagedFile=Join-Path $dotnetStage $relativeRuntime
        if (-not (Test-Path -LiteralPath $stagedFile -PathType Leaf) -or
            (Get-FileHash -LiteralPath $sourceFile.FullName -Algorithm SHA256).Hash -cne
            (Get-FileHash -LiteralPath $stagedFile -Algorithm SHA256).Hash) {
            throw 'Runtime .NET privado copiado para Garnet difere dos bytes aprovados.'
        }
    }
    if ($stagedRuntimeFiles.Count -lt 5 -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetStage 'LICENSE.txt') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetStage 'ThirdPartyNotices.txt') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetStage 'host\fxr\10.0.14\hostfxr.dll') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetStage 'shared\Microsoft.NETCore.App\10.0.14\System.Private.CoreLib.dll') -PathType Leaf)) {
        throw 'Runtime .NET privado nao foi copiado integralmente para Garnet.'
    }
    $stagedRuntimeList=@(& $stagedDotnet --list-runtimes)
    if ($LASTEXITCODE -ne 0 -or -not ($stagedRuntimeList -match 'Microsoft\.NETCore\.App 10\.0\.14')) {
        throw 'Runtime .NET copiado para Garnet nao iniciou de forma autonoma.'
    }
    if ((Get-FileHash -LiteralPath $stagedWrapper -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
        throw 'Wrapper copiado difere da versão aprovada.'
    }
    if (Test-Path -LiteralPath $garnetDir) { throw 'Outro instalador criou a pasta de destino.' }
    Move-Item -LiteralPath $stage -Destination $garnetDir -ErrorAction Stop
    $published=$true
    $serviceExe=Join-Path $garnetDir 'esterium-service-wrapper.exe'
    $garnetExe=Join-Path $garnetDir $relative
    $garnetDll=Join-Path (Split-Path -Parent $garnetExe) 'GarnetServer.dll'
    $runtimeConfig=Join-Path (Split-Path -Parent $garnetExe) 'GarnetServer.runtimeconfig.json'
    $dotnetExe=Join-Path $garnetDir 'dotnet\dotnet.exe'
    if (-not (Test-Path -LiteralPath $garnetDll -PathType Leaf) -or
        -not (Test-Path -LiteralPath $runtimeConfig -PathType Leaf) -or
        -not (Test-Path -LiteralPath $dotnetExe -PathType Leaf)) {
        throw 'Garnet assembly, runtime config ou runtime .NET local ausente.'
    }
    if ((Get-FileHash -LiteralPath $serviceExe -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
        throw 'Wrapper instalado difere do SHA aprovado.'
    }
    if ($null -ne (Get-Service -Name $GarnetServiceName -ErrorAction SilentlyContinue)) {
        throw 'Serviço Garnet apareceu durante a instalação.'
    }
    $appParameters='exec --runtimeconfig "'+$runtimeConfig+'" "'+$garnetDll+'" --config-import-path "'+$configPath+'"' 
    & $serviceExe install $GarnetServiceName $dotnetExe $appParameters | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'NSSM não registrou Garnet.' }
    $created=$true
    foreach ($command in @(
        @('set',$GarnetServiceName,'ObjectName','NT AUTHORITY\NetworkService'),
        @('set',$GarnetServiceName,'Start','SERVICE_DISABLED'),
        @('set',$GarnetServiceName,'AppDirectory',(Split-Path -Parent $garnetExe)),
        @('set',$GarnetServiceName,'AppExit','Default','Restart'),
        @('set',$GarnetServiceName,'AppRestartDelay','5000')
    )) {
        & $serviceExe @command | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Falha ao restringir o serviço Garnet.' }
    }
    $service=Get-CimInstance -ClassName Win32_Service -Filter "Name='$GarnetServiceName'"
    if ($null -eq $service -or
        $service.StartName -notin @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE') -or
        $service.StartMode -cne 'Disabled' -or $service.State -cne 'Stopped' -or
        [int]$service.ProcessId -ne 0) {
        throw 'Garnet nao esta desabilitado, parado e restrito a NETWORK SERVICE.'
    }
    Write-Output 'PASS 3/3: PostgreSQL instalado e Garnet registrado desabilitado e parado, sem ativar banco ou PDV.'
    Write-Output 'GATE: ainda são necessários TLS/SCRAM/HBA PostgreSQL, usuários SQL, migrações e testes reais de ACL/Lua.'
} catch {
    if ($created) {
        try {
            & (Join-Path $garnetDir 'esterium-service-wrapper.exe') remove $GarnetServiceName confirm | Out-Null
            if ($LASTEXITCODE -ne 0) { Write-Warning 'Rollback do serviço Garnet falhou; intervenção técnica necessária.' }
        } catch { Write-Warning 'Rollback do serviço Garnet falhou; intervenção técnica necessária.' }
    }
    throw
} finally {
    if (-not $published -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    }
}
