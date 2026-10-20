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
    [string]$GarnetServiceName='EsteriumGarnet',
    [string]$GarnetInstallDir=(Join-Path $env:ProgramFiles 'EsteriumGarnet'),
    [ValidateRange(1,65535)][int]$PostgresPort=5432,
    [ValidateRange(1,65535)][int]$GarnetPort=6379,
    [string]$TrustedPublisherThumbprint='',
    [switch]$Apply
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Instalador exige Windows.' }
if ($PostgresPort -in @(8080,$GarnetPort) -or $GarnetPort -eq 8080) {
    throw 'Portas da aplicação, banco e cache devem ser distintas.'
}
foreach ($name in @($PostgresServiceName,$GarnetServiceName)) {
    if ($name -cnotmatch '^[A-Za-z][A-Za-z0-9_-]{2,63}$' -or $name -eq 'EsteriumERP') {
        throw 'Nome inválido de serviço Windows.'
    }
}
if ($PostgresServiceName -eq $GarnetServiceName) { throw 'Serviços não podem compartilhar nome.' }
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
}
& $checker -PostgresInstaller $pg -ExpectedPostgresSHA256 $ExpectedPostgresSHA256 -ExpectedPostgresPublisherThumbprint $ExpectedPostgresPublisherThumbprint -GarnetPackage $zip -ExpectedGarnetSHA256 $ExpectedGarnetSHA256 | Out-Null
$allowedKeys=@('mode','prefix','datadir','serverport','servicename','serviceaccount',
    'superaccount','superpassword','servicepassword','enable-components','create_shortcuts','unattendedmodeui')
$lines=@([IO.File]::ReadAllLines($options))
$seen=@{}
foreach ($line in $lines) {
    if ($line -notmatch '^([a-z-]+)=(.*)$') { throw 'Opções EDB inválidas.' }
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
    'enable-components'='server,commandlinetools';'create_shortcuts'='0'
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
if (-not $Apply) {
    Write-Output 'PLAN: pacotes, fontes, portas e configurações validados sem alteração no PC.'
    Write-Output 'NEXT: -Apply instalará EDB silenciosamente e registrará Garnet parado (ainda sem banco/schema TLS comercial).'
    return
}
Write-Output '1/3: instalando PostgreSQL 16 sem interface...'
try {
    # Passwords stay inside the protected option file; never pass in argv.
    $proc=Start-Process -FilePath $pg -ArgumentList @('--mode','unattended','--unattendedmodeui','none','--optionfile',('"' + $options + '"')) -Wait -PassThru
} finally {
    # EDB reads plaintext bootstrap passwords. Do not retain another copy.
    if (Test-Path -LiteralPath $options) {
        Remove-Item -LiteralPath $options -Force -ErrorAction Stop
    }
}
if ($proc.ExitCode -ne 0) {
    throw ('EDB retornou '+$proc.ExitCode+'. Possível instalação parcial; preservar dados e investigar.')
}
if (-not (Test-Path -LiteralPath (Join-Path $pgDir 'bin\pg_isready.exe')) -or
    -not (Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION')) -or
    ([IO.File]::ReadAllText((Join-Path $pgData 'PG_VERSION')).Trim() -cne '16') -or
    $null -eq (Get-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue)) {
    throw 'PostgreSQL não confirmou serviço, binários e cluster versão 16.'
}
foreach ($listener in @(Get-NetTCPConnection -LocalPort $PostgresPort -State Listen -ErrorAction SilentlyContinue)) {
    if ([string]$listener.LocalAddress -notin @('127.0.0.1','::1')) {
        Stop-Service -Name $PostgresServiceName -ErrorAction SilentlyContinue
        throw 'PostgreSQL abriu porta fora do computador; serviço interrompido.'
    }
}
Write-Output '2/3: instalando binários e serviço Garnet...'
$stage=$garnetDir+'.staging-'+[Guid]::NewGuid().ToString('N')
$published=$false
$created=$false
try {
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash -cne $ExpectedGarnetSHA256.ToUpperInvariant()) {
        throw 'Arquivo Garnet mudou após a validação.'
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $stage -ErrorAction Stop
    $servers=@(Get-ChildItem -LiteralPath $stage -Recurse -Filter 'GarnetServer.exe' -File)
    if ($servers.Count -ne 1) { throw 'Pacote exige exatamente um GarnetServer.exe.' }
    $relative=$servers[0].FullName.Substring($stage.Length).TrimStart('\')
    $stagedWrapper=Join-Path $stage 'esterium-service-wrapper.exe'
    Copy-Item -LiteralPath $wrapper -Destination $stagedWrapper -ErrorAction Stop
    if ((Get-FileHash -LiteralPath $stagedWrapper -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
        throw 'Wrapper copiado difere da versão aprovada.'
    }
    & icacls.exe $stage '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Não foi possível proteger pasta Garnet.' }
    if (Test-Path -LiteralPath $garnetDir) { throw 'Outro instalador criou a pasta de destino.' }
    Move-Item -LiteralPath $stage -Destination $garnetDir -ErrorAction Stop
    $published=$true
    $serviceExe=Join-Path $garnetDir 'esterium-service-wrapper.exe'
    $garnetExe=Join-Path $garnetDir $relative
    if ((Get-FileHash -LiteralPath $serviceExe -Algorithm SHA256).Hash -cne $ExpectedServiceWrapperSHA256.ToUpperInvariant()) {
        throw 'Wrapper instalado difere do SHA aprovado.'
    }
    if ($null -ne (Get-Service -Name $GarnetServiceName -ErrorAction SilentlyContinue)) {
        throw 'Serviço Garnet apareceu durante a instalação.'
    }
    & $serviceExe install $GarnetServiceName $garnetExe '--config-import-path' $configPath | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'NSSM não registrou Garnet.' }
    $created=$true
    foreach ($command in @(
        @('set',$GarnetServiceName,'ObjectName','NT AUTHORITY\NetworkService'),
        @('set',$GarnetServiceName,'Start','SERVICE_DEMAND_START'),
        @('set',$GarnetServiceName,'AppDirectory',(Split-Path -Parent $garnetExe)),
        @('set',$GarnetServiceName,'AppExit','Default','Restart'),
        @('set',$GarnetServiceName,'AppRestartDelay','5000')
    )) {
        & $serviceExe @command | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Falha ao restringir o serviço Garnet.' }
    }
    $service=Get-CimInstance -ClassName Win32_Service -Filter "Name='$GarnetServiceName'"
    if ($null -eq $service -or $service.StartName -notin @('NT AUTHORITY\NetworkService','NT AUTHORITY\NETWORK SERVICE')) {
        throw 'Garnet não está sob conta NETWORK SERVICE.'
    }
    Write-Output 'PASS 3/3: PostgreSQL instalado e Garnet registrado parado, sem ativar banco ou PDV.'
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
