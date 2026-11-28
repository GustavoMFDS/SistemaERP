# Native NTFS fixture: only a new, uniquely named empty tree under a scratch root.
# The current operator SID is a surrogate for the service SID, not proof of
# a dedicated PostgreSQL account or effective cross-account denial.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted'){
    throw 'Native ACL modification fixture requires disposable hosted Windows CI.'
}
if(-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){
    throw 'Native ACL fixture requires elevation.'
}
$installer=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($installer,[ref]$t,[ref]$e)
if($e.Count){throw 'Installer AST failed.'}
foreach($name in @('Test-PostgresDataDescriptor','New-FreshPostgresDataDescriptor','Initialize-FreshPostgresAclNative','New-PrivateFreshPostgresDirectory','Assert-FreshPostgresDataAcl')){
    $defs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name},$true))
    if($defs.Count -ne 1){throw 'Missing native fixture function.'}
    . ([scriptblock]::Create($defs[0].Extent.Text))
}
$root=Join-Path $env:RUNNER_TEMP ('esterium-pg-acl-native-'+[Guid]::NewGuid().ToString('N'))
$svc=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
if($svc -cnotmatch '^S-1-5-21-(\d+-){3}\d+$'){throw 'Fixture requires a local or domain user SID.'}
if(Test-Path -LiteralPath $root){throw 'Scratch path exists.'}
[IO.Directory]::CreateDirectory($root)|Out-Null
$child=Join-Path $root 'base'
$file=Join-Path $child 'fixture.txt'
function Set-FixtureAcl([string]$Path,[bool]$Directory,[switch]$UnexpectedAce){
    $raw=New-FreshPostgresDataDescriptor $svc $Directory
    if($UnexpectedAce){
        $ace=[Security.AccessControl.CommonAce]::new([Security.AccessControl.AceFlags]::None,[Security.AccessControl.AceQualifier]::AccessAllowed,0x120089,[Security.Principal.SecurityIdentifier]::new('S-1-5-21-111-222-333-1002'),$false,$null)
        $raw.DiscretionaryAcl.InsertAce($raw.DiscretionaryAcl.Count,$ace)
    }
    $bytes=New-Object byte[] $raw.BinaryLength
    $raw.GetBinaryForm($bytes,0)
    $acl=Get-Acl -LiteralPath $Path
    $acl.SetSecurityDescriptorBinaryForm($bytes,([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Access))
    Set-Acl -LiteralPath $Path -AclObject $acl
}
Set-FixtureAcl $root $true
[IO.Directory]::CreateDirectory($child)|Out-Null
[IO.File]::WriteAllText($file,'synthetic ACL fixture only')
Set-FixtureAcl $child $true
Set-FixtureAcl $file $false
Assert-FreshPostgresDataAcl $root $svc
Write-Output 'PASS: native NTFS tree accepted.'
Set-FixtureAcl $file $false -UnexpectedAce
$rejected=$false
try{Assert-FreshPostgresDataAcl $root $svc | Out-Null}catch{if($_.Exception.Message -match 'unexpected-principal'){$rejected=$true}else{throw}}
if(-not $rejected){throw 'Native NTFS unexpected principal accepted.'}
Write-Output 'PASS: actual NTFS unexpected ACE rejected.'
Set-FixtureAcl $file $false
Assert-FreshPostgresDataAcl $root $svc
Write-Output 'PASS: native fixture restored to expected policy.'
Write-Output ('Fixture retained for inspection: '+$root)
# Test the actual handle-bound primitive separately from the production path.
Initialize-FreshPostgresAclNative
$nativeRoot=Join-Path $env:RUNNER_TEMP ('esterium-pg-handle-native-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($nativeRoot)|Out-Null
$nativeFile=Join-Path $nativeRoot 'fixture.txt'
[IO.File]::WriteAllText($nativeFile,'synthetic handle fixture')
$handle=[EsteriumFreshPgAclNative]::Open($nativeFile,$false,$true)
try {
    $identity=[EsteriumFreshPgAclNative]::Identity($handle,$false)
    $raw=New-FreshPostgresDataDescriptor $svc $false
    $bytes=New-Object byte[] $raw.BinaryLength
    $raw.GetBinaryForm($bytes,0)
    [EsteriumFreshPgAclNative]::Apply($handle,$false,$bytes)
    if([EsteriumFreshPgAclNative]::Identity($handle,$false) -cne $identity){throw 'Handle identity changed.'}
    $actual=[Security.AccessControl.RawSecurityDescriptor]::new((Get-Acl -LiteralPath $nativeFile).GetSecurityDescriptorBinaryForm(),0)
    if((Test-PostgresDataDescriptor $actual $svc $false $false) -cne 'pass'){throw 'Native handle ACL failed.'}
    Write-Output 'PASS: actual retained-handle ACL mutation and identity validation.'
}finally{$handle.Dispose()}
$linked=Join-Path $nativeRoot 'linked.txt'
$null=New-Item -ItemType HardLink -Path $linked -Target $nativeFile -ErrorAction Stop
$rejected=$false
try{
    $handle=[EsteriumFreshPgAclNative]::Open($nativeFile,$false,$true)
    $handle.Dispose()
}catch{
    if($_.Exception.ToString() -match 'Non-single-link file'){$rejected=$true}else{throw}
}
if(-not $rejected){throw 'Native handle accepted multiple hard links.'}
Write-Output 'PASS: actual NTFS hardlink rejected before mutation.'
$privateRoot=Join-Path $env:RUNNER_TEMP ('esterium-pg-private-native-'+[Guid]::NewGuid().ToString('N'))
$privateIdentity=New-PrivateFreshPostgresDirectory $privateRoot
$before=(Get-Acl -LiteralPath $privateRoot).Sddl
$duplicateRejected=$false
try{$null=New-PrivateFreshPostgresDirectory $privateRoot}catch{if($_.Exception.ToString() -match 'already exists|183'){$duplicateRejected=$true}else{throw}}
if(-not $duplicateRejected -or (Get-Acl -LiteralPath $privateRoot).Sddl -cne $before){throw 'Duplicate directory was not refused without modification.'}
$privateHandle=[EsteriumFreshPgAclNative]::Open($privateRoot,$true,$false)
try{if([EsteriumFreshPgAclNative]::Identity($privateHandle,$true) -cne $privateIdentity){throw 'Private directory identity changed.'}}finally{$privateHandle.Dispose()}
Write-Output 'PASS: atomic protected directory creation and existing directory refusal.'
