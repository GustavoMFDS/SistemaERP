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
$testId=[Guid]::NewGuid().ToString('N').Substring(0,10)
$root=Join-Path $env:RUNNER_TEMP ('esterium-install-'+$testId)
$pg=Join-Path $root 'postgresql-16.exe'
$garnet=Join-Path $root 'garnet.zip'
$wrapper=Join-Path $root 'nssm.exe'
$options=Join-Path $root 'postgresql-unattended.options'
$config=Join-Path $root 'garnet.conf'
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
        'create_shortcuts=0'
        'unattendedmodeui=minimal'
    ) -join [Environment]::NewLine
    [IO.File]::WriteAllText($options,$optionsText+[Environment]::NewLine)
    [IO.File]::WriteAllText($config,'{"Address":"127.0.0.1","Port":64322,"EnableTLS":true,"AuthenticationMode":"ACL","EnableLua":true,"LuaTransactionMode":true}')
    & icacls.exe $options '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic PostgreSQL option ACL failed.' }
    & icacls.exe $config '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-20:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic Garnet ACL failed.' }
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
        GarnetServiceName=$serviceCache
        GarnetInstallDir=$cache
        PostgresPort=64321
        GarnetPort=64322
        SyntheticUnsignedCIFixture=$true
    }
    $env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI='1'
    $env:ESTERIUM_DEPENDENCY_INSTALL_DISPOSABLE_CI='1'
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
    Write-Output 'PASS: synthetic plan checks origin/ACL/ports, never executes vendor binaries, and rejects unsafe installation.'
} finally {
    Remove-Item Env:ESTERIUM_DEPENDENCY_PACKAGE_DISPOSABLE_CI -ErrorAction SilentlyContinue
    Remove-Item Env:ESTERIUM_DEPENDENCY_INSTALL_DISPOSABLE_CI -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
