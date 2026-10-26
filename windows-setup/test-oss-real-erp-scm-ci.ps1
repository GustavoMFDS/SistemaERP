#requires -Version 5.1
<#
  Register REAL Esterium ERP Windows service from the exact bundle extracted
  by the MIT unsigned EXE, but ONLY on disposable GitHub-hosted CI. All
  credential files are throwaway fixtures, NOT a configured ERP installation.
  NEVER start the ERP service or enable fiscal/SEFAZ operations.
#>
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:ESTERIUM_SCM_DISPOSABLE_CI -ne '1' -or
    $env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or
    $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1') {
    throw 'Real unsigned ERP SCM test runs ONLY on disposable GitHub-hosted Windows CI.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'CI ERP SCM test requires Administrator.'
}
$source=Join-Path $env:ProgramFiles 'EsteriumOpenSourceStage'
$tools=Join-Path $source 'tools'
$registration=Join-Path $tools 'register-esterium-windows-service.ps1'
$verify=Join-Path $tools 'verify-esterium-bundle.ps1'
$guid=[Guid]::NewGuid().ToString('N')
$install=Join-Path $env:ProgramFiles ('EsteriumERPCI-'+$guid)
$temp=Join-Path $env:RUNNER_TEMP ('esterium-real-erp-scm-'+$guid)
$files=@{}
$registrationAttempted=$false
$serviceName='EsteriumERP'
if(-not(Test-Path -LiteralPath $registration -PathType Leaf) -or
   -not(Test-Path -LiteralPath (Join-Path $source 'esterium-server.exe') -PathType Leaf) -or
   -not(Test-Path -LiteralPath (Join-Path $source 'web\index.html') -PathType Leaf) -or
   (Test-Path -LiteralPath $install) -or
   $null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){
    throw 'Real native ERP bundle unavailable or CI service/install destination already exists.'
}
& $verify -BundleDir $source | Out-Null
if($LASTEXITCODE -ne 0){throw 'Pinned native ERP staging integrity failed.'}
try {
    [IO.Directory]::CreateDirectory($temp)|Out-Null
    $secretsDir=Join-Path $temp 'secrets'
    [IO.Directory]::CreateDirectory($secretsDir)|Out-Null
    foreach($name in @('database','redis','jwt','metrics')){
        $p=Join-Path $secretsDir ($name+'.secret')
        [IO.File]::WriteAllText($p,('CI-only-non-operational-'+$name+'-'+$guid),[Text.UTF8Encoding]::new($false))
        & icacls.exe $p '/inheritance:r' '/grant:r' '*S-1-5-19:R' '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
        if($LASTEXITCODE -ne 0){throw 'Cannot protect CI-only ERP credential fixture.'}
        $files[$name]=$p
    }
    $args=@{
        BundleDir=$source
        DatabaseURLFile=$files['database']
        RedisURLFile=$files['redis']
        JWTSecretFile=$files['jwt']
        MetricsBearerTokenFile=$files['metrics']
        AllowedOrigin='https://esterium.ci.invalid'
        ListenAddress='127.0.0.1:8080'
        InstallDir=$install
        UnsignedDisposableCIFixture=$true
    }
    # Customer mode must still require independent Authenticode approval.
    $unsafe=$args.Clone()
    $unsafe.Remove('UnsignedDisposableCIFixture')
    $rejected=$false
    try{& $registration @unsafe|Out-Null}catch{$rejected=$_.Exception.Message -match 'requires independently approved'}
    if(-not $rejected){throw 'Unsigned source was accepted by the signed customer registrar.'}
    $registrationAttempted=$true
    & $registration @args|Out-Null
    $scm=@(Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction Stop)
    if($scm.Count -ne 1 -or
       $scm[0].State -cne 'Stopped' -or
       $scm[0].StartMode -cne 'Manual' -or
       $scm[0].StartName -notin @('NT AUTHORITY\LocalService','NT AUTHORITY\LOCAL SERVICE') -or
       $scm[0].PathName -notlike ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')){
        throw 'CI ERP service is not manual/stopped, LocalService or the verified executable.'
    }
    & (Join-Path $install 'tools\verify-esterium-bundle.ps1') -BundleDir $install|Out-Null
    if($LASTEXITCODE -ne 0){throw 'Installed ERP bundle diverges from its own pinned manifest.'}
    if(@(Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue).Count -ne 0){
        throw 'CI service registerer unexpectedly started an ERP HTTP listener.'
    }
    $environment=@((Get-ItemProperty -LiteralPath ('HKLM:\SYSTEM\CurrentControlSet\Services\'+$serviceName) -Name Environment).Environment)
    if(@($environment|Where-Object {$_ -eq 'FISCAL_PROVIDER=disabled'}).Count -ne 1 -or
       @($environment|Where-Object {$_ -eq 'NFCE_SEFAZ_PRODUCTION_ENABLED=false'}).Count -ne 1 -or
       @($environment|Where-Object {$_ -eq 'NFCE_SEFAZ_HOMOLOGATION_ENABLED=false'}).Count -ne 1 -or
       @($environment|Where-Object {$_ -eq 'HTTP_ADDR=127.0.0.1:8080'}).Count -ne 1 -or
       ($environment -join ';') -match 'CI-only-non-operational-'){
       throw 'CI ERP SCM environment is not fiscal-disabled/loopback-only or exposed secrets.'
    }
    Write-Output 'PASS: REAL unsigned MIT ERP executable from native EXE stage registered in Windows SCM as stopped/manual LocalService.'
    Write-Output 'PASS: installed ERP manifest intact; only credential FILE PATHS in SCM; fiscal disabled, localhost only, no HTTP process.'
} finally {
    if($registrationAttempted) {
        $service=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
        if($null -ne $service) {
            if($service.PathName -notlike ('*'+[IO.Path]::Combine($install,'esterium-server.exe')+'*')) {
                throw 'Refuse to delete a Windows service not owned by the CI test installation.'
            }
            if($service.State -ne 'Stopped') {
                try {Stop-Service -Name $serviceName -Force -ErrorAction Stop}
                catch {throw 'Could not stop unexpected CI ERP service; inspect ephemeral runner.'}
            }
            & sc.exe delete $serviceName|Out-Null
            if($LASTEXITCODE -ne 0){throw 'Could not remove CI-owned ERP service.'}
            for($i=0;$i -lt 16;$i++){
                if($null -eq (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){break}
                Start-Sleep -Milliseconds 400
            }
            if($null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)){
                throw 'Disposable ERP service still registered after cleanup.'
            }
            Write-Output 'PASS: only the CI-owned ERP Windows service was deregistered.'
        }
    }
}
