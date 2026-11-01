#requires -Version 5.1
<#
 Actual protected ProgramData layout + real PostgreSQL 16 smoke.
 GitHub-hosted disposable Windows CI ONLY. No customer installer execution.
#>
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$IntegratedExe)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or
   $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
   $env:ESTERIUM_PG_ZIP_DISPOSABLE_CI -ne '1' -or
   $env:ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI -ne '1' -or
   $env:ESTERIUM_NATIVE_DATA_DISPOSABLE_CI -ne '1'){
    throw 'Native protected ProgramData SQL test runs ONLY on disposable GitHub-hosted Windows.'
}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'CI native data test requires Administrator.'
}
$exe=[IO.Path]::GetFullPath($IntegratedExe)
$runner=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\')+'\'
if(-not $exe.StartsWith($runner,[StringComparison]::OrdinalIgnoreCase) -or
   (Split-Path -Leaf $exe) -cne 'EsteriumERP-Setup-TECHNICAL-PREVIEW.exe' -or
   -not(Test-Path -LiteralPath $exe -PathType Leaf) -or
   ((Get-Item -LiteralPath $exe -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){
    throw 'EXE must be a real signed-off test fixture under runner temporary files.'
}
$layout=Join-Path $env:ProgramData 'Esterium'
$dataParent=Join-Path $layout 'PostgresData'
$data=Join-Path $dataParent 'cluster'
$backup=Join-Path $layout 'Backups\disposable-recovery.dump'
$marker=Join-Path $layout 'oss-data-layout.json'
$pgRoot=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin\pgsql'
$binParent=Join-Path $env:ProgramFiles 'EsteriumOpenSourcePostgresBin'
if(-not(Test-Path -LiteralPath $marker -PathType Leaf) -or
   -not(Test-Path -LiteralPath (Join-Path $pgRoot 'bin\initdb.exe') -PathType Leaf) -or
   (Test-Path -LiteralPath $data) -or (Test-Path -LiteralPath $backup)){
    throw 'Stage must contain only intact vendor bytes and EMPTY native PostgreSQL data layout.'
}
$journalHash=(Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash
$journal=Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json -ErrorAction Stop
if($journal.schemaVersion -ne 1 -or $journal.phase -cne 'DATA_DIRECTORIES_ONLY' -or
   $journal.databaseInitialized -ne $false -or $journal.servicesInstalled -ne $false -or
   $journal.backupsConfigured -ne $false){throw 'Unexpected native stage journal.'}
$rootAcl=Get-Acl -LiteralPath $layout
if(-not $rootAcl.AreAccessRulesProtected){throw 'Native stage must keep protected ACL.'}
$bad=@($rootAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) | Where-Object {
    $_.AccessControlType -eq 'Allow' -and
    $_.IdentityReference.Value -notin @('S-1-5-18','S-1-5-32-544')
})
if($bad.Count){throw 'Native stage has an unapproved access rule.'}
foreach($leaf in @('PostgresData','GarnetData','Secrets','Backups')){
    $dir=Join-Path $layout $leaf
    if(-not(Test-Path -LiteralPath $dir -PathType Container) -or
       ((Get-Item -LiteralPath $dir -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -or
       @(Get-ChildItem -LiteralPath $dir -Force).Count -ne 0){
        throw ('Native stage must be empty before test: '+$leaf)
    }
}
$audited=Start-Process -FilePath $exe -ArgumentList '--audit-oss-postgres' -Wait -PassThru
if($null -eq $audited -or $audited.ExitCode -ne 0){
    throw 'Refuse unverified vendor executable before native protected initdb.'
}
$name='epg'+[Guid]::NewGuid().ToString('N').Substring(0,12)
$work=Join-Path $env:RUNNER_TEMP ('esterium-native-ci-'+[Guid]::NewGuid().ToString('N'))
$pw=Join-Path $work 'pg-password'
$out=Join-Path $work 'initdb.out'
$err=Join-Path $work 'initdb.err'
$created=$false
$grantedData=$false
$grantedBin=$false
$ownsCluster=$false
$sid=''
$proc=$null
$password=$null
$secure=$null
try{
    $bytes=New-Object byte[] 48
    $rng=[Security.Cryptography.RandomNumberGenerator]::Create()
    try{
        $rng.GetBytes($bytes)
        $password='Q9_z'+[Convert]::ToBase64String($bytes).Replace('+','-').Replace('/','_').TrimEnd('=')
    }finally{$rng.Dispose();[Array]::Clear($bytes,0,$bytes.Length)}
    $secure=ConvertTo-SecureString $password -AsPlainText -Force
    $user=New-LocalUser -Name $name -Password $secure -Description 'Disposable native PG CI'
    if($null -eq $user -or $null -eq $user.SID){throw 'Could not create ephemeral CI user.'}
    $sid=[string]$user.SID.Value
    $created=$true
    if(@(Get-LocalGroupMember -Group 'Administrators' | Where-Object {
        $_.SID.Value -eq $sid
    }).Count){throw 'Ephemeral PostgreSQL account unexpectedly holds admin role.'}
    [IO.Directory]::CreateDirectory($work)|Out-Null
    & icacls.exe $work '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' ('*'+$sid+':(OI)(CI)M') | Out-Null
    if($LASTEXITCODE){throw 'Could not secure throwaway CI secret folder.'}
    [IO.File]::WriteAllText($pw,$password+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
    & icacls.exe $pw '/inheritance:r' '/grant:r' '*S-1-5-18:F' '*S-1-5-32-544:F' ('*'+$sid+':R')|Out-Null
    if($LASTEXITCODE){throw 'Cannot secure ephemeral pg password file.'}
    & icacls.exe $dataParent '/grant' ('*'+$sid+':(OI)(CI)M')|Out-Null
    if($LASTEXITCODE){throw 'Cannot grant only the temporary CI user PostgresData access.'}
    $grantedData=$true
    & icacls.exe $binParent '/grant' ('*'+$sid+':(OI)(CI)RX')|Out-Null
    if($LASTEXITCODE){throw 'Cannot grant only the temporary CI user vendor executable RX.'}
    $grantedBin=$true
    $credential=[PSCredential]::new(($env:COMPUTERNAME+'\'+$name),$secure)
    $password=$null
    $argsLine='-D "'+$data+'" -U postgres --auth-local=scram-sha-256 --auth-host=scram-sha-256 --encoding=UTF8 --pwfile "'+$pw+'" --no-instructions'
    if(Test-Path -LiteralPath $data){throw 'Refuse preexisting PostgreSQL cluster.'}
    $ownsCluster=$true
    $startParams=@{FilePath=(Join-Path $pgRoot 'bin\initdb.exe');Credential=$credential;ArgumentList=$argsLine;
        Wait=$true;PassThru=$true;RedirectStandardOutput=$out;RedirectStandardError=$err}
    $proc=Start-Process @startParams
    if($null -eq $proc -or $proc.ExitCode -ne 0){throw 'Native ProgramData initdb failed.'}
    if(-not(Test-Path -LiteralPath (Join-Path $data 'PG_VERSION') -PathType Leaf) -or
       ([IO.File]::ReadAllText((Join-Path $data 'PG_VERSION'))).Trim() -cne '16'){
        throw 'Native ProgramData cluster is incomplete.'
    }
    $serviceParams=@{VerifiedPgRoot=$pgRoot;DataDir=$data;ServiceCredential=$credential;
        ExerciseLoopback=$true;FromIntegratedStage=$true;NativeDataLayout=$true}
    & (Join-Path $PSScriptRoot 'test-postgres-oss-service-ci.ps1') @serviceParams
    if(-not(Test-Path -LiteralPath $backup -PathType Leaf) -or
       (Get-Item -LiteralPath $backup).Length -lt 128){
        throw 'Backup archive missing in native protected Backups folder.'
    }
    Write-Output 'PASS: native EXE-protected ProgramData PostgreSQL cluster, non-admin service, SQL and backup/restore.'
}finally{
    $password=$null
    $secure=$null
    if($null -ne $proc){$proc.Dispose()}
    if($created){
        $existing=@(Get-CimInstance Win32_Service -ErrorAction Stop | Where-Object {
            $_.Name -like 'EsteriumCIPG*' -and
            $_.StartName -match ('\\'+[Regex]::Escape($name)+'$')
        })
        if($existing.Count){throw 'Refuse cleanup while owned PostgreSQL test service remains registered.'}
    }
    if($ownsCluster -and
       $data -ceq (Join-Path $env:ProgramData 'Esterium\PostgresData\cluster')){
        if(Test-Path -LiteralPath $data){Remove-Item -LiteralPath $data -Recurse -Force -ErrorAction Stop}
        if(Test-Path -LiteralPath $backup){Remove-Item -LiteralPath $backup -Force -ErrorAction Stop}
    }
    if($grantedData){
        & icacls.exe $dataParent '/remove:g' ('*'+$sid)|Out-Null
        if($LASTEXITCODE){throw 'Failed to revoke native data CI permission.'}
    }
    if($grantedBin){
        & icacls.exe $binParent '/remove:g' ('*'+$sid)|Out-Null
        if($LASTEXITCODE){throw 'Failed to revoke vendor binary CI permission.'}
    }
    if($created){Remove-LocalUser -Name $name -ErrorAction Stop}
    if(Test-Path -LiteralPath $work){Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction Stop}
    if((Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash -cne $journalHash){
        throw 'Native layout phase marker was altered by CI database test.'
    }
    foreach($leaf in @('PostgresData','GarnetData','Secrets','Backups')){
        if(@(Get-ChildItem -LiteralPath (Join-Path $layout $leaf) -Force).Count){
            throw ('Native stage contains leftover CI data: '+$leaf)
        }
    }
    Write-Output 'PASS: native ProgramData empty protected layout restored after CI test; no account/service/backup left.'
}
