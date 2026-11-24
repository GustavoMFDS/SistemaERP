#requires -Version 5.1
<#
  Instalação técnica offline das dependências em Windows novo.
  Pré-requisitos: metadados PostgreSQL (opções EDB ou diretório ZIP vazio)
  e configuração Garnet preparados com ACL restrita.
  Padrão: somente plano, sem executar programas. -Apply exige assinatura.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PostgresInstaller,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresSHA256,
    [string]$ExpectedPostgresPublisherThumbprint='',
    [Parameter(Mandatory=$true)][string]$GarnetPackage,
    [Parameter(Mandatory=$true)][string]$ExpectedGarnetSHA256,
    [Parameter(Mandatory=$true)][string]$ServiceWrapperExe,
    [Parameter(Mandatory=$true)][string]$ExpectedServiceWrapperSHA256,
    [string]$PostgresOptionsFile='',
    [string]$PostgresOptionsDir=(Join-Path $env:ProgramData 'Esterium\PostgresInstallOptions'),
    [string]$PostgresSuperPasswordFile=(Join-Path $env:ProgramData 'Esterium\Secrets\pg-super.secret'),
    [string]$PostgresServicePasswordFile=(Join-Path $env:ProgramData 'Esterium\Secrets\pg-service.secret'),
    [Parameter(Mandatory=$true)][string]$GarnetConfigFile,
    [string]$PostgresInstallDir=(Join-Path $env:ProgramFiles 'PostgreSQL\16'),
    [string]$PostgresDataDir=(Join-Path $env:ProgramData 'Esterium\PostgresData'),
    [string]$PostgresServiceName='EsteriumPostgreSQL16',
    [string]$PostgresServiceAccount='esterium_pgsvc',
    [string]$GarnetServiceName='EsteriumGarnet',
    [string]$GarnetInstallDir=(Join-Path $env:ProgramFiles 'EsteriumGarnet'),
    [string]$GarnetLogsDir=(Join-Path $env:ProgramData 'Esterium\GarnetLogs'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$TrustedPublisherThumbprint='',
    [switch]$SyntheticUnsignedCIFixture,
    [switch]$PostgresBinaryZip,
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
$optionsDir=[IO.Path]::GetFullPath($PostgresOptionsDir).TrimEnd('\')
$optionsBase=[IO.Path]::GetFullPath((Join-Path $env:ProgramData 'Esterium')).TrimEnd('\')
if(-not $optionsDir.StartsWith($optionsBase+'\',[StringComparison]::OrdinalIgnoreCase)){
    throw 'PostgreSQL first-install metadata must remain under ProgramData Esterium.'
}
if($optionsDir.IndexOf([char]34) -ge 0 -or $optionsDir.IndexOf([char]13) -ge 0 -or $optionsDir.IndexOf([char]10) -ge 0){
    throw 'PostgreSQL metadata path contains unsupported characters.'
}
$options=''
if(-not $PostgresBinaryZip){
    $options=Get-SafeFile $PostgresOptionsFile
    if(-not [string]::Equals([IO.Path]::GetDirectoryName($options),$optionsDir,[StringComparison]::OrdinalIgnoreCase)){
        throw 'EDB option file must be in its protected first-install directory.'
    }
}
$configPath=Get-SafeFile $GarnetConfigFile
if ((-not $PostgresBinaryZip -and [IO.Path]::GetExtension($pg).ToLowerInvariant() -ne '.exe') -or
    ($PostgresBinaryZip -and [IO.Path]::GetExtension($pg).ToLowerInvariant() -ne '.zip') -or
    [IO.Path]::GetExtension($zip).ToLowerInvariant() -ne '.zip' -or
    [IO.Path]::GetExtension($wrapper).ToLowerInvariant() -ne '.exe') {
    throw 'Pacotes exigem PostgreSQL no formato aprovado, Garnet .zip e wrapper Windows .exe.'
}
$pgDir=Assert-NewDestination $PostgresInstallDir $env:ProgramFiles
$pgData=Assert-NewDestination $PostgresDataDir (Join-Path $env:ProgramData 'Esterium')
$garnetDir=Assert-NewDestination $GarnetInstallDir $env:ProgramFiles
$garnetLogsDir=Assert-NewDestination $GarnetLogsDir (Join-Path $env:ProgramData 'Esterium')
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
if($PostgresBinaryZip){$packageArgs.PostgresBinaryZip=$true}
if ($SyntheticUnsignedCIFixture) { $packageArgs.SyntheticUnsignedCIFixture=$true }
& $checker @packageArgs | Out-Null
$allowedKeys=@('mode','prefix','datadir','serverport','servicename','serviceaccount',
    'superaccount','superpassword','servicepassword','enable-components','enable_acledit','create_shortcuts','unattendedmodeui')
$lines=@()
if(-not $PostgresBinaryZip){
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
if($PostgresBinaryZip){
    if(-not(Test-Path -LiteralPath $optionsDir -PathType Container) -or
       ((Get-Item -LiteralPath $optionsDir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
        throw 'PostgreSQL ZIP first-install metadata directory is missing or linked.'
    }
    $optionsAcl=Get-Acl -LiteralPath $optionsDir
    if(-not $optionsAcl.AreAccessRulesProtected){throw 'PostgreSQL marker directory ACL must be protected.'}
    foreach($ace in $optionsAcl.Access){
        if($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow){continue}
        $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if($sid -notin @('S-1-5-18','S-1-5-32-544')){
            throw 'PostgreSQL marker directory permits an unapproved reader.'
        }
    }
}else{
    Assert-Readers $options @('S-1-5-18','S-1-5-32-544') 'S-1-5-18'
}
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
$freshMarker=Join-Path $optionsDir ('PostgresFresh-'+$PostgresServiceName+'.marker')
if (Test-Path -LiteralPath $freshMarker) {
    throw 'Existing PostgreSQL fresh-cluster attestation: never reinstall automatically.'
}
if (-not $Apply) {
    Write-Output 'PLAN: pacotes, fontes, portas e configurações validados sem alteração no PC.'
    Write-Output ('NEXT: -Apply instalará PostgreSQL '+$(if($PostgresBinaryZip){'do ZIP oficial e criará conta/cluster'}else{'pelo EDB'})+' e registrará Garnet parado; banco/schema TLS comercial ainda não estão prontos.')
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
if(-not $PostgresBinaryZip){
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
}else{
    # The ZIP bytes and archive layout have just been rechecked against the
    # fixed official SHA-256. Stage under Program Files with a protected ACL
    # before extraction; preserve a published partial install for inspection.
    # Keep the temporary extraction root short to stay below legacy Windows
    # MAX_PATH after appending the vendor archive's relative paths.
    $pgStage=Join-Path ([IO.Path]::GetPathRoot($pgDir)) ('EsteriumPG-'+[Guid]::NewGuid().ToString('N'))
    $pgPublished=$false
    try{
        [IO.Directory]::CreateDirectory((Split-Path -Parent $pgDir))|Out-Null
        [IO.Directory]::CreateDirectory($pgStage)|Out-Null
        & icacls.exe $pgStage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F'|Out-Null
        if($LASTEXITCODE -ne 0){throw 'Não foi possível proteger a extração temporária PostgreSQL.'}
        if((Get-FileHash -LiteralPath $pg -Algorithm SHA256).Hash -cne
            '43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'){
            throw 'ZIP PostgreSQL mudou antes da extração.'
        }
        Expand-Archive -LiteralPath $pg -DestinationPath $pgStage -ErrorAction Stop
        foreach($entry in @(Get-ChildItem -LiteralPath $pgStage -Recurse -Force)){
            if(($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){
                throw 'ZIP PostgreSQL extraído contém link/junção proibido.'
            }
        }
        $roots=@(Get-ChildItem -LiteralPath $pgStage -Force)
        $stagedPg=Join-Path $pgStage 'pgsql'
        if($roots.Count -ne 1 -or $roots[0].Name -cne 'pgsql' -or -not $roots[0].PSIsContainer){
            throw 'ZIP PostgreSQL não contém a raiz binária esperada.'
        }
        foreach($relative in @('bin\postgres.exe','bin\initdb.exe','bin\pg_ctl.exe',
            'bin\psql.exe','bin\pg_isready.exe','bin\pg_config.exe')){
            $file=Join-Path $stagedPg $relative
            if(-not(Test-Path -LiteralPath $file -PathType Leaf) -or
               ((Get-Item -LiteralPath $file -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
                throw 'ZIP PostgreSQL não contém todos os binários necessários.'
            }
        }
        $pgVersion=@(& (Join-Path $stagedPg 'bin\pg_config.exe') '--version' 2>&1)
        if($LASTEXITCODE -ne 0 -or ($pgVersion -join ' ') -notmatch '^PostgreSQL 16\.15(?:\s|$)'){
            throw 'A versão dos binários PostgreSQL não corresponde ao ZIP aprovado.'
        }
        if(Test-Path -LiteralPath $pgDir){throw 'Destino PostgreSQL foi criado durante a validação; não sobrescrever.'}
        [IO.Directory]::Move($stagedPg,$pgDir)
        $pgPublished=$true
    }finally{
        if(-not $pgPublished -and (Test-Path -LiteralPath $pgStage)){
            if(((Get-Item -LiteralPath $pgStage -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -eq 0){
                Remove-Item -LiteralPath $pgStage -Recurse -Force -ErrorAction SilentlyContinue
            }
        }elseif(Test-Path -LiteralPath $pgStage){
            Remove-Item -LiteralPath $pgStage -Force -ErrorAction SilentlyContinue
        }
    }
    $serviceSecret=''
    $superPassword=''
    $serviceSecure=$null
    $credential=$null
    $serviceCreated=$false
    $passwordFile=Join-Path $optionsDir ('.initdb-'+[Guid]::NewGuid().ToString('N')+'.pw')
    try{
        foreach($secretPath in @($PostgresServicePasswordFile,$PostgresSuperPasswordFile)){
            if(-not [IO.Path]::IsPathRooted($secretPath) -or
               -not (Test-Path -LiteralPath $secretPath -PathType Leaf) -or
               ((Get-Item -LiteralPath $secretPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
                throw 'Segredo PostgreSQL local ausente ou não protegido.'
            }
            $secretAcl=Get-Acl -LiteralPath $secretPath
            if(-not $secretAcl.AreAccessRulesProtected){
                throw 'ACL do segredo PostgreSQL ainda herda permissões externas.'
            }
            foreach($ace in $secretAcl.Access){
                if($ace.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow){continue}
                $sid=$ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
                if($sid -notin @('S-1-5-18','S-1-5-32-544')){
                    throw 'Segredo PostgreSQL permite acesso por identidade não aprovada.'
                }
            }
        }
        $serviceSecret=[IO.File]::ReadAllText($PostgresServicePasswordFile).TrimEnd([char]13,[char]10)
        $superPassword=[IO.File]::ReadAllText($PostgresSuperPasswordFile).TrimEnd([char]13,[char]10)
        if($serviceSecret -cnotmatch '^Q9_z[A-Za-z0-9_-]{64}$' -or
           $superPassword -cnotmatch '^Q9_z[A-Za-z0-9_-]{64}$' -or
           $serviceSecret -ceq $superPassword){
            throw 'Segredos PostgreSQL não correspondem ao formato aleatório aprovado.'
        }
        $serviceSecure=ConvertTo-SecureString $serviceSecret -AsPlainText -Force
        $user=New-LocalUser -Name $PostgresServiceAccount -Password $serviceSecure `
            -Description 'Esterium PostgreSQL service identity' -AccountNeverExpires `
            -PasswordNeverExpires -UserMayNotChangePassword -ErrorAction Stop
        if($null -eq $user -or $null -eq $user.SID){throw 'Conta local de serviço PostgreSQL não foi criada.'}
        $serviceSid=[string]$user.SID.Value
        if($serviceSid -cnotmatch '^S-1-5-21-(\d+-){3}\d+$'){
            throw 'SID da conta PostgreSQL não é uma identidade local aprovada.'
        }
        $adminGroup=Get-LocalGroup -SID 'S-1-5-32-544' -ErrorAction Stop
        if(@(Get-LocalGroupMember -Group $adminGroup.Name -ErrorAction Stop | Where-Object {
            $_.SID.Value -ceq $serviceSid
        }).Count){throw 'Conta de serviço PostgreSQL não pode pertencer a Administradores.'}
        $credential=[PSCredential]::new(($env:COMPUTERNAME+'\'+$PostgresServiceAccount),$serviceSecure)

        $lsaSource=@'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
public static class EsteriumPostgresLsa {
    [StructLayout(LayoutKind.Sequential)] private struct OA { public uint Length; public IntPtr RootDirectory; public IntPtr ObjectName; public uint Attributes; public IntPtr SecurityDescriptor; public IntPtr SecurityQualityOfService; }
    [StructLayout(LayoutKind.Sequential)] private struct US { public ushort Length; public ushort MaximumLength; public IntPtr Buffer; }
    [DllImport("advapi32.dll")] private static extern uint LsaOpenPolicy(IntPtr systemName, ref OA attrs, uint access, out IntPtr handle);
    [DllImport("advapi32.dll")] private static extern uint LsaAddAccountRights(IntPtr policy, IntPtr sid, [In] US[] rights, uint count);
    [DllImport("advapi32.dll")] private static extern uint LsaEnumerateAccountRights(IntPtr policy, IntPtr sid, out IntPtr rights, out uint count);
    [DllImport("advapi32.dll")] private static extern uint LsaFreeMemory(IntPtr buffer);
    [DllImport("advapi32.dll")] private static extern uint LsaClose(IntPtr handle);
    [DllImport("advapi32.dll")] private static extern uint LsaNtStatusToWinError(uint status);
    private static IntPtr SidPtr(string value) { var sid=new SecurityIdentifier(value); var bytes=new byte[sid.BinaryLength]; sid.GetBinaryForm(bytes,0); var ptr=Marshal.AllocHGlobal(bytes.Length); Marshal.Copy(bytes,0,ptr,bytes.Length); Array.Clear(bytes,0,bytes.Length); return ptr; }
    public static void GrantServiceLogon(string value) {
        IntPtr sid=IntPtr.Zero, text=IntPtr.Zero, policy=IntPtr.Zero;
        try { sid=SidPtr(value); string right="SeServiceLogonRight"; text=Marshal.StringToHGlobalUni(right); var entry=new US { Length=(ushort)(right.Length*2), MaximumLength=(ushort)((right.Length+1)*2), Buffer=text }; var attrs=new OA(); attrs.Length=(uint)Marshal.SizeOf(typeof(OA)); uint status=LsaOpenPolicy(IntPtr.Zero,ref attrs,0x810,out policy); if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status)); status=LsaAddAccountRights(policy,sid,new US[]{entry},1); if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status)); }
        finally { if(policy!=IntPtr.Zero)LsaClose(policy); if(text!=IntPtr.Zero)Marshal.FreeHGlobal(text); if(sid!=IntPtr.Zero)Marshal.FreeHGlobal(sid); }
    }
    public static bool HasRight(string value,string expected) {
        IntPtr sid=IntPtr.Zero, policy=IntPtr.Zero, rights=IntPtr.Zero;
        try { sid=SidPtr(value); var attrs=new OA(); attrs.Length=(uint)Marshal.SizeOf(typeof(OA)); uint status=LsaOpenPolicy(IntPtr.Zero,ref attrs,0x800,out policy); if(status!=0) throw new Win32Exception((int)LsaNtStatusToWinError(status)); uint count=0; status=LsaEnumerateAccountRights(policy,sid,out rights,out count); if(status==0xC0000034)return false; if(status!=0)throw new Win32Exception((int)LsaNtStatusToWinError(status)); int size=Marshal.SizeOf(typeof(US)); for(uint i=0;i<count;i++){var entry=(US)Marshal.PtrToStructure(IntPtr.Add(rights,checked((int)i*size)),typeof(US)); if(String.Equals(Marshal.PtrToStringUni(entry.Buffer,entry.Length/2),expected,StringComparison.OrdinalIgnoreCase))return true;} return false; }
        finally { if(rights!=IntPtr.Zero)LsaFreeMemory(rights); if(policy!=IntPtr.Zero)LsaClose(policy); if(sid!=IntPtr.Zero)Marshal.FreeHGlobal(sid); }
    }
}
'@
        if($null -eq ([Management.Automation.PSTypeName]'EsteriumPostgresLsa').Type){
            Add-Type -TypeDefinition $lsaSource -ErrorAction Stop
        }
        [EsteriumPostgresLsa]::GrantServiceLogon($serviceSid)
        if(-not [EsteriumPostgresLsa]::HasRight($serviceSid,'SeServiceLogonRight') -or
           [EsteriumPostgresLsa]::HasRight($serviceSid,'SeDenyServiceLogonRight')){
            throw 'Direito local de serviço PostgreSQL não pôde ser verificado com segurança.'
        }
        if(Test-Path -LiteralPath $pgData){throw 'Pasta de dados PostgreSQL foi criada durante a validação; não adotar.'}
        [IO.Directory]::CreateDirectory($pgData)|Out-Null
        & icacls.exe $pgData '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' ('*'+$serviceSid+':(OI)(CI)M')|Out-Null
        if($LASTEXITCODE -ne 0){throw 'ACL privada do cluster PostgreSQL não pôde ser aplicada.'}
        & icacls.exe $pgDir '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F'|Out-Null
        if($LASTEXITCODE -ne 0){throw 'ACL base dos binários PostgreSQL não pôde ser protegida.'}
        & icacls.exe $pgDir '/grant:r' ('*'+$serviceSid+':(OI)(CI)RX') '/T' '/C'|Out-Null
        if($LASTEXITCODE -ne 0){throw 'Conta PostgreSQL não recebeu somente leitura/execução dos binários.'}
        $pwBytes=[Text.UTF8Encoding]::new($false).GetBytes($superPassword+[Environment]::NewLine)
        try{
            $pwStream=[IO.File]::Open($passwordFile,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
            try{$pwStream.Write($pwBytes,0,$pwBytes.Length);$pwStream.Flush($true)}finally{$pwStream.Dispose()}
        }finally{[Array]::Clear($pwBytes,0,$pwBytes.Length)}
        & icacls.exe $passwordFile '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' ('*'+$serviceSid+':R')|Out-Null
        if($LASTEXITCODE -ne 0){throw 'Arquivo temporário de senha initdb não pôde ser protegido.'}
        $initdb=Join-Path $pgDir 'bin\initdb.exe'
        $initArgs='-D '+('"'+$pgData+'"')+' -U postgres --auth-local=scram-sha-256 --auth-host=scram-sha-256 --encoding=UTF8 --pwfile '+('"'+$passwordFile+'"')+' --no-instructions'
        $initOut=Join-Path $optionsDir 'postgres-initdb.stdout.log'
        $initErr=Join-Path $optionsDir 'postgres-initdb.stderr.log'
        $initProc=Start-Process -FilePath $initdb -Credential $credential -ArgumentList $initArgs `
            -Wait -PassThru -RedirectStandardOutput $initOut -RedirectStandardError $initErr -ErrorAction Stop
        if($null -eq $initProc -or $initProc.ExitCode -ne 0){throw 'PostgreSQL initdb não concluiu; dados parciais exigem inspeção manual.'}
        $initProc.Dispose()
        if(-not(Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION') -PathType Leaf) -or
           ([IO.File]::ReadAllText((Join-Path $pgData 'PG_VERSION'))).Trim() -cne '16'){
            throw 'initdb não confirmou um cluster PostgreSQL 16.'
        }
        $serviceCommand='"'+(Join-Path $pgDir 'bin\pg_ctl.exe')+'" runservice -N "'+$PostgresServiceName+'" -D "'+$pgData+'" -w'
        New-Service -Name $PostgresServiceName -BinaryPathName $serviceCommand -Credential $credential `
            -StartupType Disabled -DisplayName 'Esterium PostgreSQL 16' -ErrorAction Stop|Out-Null
        $serviceCreated=$true
    }finally{
        $serviceSecret=$null
        $superPassword=$null
        $serviceSecure=$null
        if(Test-Path -LiteralPath $passwordFile){Remove-Item -LiteralPath $passwordFile -Force -ErrorAction Stop}
        $newService=if($serviceCreated){Get-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue}else{$null}
        if($null -ne $newService){
            try{
                Set-Service -Name $PostgresServiceName -StartupType Disabled -ErrorAction Stop
                if($newService.Status -ne 'Stopped'){
                    Stop-Service -Name $PostgresServiceName -ErrorAction Stop
                    (Get-Service -Name $PostgresServiceName -ErrorAction Stop).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
            }catch{throw 'Falha ao proteger PostgreSQL parcial: serviço deve permanecer desabilitado e parado.'}
        }
    }
    $credential=$null
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
if($PostgresBinaryZip){
    $expectedServiceCommand='"'+(Join-Path $pgDir 'bin\pg_ctl.exe')+'" runservice -N "'+$PostgresServiceName+'" -D "'+$pgData+'" -w'
    if($pgService[0].PathName -cne $expectedServiceCommand){
        throw 'Serviço PostgreSQL ZIP não aponta exatamente aos binários e dados aprovados.'
    }
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
Write-Output ('PASS 1/3: PostgreSQL 16 '+$(if($PostgresBinaryZip){'ZIP oficial instalado com conta local dedicada'}else{'EDB instalado'})+'; serviço desabilitado e parado para configuração TLS/SCRAM.')
Write-Output '2/3: instalando binários e serviço Garnet...'
$stage=$garnetDir+'.staging-'+[Guid]::NewGuid().ToString('N')
$published=$false
$registrationAttempted=$false
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
    $serverAssemblies=@(Get-ChildItem -LiteralPath $stage -Recurse -Filter 'GarnetServer.dll' -File)
    if($serverAssemblies.Count -gt 1){throw 'Pacote Garnet contém mais de uma assembly GarnetServer.dll.'}
    $frameworkDependent=$false
    $relativeAssembly=''
    if($serverAssemblies.Count -eq 1){
        $assemblyDir=$serverAssemblies[0].DirectoryName
        $hasRuntimeConfig=Test-Path -LiteralPath (Join-Path $assemblyDir 'GarnetServer.runtimeconfig.json') -PathType Leaf
        $hasDeps=Test-Path -LiteralPath (Join-Path $assemblyDir 'GarnetServer.deps.json') -PathType Leaf
        if(-not $hasRuntimeConfig -or -not $hasDeps){
            throw 'Pacote Garnet contém assembly framework-dependent incompleta.'
        }
        $frameworkDependent=$true
        $relativeAssembly=$serverAssemblies[0].FullName.Substring($stage.Length).TrimStart('\')
    }elseif(@(Get-ChildItem -LiteralPath $stage -Recurse -File | Where-Object {$_.Name -in @('GarnetServer.runtimeconfig.json','GarnetServer.deps.json')}).Count -gt 0){
        throw 'Pacote Garnet contém metadados framework-dependent sem GarnetServer.dll.'
    }
    $stagedWrapper=Join-Path $stage 'EsteriumGarnet.exe'
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
    $serviceExe=Join-Path $garnetDir 'EsteriumGarnet.exe'
    $garnetExe=Join-Path $garnetDir $relative
    $garnetServerDll=Join-Path $garnetDir $relativeAssembly
    $dotnetRuntimeDir=Join-Path $garnetDir 'dotnet'
    $dotnetExe=Join-Path $dotnetRuntimeDir 'dotnet.exe'
    if (-not (Test-Path -LiteralPath $garnetExe -PathType Leaf) -or
        -not (Test-Path -LiteralPath $dotnetExe -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetRuntimeDir 'host\fxr\10.0.14\hostfxr.dll') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path $dotnetRuntimeDir 'shared\Microsoft.NETCore.App\10.0.14\coreclr.dll') -PathType Leaf)) {
        throw 'Garnet apphost ou runtime .NET privado ausente.'
    }
    if($frameworkDependent -and (
        -not (Test-Path -LiteralPath $garnetServerDll -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path (Split-Path -Parent $garnetServerDll) 'GarnetServer.runtimeconfig.json') -PathType Leaf) -or
        -not (Test-Path -LiteralPath (Join-Path (Split-Path -Parent $garnetServerDll) 'GarnetServer.deps.json') -PathType Leaf))){
        throw 'Garnet framework-dependent assembly metadata changed after extraction.'
    }
    if ((Get-FileHash -LiteralPath $serviceExe -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
        throw 'Wrapper instalado difere do SHA aprovado.'
    }
    if ($null -ne (Get-Service -Name $GarnetServiceName -ErrorAction SilentlyContinue)) {
        throw 'Serviço Garnet apareceu durante a instalação.'
    }
    $launchExecutable=$garnetExe
    $launchArguments=@('--config-import-path',$configPath)
    if($frameworkDependent){
        $launchExecutable=$dotnetExe
        $launchArguments=@($garnetServerDll,'--config-import-path',$configPath)
    }
    $quotePowerShellLiteral={param([string]$value) return ("'"+$value.Replace("'","''")+"'")}
    $launcherArgumentText=(@($launchArguments | ForEach-Object {& $quotePowerShellLiteral ([string]$_)}) -join ',')
    $launcherLines=@(
        '$ErrorActionPreference = ''Stop''',
        ('$env:DOTNET_ROOT_X64 = '+(& $quotePowerShellLiteral $dotnetRuntimeDir)),
        ('$env:DOTNET_ROOT = '+(& $quotePowerShellLiteral $dotnetRuntimeDir)),
        '$env:DOTNET_MULTILEVEL_LOOKUP = ''0''',
        ('$launchArguments = @('+$launcherArgumentText+')'),
        ('& '+(& $quotePowerShellLiteral $launchExecutable)+' @launchArguments'),
        'exit $LASTEXITCODE'
    )
    $launcherScript=Join-Path $garnetDir 'esterium-garnet-launcher.ps1'
    [IO.File]::WriteAllText($launcherScript,(($launcherLines -join [Environment]::NewLine)+[Environment]::NewLine),[Text.UTF8Encoding]::new($true))
    $serviceApplication=Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    if(-not (Test-Path -LiteralPath $serviceApplication -PathType Leaf)){
        throw 'Windows PowerShell launcher is unavailable for the private Garnet runtime.'
    }
    $appParameters='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$launcherScript+'"'
    if (Test-Path -LiteralPath $garnetLogsDir) { throw 'Garnet log directory appeared during setup; refuse overwrite.' }
    New-Item -ItemType Directory -Path $garnetLogsDir -ErrorAction Stop | Out-Null
    & icacls.exe $garnetLogsDir '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)M' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Não foi possível proteger a pasta de logs Garnet.' }
    $xmlName=[Security.SecurityElement]::Escape($GarnetServiceName)
    $xmlExe=[Security.SecurityElement]::Escape($serviceApplication)
    $xmlArguments=[Security.SecurityElement]::Escape($appParameters)
    $xmlWorkingDirectory=[Security.SecurityElement]::Escape($garnetDir)
    $xmlLogs=[Security.SecurityElement]::Escape($garnetLogsDir)
    $serviceConfig=@"
<?xml version="1.0" encoding="utf-8"?>
<service>
  <id>$xmlName</id>
  <name>Esterium Garnet</name>
  <description>Esterium local cache service</description>
  <executable>$xmlExe</executable>
  <arguments>$xmlArguments</arguments>
  <workingdirectory>$xmlWorkingDirectory</workingdirectory>
  <startmode>Manual</startmode>
  <stoptimeout>30sec</stoptimeout>
  <onfailure action="restart" delay="5sec" />
  <serviceaccount><domain>NT AUTHORITY</domain><user>NetworkService</user></serviceaccount>
  <logpath>$xmlLogs</logpath>
  <log mode="roll" />
</service>
"@
    $serviceConfigPath=Join-Path $garnetDir 'EsteriumGarnet.xml'
    [IO.File]::WriteAllText($serviceConfigPath,$serviceConfig,[Text.UTF8Encoding]::new($false))
    # Arm ownership-safe rollback before install: WinSW may register the SCM
    # entry and still return an error, so cleanup must inspect PathName either way.
    $registrationAttempted=$true
    & $serviceExe install | Out-Null
    $installExit=$LASTEXITCODE
    $service=Get-CimInstance -ClassName Win32_Service -Filter "Name='$GarnetServiceName'" -ErrorAction SilentlyContinue
    $serviceOwned=$null -ne $service -and $service.PathName -and
        $service.PathName.IndexOf($serviceExe,[StringComparison]::OrdinalIgnoreCase) -ge 0
    if ($installExit -ne 0 -or -not $serviceOwned) { throw 'WinSW não registrou Garnet.' }
    Set-Service -Name $GarnetServiceName -StartupType Disabled -ErrorAction Stop
    $service=Get-CimInstance -ClassName Win32_Service -Filter "Name='$GarnetServiceName'"
    if ($null -eq $service -or
        $service.StartName -notin @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE') -or
        $service.StartMode -cne 'Disabled' -or $service.State -cne 'Stopped' -or
        [int]$service.ProcessId -ne 0 -or
        $service.PathName.IndexOf($serviceExe,[StringComparison]::OrdinalIgnoreCase) -lt 0) {
        throw 'Garnet não está desabilitado, parado e restrito a NETWORK SERVICE via WinSW.'
    }
    Write-Output 'PASS 3/3: PostgreSQL instalado e Garnet registrado desabilitado e parado, sem ativar banco ou PDV.'
    Write-Output 'GATE: ainda são necessários TLS/SCRAM/HBA PostgreSQL, usuários SQL, migrações e testes reais de ACL/Lua.'
} catch {
    if ($registrationAttempted) {
        try {
            $serviceExe=Join-Path $garnetDir 'EsteriumGarnet.exe'
            $service=Get-CimInstance -ClassName Win32_Service -Filter "Name='$GarnetServiceName'" -ErrorAction SilentlyContinue
            if ($null -ne $service -and $service.PathName -and
                $service.PathName.IndexOf($serviceExe,[StringComparison]::OrdinalIgnoreCase) -ge 0) {
                Set-Service -Name $GarnetServiceName -StartupType Disabled -ErrorAction Stop
                $registered=Get-Service -Name $GarnetServiceName -ErrorAction Stop
                if ($registered.Status -ne 'Stopped') {
                    Stop-Service -Name $GarnetServiceName -Force -ErrorAction Stop
                    (Get-Service -Name $GarnetServiceName -ErrorAction Stop).WaitForStatus(
                        [ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
                }
                & $serviceExe uninstall | Out-Null
                if ($LASTEXITCODE -ne 0) { Write-Warning 'Rollback do serviço Garnet via WinSW falhou; intervenção técnica necessária.' }
            }
        } catch { Write-Warning 'Rollback do serviço Garnet falhou; intervenção técnica necessária.' }
    }
    throw
} finally {
    if (-not $published -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    }
}
