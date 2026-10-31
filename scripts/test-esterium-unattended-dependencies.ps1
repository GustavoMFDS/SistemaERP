#requires -Version 5.1
# Disposable Windows CI: fixture never executes an EDB/Garnet/NSSM binary.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Only the disposable hosted Windows runner may execute these fixtures.'
}
$tool=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
$tokens=$null
$parseErrors=$null
[Management.Automation.Language.Parser]::ParseFile($tool,[ref]$tokens,[ref]$parseErrors) | Out-Null
if (@($parseErrors).Count -gt 0) {
    $details=(@($parseErrors) | ForEach-Object {
        'line '+$_.Extent.StartLineNumber+': '+$_.Message
    }) -join '; '
    throw ('Unattended install script PowerShell parser failed: '+$details)
}
$implementation=[IO.File]::ReadAllText($tool)
if (-not $implementation.Contains('Set-Service -Name $PostgresServiceName -StartupType Disabled') -or
    -not $implementation.Contains("'Start','SERVICE_DISABLED'")) {
    throw 'Unattended dependency installer must leave PostgreSQL and Garnet disabled.'
}
$runtimeLaunchTokens=@(
    '$dotnetRuntimeSource=Join-Path $PSScriptRoot ''dotnet''',
    '$dotnetRuntimeDir=Join-Path $garnetDir ''dotnet''',
    '$garnetServerDll=Join-Path $garnetDir $relativeAssembly',
    'GarnetServer.runtimeconfig.json',
    'GarnetServer.deps.json',
    '& $serviceExe install $GarnetServiceName $dotnetExe $appParameters',
    'AppEnvironmentExtra',
    'DOTNET_ROOT_X64=',
    'DOTNET_ROOT=',
    'DOTNET_MULTILEVEL_LOOKUP=0'
)
foreach($token in $runtimeLaunchTokens){
    if(-not $implementation.Contains($token)){
        throw 'Garnet installer must launch through the package-local pinned .NET runtime.'
    }
}
$testId=[Guid]::NewGuid().ToString('N').Substring(0,10)
$root=Join-Path $env:RUNNER_TEMP ('esterium-install-'+$testId)
$pg=Join-Path $root 'postgresql-16.exe'
$garnet=Join-Path $root 'garnet.zip'
$wrapper=Join-Path $root 'nssm.exe'
$options=Join-Path $root 'postgresql-unattended.options'
$config=Join-Path $root 'garnet.conf'
$aclFile=Join-Path $root 'garnet.acl'
$pfxFile=Join-Path $root 'garnet.pfx'
$unpacked=Join-Path $root 'source'
$pgDir=Join-Path $env:ProgramFiles ('PostgreSQL\16-fixture-'+$testId)
$data=Join-Path $env:ProgramData ('Esterium\pg-fixture-'+$testId)
$cache=Join-Path $env:ProgramFiles ('EsteriumGarnet-fixture-'+$testId)
$servicePg='EstPG'+$testId
$serviceCache='EstGC'+$testId
try {
    [IO.Directory]::CreateDirectory($unpacked) | Out-Null
    [IO.File]::WriteAllText($pg,'fake EDB installer never executed')
    [IO.File]::WriteAllText($wrapper,'fake NSSM never executed')
    [IO.File]::WriteAllText((Join-Path $unpacked 'GarnetServer.exe'),'fake Garnet server never executed')
    [IO.File]::WriteAllText((Join-Path $unpacked 'GarnetServer.dll'),'fake Garnet assembly never executed')
    [IO.File]::WriteAllText((Join-Path $unpacked 'GarnetServer.runtimeconfig.json'),'{}')
    [IO.File]::WriteAllText((Join-Path $unpacked 'GarnetServer.deps.json'),'{}')
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::CreateFromDirectory($unpacked,$garnet)
    $optionsText=@(
        'mode=unattended'
        'prefix='+$pgDir
        'datadir='+$data
        'serverport=64321'
        'servicename='+$servicePg
        'serviceaccount=ci_fixture_pg'
        'superaccount=postgres'
        'superpassword=synthetic-only-do-not-use'
        'servicepassword=synthetic-only-do-not-use-2'
        'enable-components=server,commandlinetools'
        'enable_acledit=1'
        'create_shortcuts=0'
        'unattendedmodeui=minimal'
    ) -join [Environment]::NewLine
    [IO.File]::WriteAllText($options,$optionsText+[Environment]::NewLine)
    [IO.File]::WriteAllText($aclFile,('user default off'+[Environment]::NewLine+'user esteriumcache on >synthetic-strong-test ~* +@all'))
    [IO.File]::WriteAllText($pfxFile,'synthetic-only-nonexecuted-PFX')
    $state=Join-Path $root 'state'
    $checkpoints=Join-Path $state 'checkpoints'
    [IO.Directory]::CreateDirectory($checkpoints) | Out-Null
    foreach ($dir in @($state,$checkpoints)) {
        & icacls.exe $dir '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-20:(OI)(CI)M' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Synthetic AOF storage ACL failed.' }
    }
    $garnetConfig=[ordered]@{
        Address='127.0.0.1';Port=64322;EnableCluster=$false
        AuthenticationMode='ACL';AclFile=$aclFile
        EnableTLS=$true;ClientCertificateRequired=$false;CertFileName=$pfxFile;CertPassword='synthetic-only'
        EnableLua=$true;LuaTransactionMode=$true
        LuaMemoryManagementMode='Tracked';LuaScriptMemoryLimit='8m';LuaScriptTimeoutMs=5000
        EnableAOF=$true;WaitForCommit=$true;CommitFrequencyMs=0
        Recover=$true;FailOnRecoveryError=$true;UseAofNullDevice=$false
        CheckpointDir=$checkpoints
    }
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    & icacls.exe $options '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic PostgreSQL option ACL failed.' }
    & icacls.exe $config '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-20:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic Garnet ACL failed.' }
    foreach ($item in @($aclFile,$pfxFile)) {
        & icacls.exe $item '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-20:R' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Synthetic Garnet source ACL failed.' }
    }
    $values=@{
        PostgresInstaller=$pg
        ExpectedPostgresSHA256=(Get-FileHash $pg -Algorithm SHA256).Hash
        ExpectedPostgresPublisherThumbprint=('A'*40)
        GarnetPackage=$garnet
        ExpectedGarnetSHA256=(Get-FileHash $garnet -Algorithm SHA256).Hash
        ServiceWrapperExe=$wrapper
        ExpectedServiceWrapperSHA256=(Get-FileHash $wrapper -Algorithm SHA256).Hash
        PostgresOptionsFile=$options
        GarnetConfigFile=$config
        PostgresInstallDir=$pgDir
        PostgresDataDir=$data
        PostgresServiceName=$servicePg
        PostgresServiceAccount='ci_fixture_pg'
        GarnetServiceName=$serviceCache
        GarnetInstallDir=$cache
        PostgresPort=64321
        GarnetPort=64322
        SyntheticUnsignedCIFixture=$true
    }
    $env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI='1'
    $env:ESTERIUM_DEPENDENCY_INSTALL_DISPOSABLE_CI='1'
    $env:ESTERIUM_SETUP_PLAN_DISPOSABLE_CI='1'
    $setup=Join-Path $PSScriptRoot 'setup-esterium-local-dependencies.ps1'
    $setupArgs=@{
        PostgresInstaller=$pg
        ExpectedPostgresSHA256=(Get-FileHash $pg -Algorithm SHA256).Hash
        ExpectedPostgresPublisherThumbprint=('A'*40)
        GarnetPackage=$garnet
        ExpectedGarnetSHA256=(Get-FileHash $garnet -Algorithm SHA256).Hash
        ServiceWrapperExe=$wrapper
        ExpectedServiceWrapperSHA256=(Get-FileHash $wrapper -Algorithm SHA256).Hash
        BootstrapDir=(Join-Path $env:ProgramData ('Esterium\BootstrapCI-'+$testId))
        PostgresOptionsDir=(Join-Path $env:ProgramData ('Esterium\OptionsCI-'+$testId))
        GarnetConfigDir=(Join-Path $env:ProgramData ('Esterium\GarnetCI-'+$testId))
        PostgresInstallDir=$pgDir
        PostgresDataDir=$data
        GarnetInstallDir=$cache
        PostgresServiceName=$servicePg
        GarnetServiceName=$serviceCache
        PostgresPort=64321
        GarnetPort=64322
        SyntheticPlanCIFixture=$true
    }
    $source=Get-Content -LiteralPath $setup -Raw
    $tokens=$null
    $errors=$null
    [Management.Automation.Language.Parser]::ParseInput($source,[ref]$tokens,[ref]$errors) | Out-Null
    if (@($errors).Count -gt 0) {
        throw ('Coordinator PowerShell syntax rejected: ' + $errors[0].Message)
    }
    $setupPlan=@(& $setup @setupArgs) -join ' '
    if ($setupPlan -notmatch 'PLAN: nenhum segredo' -or
        (Test-Path -LiteralPath $setupArgs.BootstrapDir) -or
        (Test-Path -LiteralPath $setupArgs.PostgresOptionsDir)) {
        throw 'Unified offline coordinator plan modified the host.'
    }
    $setupArgs.Apply=$true
    $reject=''
    try { & $setup @setupArgs | Out-Null } catch { $reject=$_.Exception.Message }
    if ($reject -notmatch 'Teste sem assinatura restrito') {
        throw 'Unified coordinator incorrectly allowed unsigned Apply.'
    }
    $setupArgs.Remove('Apply')
    $ok=@(& $tool @values) -join ' '
    if ($ok -notmatch 'PLAN: pacotes, fontes, portas e configurações validados') {
        throw 'Offline dependency plan did not complete.'
    }
    if ((Test-Path $pgDir) -or (Test-Path $data) -or (Test-Path $cache) -or
        $null -ne (Get-Service -Name $servicePg -ErrorAction SilentlyContinue) -or
        $null -ne (Get-Service -Name $serviceCache -ErrorAction SilentlyContinue)) {
        throw 'Read-only plan created executable files, data or services.'
    }
    function Assert-Reject([string]$label,[string]$expected) {
        $actual='(accepted)'
        try { & $tool @values | Out-Null } catch { $actual=$_.Exception.Message }
        if ($actual -notmatch $expected) { throw "Expected rejection for $label; received $actual" }
    }
    $values.Apply=$true
    Assert-Reject 'unsigned apply even on CI' 'Fixture sem assinatura somente em planejamento'
    $values.Remove('Apply')
    $values.ExpectedServiceWrapperSHA256='0'*64
    Assert-Reject 'unapproved service wrapper' 'Wrapper de serviço não corresponde'
    $values.ExpectedServiceWrapperSHA256=(Get-FileHash $wrapper -Algorithm SHA256).Hash
    $values.GarnetPort=64321
    Assert-Reject 'conflicting ports' 'Portas da aplicação'
    $values.GarnetPort=64322
    [IO.File]::AppendAllText($options,'other=evil'+[Environment]::NewLine)
    Assert-Reject 'extra EDB option' 'Opções EDB duplicadas ou extras'
    [IO.File]::WriteAllText($options,$optionsText+[Environment]::NewLine)
    [IO.File]::WriteAllText($options,($optionsText.Replace('serviceaccount=ci_fixture_pg','serviceaccount=LocalSystem')+[Environment]::NewLine))
    Assert-Reject 'service account overwritten by privileged identity' 'Opções EDB não correspondem'
    [IO.File]::WriteAllText($options,($optionsText.Replace('enable_acledit=1','enable_acledit=0')+[Environment]::NewLine))
    Assert-Reject 'vendor ACL editing disabled for Windows service account' 'Opções EDB não correspondem'
    [IO.File]::WriteAllText($options,$optionsText+[Environment]::NewLine)
    $garnetConfig.EnableTLS=$false
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'TLS disabled in protected Garnet config' 'Garnet exige loopback'
    $garnetConfig.EnableTLS=$true
    $garnetConfig.LuaScriptMemoryLimit='unlimited'
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'unlimited Lua memory in Garnet config' 'perfil protegido'
    $garnetConfig.LuaScriptMemoryLimit='8m'
    $garnetConfig.AclFile=$options
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'Garnet ACL replaced by unreadable PostgreSQL secret' 'acesso esperado'
    $garnetConfig.AclFile=$aclFile
    $garnetConfig.WaitForCommit=$false
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'AOF write acknowledgment disabled' 'perfil protegido'
    $garnetConfig.WaitForCommit=$true
    $garnetConfig.CheckpointDir=$root
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'checkpoint escapes state directory' 'Pasta de checkpoints AOF'
    $garnetConfig.CheckpointDir=$checkpoints
    $garnetConfig.EnableAOF=$false
    [IO.File]::WriteAllText($config,($garnetConfig | ConvertTo-Json))
    Assert-Reject 'AOF explicitly disabled' 'perfil protegido'
    Write-Output 'PASS: synthetic plan checks origin/ACL/ports, never executes vendor binaries, and rejects unsafe installation.'
} finally {
    Remove-Item Env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_DEPENDENCY_INSTALL_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_SETUP_PLAN_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
