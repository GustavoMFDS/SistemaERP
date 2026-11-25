#requires -Version 5.1
<#
  Assisted inspection of an incomplete, default-path Windows installation.
  PLAN is read-only. Inspect temporarily starts ONLY the approved PostgreSQL
  service. ResumeAdmin explicitly permits only the first-admin transaction;
  neither mode creates readiness receipts, firewall rules or activates ERP.
  A fresh snapshot/backup and independently approved signed diagnostic media
  are operator prerequisites. Never use on a live store.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$OriginalManifestSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{40}$')][string]$OriginalPublisher,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$DiagnosticSHA256,
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Fa-f0-9]{40}$')][string]$DiagnosticPublisher,
    [switch]$Inspect,
    [switch]$ResumeAdmin,
    [ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$BootstrapScriptSHA256
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($Inspect -and $ResumeAdmin){throw 'Choose exactly one operation.'}
if($ResumeAdmin -and -not $BootstrapScriptSHA256){throw 'Approved bootstrap script digest required.'}
function Assert-Physical([string]$Path) {
    $full=[IO.Path]::GetFullPath($Path)
    if ($full.StartsWith('\\')) { throw 'Network paths are prohibited.' }
    $cursor=$full
    while($cursor){
        $item=Get-Item -LiteralPath $cursor -Force
        if($item.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Reparse points are prohibited.'}
        $parent=Split-Path -Parent $cursor
        if($parent -eq $cursor){break}
        $cursor=$parent
    }
    return $full
}
function Assert-Signed([string]$Path,[string]$Publisher) {
    $signature=Get-AuthenticodeSignature -LiteralPath $Path
    if($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint -ine $Publisher){throw 'Approved signature required.'}
}
function Get-ExactService([string]$Name) {
    $services=@(Get-CimInstance Win32_Service -Filter "Name='$Name'")
    if($services.Count -ne 1){throw 'Expected unique service registration.'}
    return $services[0]
}
function Assert-Disabled([string]$Name) {
    $s=Get-ExactService $Name
    if($s.State -cne 'Stopped' -or $s.StartMode -cne 'Disabled' -or [int]$s.ProcessId -ne 0){
        throw 'Incomplete installation must remain stopped and disabled.'
    }
}
function Assert-NoListeners {
    $ports=@(5432,6379,8443)
    if(@(Get-NetTCPConnection -State Listen -ErrorAction Stop | Where-Object LocalPort -in $ports).Count){
        throw 'A managed port is already occupied.'
    }
}
if($env:OS -ne 'Windows_NT'){throw 'Windows required.'}
$machine=Get-CimInstance Win32_ComputerSystem
if (($machine.Manufacturer+' '+$machine.Model) -notmatch '(?i)VirtualBox|innotek') {throw 'Diagnostic rehearsal is restricted to a VirtualBox guest.'}
$principal=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Elevated operator required.'}
$diagnosticDir=Assert-Physical (Join-Path $env:ProgramFiles 'EsteriumRecoveryDiagnostics')
if([IO.Path]::GetFullPath($PSScriptRoot) -ine $diagnosticDir){throw 'Run from dedicated diagnostic directory.'}
$exe=Assert-Physical (Join-Path $diagnosticDir 'esterium-bootstrap-admin.exe')
$script=Assert-Physical $PSCommandPath
$protectedPaths=@($diagnosticDir,$exe,$script)
$bootstrap=$null
if($ResumeAdmin){
    $bootstrap=Assert-Physical (Join-Path $diagnosticDir 'bootstrap-esterium-first-admin.ps1')
    Assert-Signed $bootstrap $DiagnosticPublisher
    if((Get-FileHash -LiteralPath $bootstrap -Algorithm SHA256).Hash -ine $BootstrapScriptSHA256){throw 'Bootstrap script digest mismatch.'}
    $protectedPaths+=$bootstrap
}
# Reject writable diagnostic media for non-administrative identities.
foreach($path in $protectedPaths){
    foreach($ace in (Get-Acl -LiteralPath $path).Access){
        $writeMask=[Security.AccessControl.FileSystemRights]::Write -bor [Security.AccessControl.FileSystemRights]::Delete -bor [Security.AccessControl.FileSystemRights]::ChangePermissions -bor [Security.AccessControl.FileSystemRights]::TakeOwnership
        if($ace.AccessControlType -eq 'Allow' -and ($ace.FileSystemRights -band $writeMask)){
            if($ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin @('S-1-5-18','S-1-5-32-544')){throw 'Diagnostic media must be administrator-protected.'}
        }
    }
}
Assert-Signed $script $DiagnosticPublisher
Assert-Signed $exe $DiagnosticPublisher
if((Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash -ine $DiagnosticSHA256){throw 'Diagnostic digest mismatch.'}
$bundle=Assert-Physical (Join-Path $env:ProgramFiles 'EsteriumERP')
$gate=Assert-Physical (Join-Path $bundle 'tools\check-esterium-signed-release.ps1')
$verify=Assert-Physical (Join-Path $bundle 'tools\verify-esterium-bundle.ps1')
$policy=Assert-Physical (Join-Path $bundle 'tools\check-esterium-postgres-tls-policy.ps1')
foreach($file in @($gate,$verify,$policy)){Assert-Signed $file $OriginalPublisher}
& $gate -BundleDir $bundle -ExpectedManifestSHA256 $OriginalManifestSHA256 -ExpectedPublisherThumbprint $OriginalPublisher | Out-Null
$base=Assert-Physical (Join-Path $env:ProgramData 'Esterium')
foreach($receipt in @('FirstRunReadiness','Installation')){
    if(Test-Path -LiteralPath (Join-Path $base $receipt)){throw 'Unexpected readiness/activation evidence; manual review required.'}
}
$data=Assert-Physical (Join-Path $base 'PostgresData')
$bin=Assert-Physical (Join-Path $env:ProgramFiles 'PostgreSQL\16\bin')
$ctl=Assert-Physical (Join-Path $bin 'pg_ctl.exe')
$postgres=Assert-Physical (Join-Path $bin 'postgres.exe')
$version=Assert-Physical (Join-Path $data 'PG_VERSION')
if([IO.File]::ReadAllText($version).Trim() -cne '16'){throw 'PostgreSQL 16 required.'}
$options=Assert-Physical (Join-Path $base 'PostgresInstallOptions')
$marker=Assert-Physical (Join-Path $options 'PostgresFresh-EsteriumPostgreSQL16.marker')
$tlsReceipt=Assert-Physical (Join-Path $options 'PostgresTlsApplied-EsteriumPostgreSQL16.receipt')
$proof=[IO.File]::ReadAllLines($marker)
if($proof.Count -ne 3 -or $proof[0] -cne 'ESTERIUM_FRESH_POSTGRES_16' -or
    $proof[1] -cne 'EsteriumPostgreSQL16' -or $proof[2] -ine $data -or
    -not [IO.File]::ReadAllText($tlsReceipt).StartsWith('TLS_POLICY_STAGED_NEVER_STARTED')){
    throw 'Missing or inconsistent original cluster evidence.'
}
$conf=Assert-Physical (Join-Path $data 'postgresql.conf')
$hba=Assert-Physical (Join-Path $data 'pg_hba.conf')
& $policy -PostgresqlConf $conf -PgHbaConf $hba | Out-Null
# Reject configuration relocation/overrides rather than trusting a partial
# parse. The original signed policy already rejects includes and auto.conf.
foreach($line in [IO.File]::ReadAllLines($conf)){
    if($line -match '^\s*(data_directory|config_file|external_pid_file|ident_file|include(?:_dir|_if_exists)?)\b'){
        throw 'Unexpected PostgreSQL configuration relocation.'
    }
}
$portLines=@([IO.File]::ReadAllLines($conf) | Where-Object {$_ -match '^\s*port\b'})
if($portLines.Count -gt 1 -or ($portLines.Count -eq 1 -and $portLines[0] -notmatch '^\s*port\s*=\s*5432\s*(?:#.*)?$')){
    throw 'Unexpected PostgreSQL port.'
}
$urlFile=Assert-Physical (Join-Path $base 'PostgresApplication\database-url.secret')
foreach($ace in (Get-Acl -LiteralPath $urlFile).Access){
    if($ace.AccessControlType -eq 'Allow' -and ($ace.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData)){
        if($ace.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin @('S-1-5-18','S-1-5-32-544','S-1-5-19')){
            throw 'Unexpected database URL reader.'
        }
    }
}
foreach($name in @('EsteriumERP','EsteriumGarnet','EsteriumPostgreSQL16')){Assert-Disabled $name}
$pg=Get-ExactService 'EsteriumPostgreSQL16'
$expected='"'+$ctl+'" runservice -N "EsteriumPostgreSQL16" -D "'+$data+'" -w'
if($pg.PathName -ine $expected -or $pg.StartName -notin @('esterium_pgsvc','.\esterium_pgsvc',($env:COMPUTERNAME+'\esterium_pgsvc'))){
    throw 'PostgreSQL service does not match the approved default EDB registration.'
}
Assert-NoListeners
if(-not $Inspect -and -not $ResumeAdmin){
    Write-Output 'PLAN: approved diagnostic, original bundle, cluster, TLS policy and stopped services verified.'
    Write-Output 'PLAN: no services or data changed; inspection requires separate explicit opt-in.'
    return
}
# Keep ERP/Garnet disabled. No automatic retry, no activation receipt.
$completed=$false
$result='ADMIN_INDETERMINATE'
$failureStage='ADMIN_INDETERMINATE'
try {
    Set-Service EsteriumPostgreSQL16 -StartupType Manual
    Start-Service EsteriumPostgreSQL16
    (Get-Service EsteriumPostgreSQL16).WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Running,[TimeSpan]::FromSeconds(45))
    $listeners=@(Get-NetTCPConnection -State Listen -ErrorAction Stop | Where-Object LocalPort -in @(5432,6379,8443))
    if($listeners.Count -lt 1){throw 'PostgreSQL listener not found.'}
    $pg=Get-ExactService 'EsteriumPostgreSQL16'
    foreach($listener in $listeners){
        if($listener.LocalPort -ne 5432 -or $listener.LocalAddress -notin @('127.0.0.1','::1')){throw 'Unexpected listener.'}
        $proc=Get-CimInstance Win32_Process -Filter ("ProcessId="+[int]$listener.OwningProcess)
        if($null -eq $proc -or $proc.ExecutablePath -ine $postgres -or [int]$proc.ParentProcessId -ne [int]$pg.ProcessId){
            throw 'Listener ownership could not be proven.'
        }
    }
    Assert-Disabled 'EsteriumERP'
    Assert-Disabled 'EsteriumGarnet'
    $output=@(& $exe --inspect-status --database-url-file $urlFile 2>&1)
    if($LASTEXITCODE -ne 0 -or $output.Count -ne 1 -or [string]$output[0] -cnotin @('ADMIN_ABSENT','ADMIN_COMMITTED','ADMIN_INDETERMINATE')){
        throw 'Diagnostic did not return an approved status.'
    }
    $result=[string]$output[0]
    if($ResumeAdmin){
        $failureStage='RESUME_REFUSED_NOT_ABSENT'
        if($result -cne 'ADMIN_ABSENT'){throw 'First administrator is not absent.'}
        # Create rechecks the fresh-schema/business-data invariants within
        # its serializable transaction; the inspection alone is insufficient.
        $failureStage='RESUME_BOOTSTRAP_FAILED'
        & $bootstrap -DatabaseURLFile $urlFile -BootstrapAdminExe $exe -TrustedPublisherThumbprint $DiagnosticPublisher -Apply | Out-Null
        $failureStage='RESUME_COMMIT_UNCONFIRMED'
        $after=@(& $exe --inspect-status --database-url-file $urlFile 2>&1)
        if($LASTEXITCODE -ne 0 -or $after.Count -ne 1 -or [string]$after[0] -cne 'ADMIN_COMMITTED'){
            throw 'First administrator commit could not be verified.'
        }
        $result='ADMIN_COMMITTED'
    }
    $completed=$true
} catch {
    # Expose only exact, fixed validation labels; never arbitrary error text.
    if($failureStage -eq 'RESUME_BOOTSTRAP_FAILED'){
        switch -CaseSensitive ($_.Exception.Message){
            'As senhas nao coincidem.' {$failureStage='RESUME_PASSWORD_MISMATCH'}
            'Senha fora dos limites do bootstrap.' {$failureStage='RESUME_PASSWORD_LENGTH'}
            'Senha deve ocupar uma unica linha.' {$failureStage='RESUME_PASSWORD_FORMAT'}
            'Bootstrap recusado: FAIL: first administrator was not created: invalid CNPJ' {$failureStage='RESUME_INVALID_CNPJ'}
            'Bootstrap recusado: FAIL: first administrator was not created: invalid administrator email' {$failureStage='RESUME_INVALID_EMAIL'}
            'Bootstrap recusado: FAIL: first administrator was not created: database contains business data; first-admin setup refused' {$failureStage='RESUME_NONEMPTY_DATABASE'}
            'Bootstrap recusado: FAIL: first administrator was not created: password must be 12-72 UTF-8 bytes and at least 12 characters' {$failureStage='RESUME_PASSWORD_LENGTH'}
        }
    }
    $completed=$false
} finally {
    try {
        Set-Service EsteriumPostgreSQL16 -StartupType Disabled
        if((Get-Service EsteriumPostgreSQL16).Status -ne 'Stopped'){
            Stop-Service EsteriumPostgreSQL16
            (Get-Service EsteriumPostgreSQL16).WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped,[TimeSpan]::FromSeconds(45))
        }
        foreach($name in @('EsteriumERP','EsteriumGarnet','EsteriumPostgreSQL16')){Assert-Disabled $name}
        Assert-NoListeners
    } catch {throw 'RECOVERY_CLEANUP_FAILED: manual intervention required; no activation allowed.'}
}
if(-not $completed){throw ($failureStage+': operation failed; services verified stopped and disabled. Re-inspect before any retry.')}
Write-Output $result
Write-Output 'PASS: all three services stopped and disabled; no recovery approval granted.'
