#requires -Version 5.1
# Pure descriptor fixtures; never invokes installer or mutates ACLs.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$tool=Join-Path $PSScriptRoot 'install-esterium-local-dependencies.ps1'
$tokens=$null; $errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($tool,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Installer AST parse failed.'}
$definitions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Test-PostgresDataDescriptor'},$true))
if($definitions.Count -ne 1){throw 'Missing or ambiguous descriptor validator.'}
$validator=$definitions[0].Body.GetScriptBlock()
$svc='S-1-5-21-111-222-333-1001'
$operator='S-1-5-21-111-222-333-1002'
$rootAcl="(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;$svc)"
$fileAcl="(A;ID;FA;;;SY)(A;ID;FA;;;BA)(A;ID;0x1301bf;;;$svc)"
$cases=@(
    @{name='protected-root'; sddl="O:BAD:P$rootAcl"; dir=$true; root=$true; want='pass'},
    @{name='trusted-service-owned-file'; sddl="O:${svc}D:$fileAcl"; dir=$false; root=$false; want='pass'},
    @{name='installer-owned-child'; sddl="O:${operator}D:$rootAcl"; dir=$true; root=$false; want='owner'},
    @{name='installer-explicit-ace'; sddl="O:BAD:P$rootAcl(A;OICI;FA;;;$operator)"; dir=$true; root=$true; want='unexpected-principal'},
    @{name='installer-inherited-ace'; sddl="O:BAD:$fileAcl(A;ID;FA;;;$operator)"; dir=$false; root=$false; want='unexpected-principal'},
    @{name='root-inherits'; sddl="O:BAD:$rootAcl"; dir=$true; root=$true; want='root-inheritance'},
    @{name='everyone-ace'; sddl="O:BAD:P$rootAcl(A;OICI;FR;;;WD)"; dir=$true; root=$true; want='unexpected-principal'},
    @{name='creator-owner-ace'; sddl="O:BAD:P$rootAcl(A;OICIIO;FA;;;CO)"; dir=$true; root=$true; want='unexpected-principal'},
    @{name='service-full-control'; sddl="O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;$svc)"; dir=$true; root=$true; want='excessive-rights'},
    @{name='service-missing'; sddl='O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'; dir=$true; root=$true; want='missing-effective-rights'},
    @{name='inherit-only-service'; sddl="O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICIIO;0x1301bf;;;$svc)"; dir=$true; root=$true; want='missing-effective-rights'},
    @{name='missing-propagation'; sddl="O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;;0x1301bf;;;$svc)"; dir=$true; root=$true; want='missing-child-rights'},
    @{name='no-propagate'; sddl="O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICINP;0x1301bf;;;$svc)"; dir=$true; root=$true; want='missing-child-rights'},
    @{name='deny-ace'; sddl="O:BAD:P(D;;FR;;;$svc)$rootAcl"; dir=$true; root=$true; want='unsupported-ace'},
    @{name='no-dacl'; sddl='O:BA'; dir=$true; root=$true; want='null-dacl'}
)
foreach($t in $cases){
    $descriptor=[Security.AccessControl.RawSecurityDescriptor]::new($t.sddl)
    $actual=& $validator $descriptor $svc $t.dir $t.root
    if($actual -ne $t.want){throw ($t.name+': expected '+$t.want+', got '+$actual)}
    Write-Output ('PASS '+$t.name+' '+$actual)
}
# Require the common call to precede attestation and follow stopped/listener checks.
$source=[IO.File]::ReadAllText($tool)
$call='Assert-FreshPostgresDataAcl $pgData $pgServiceSid'
$marker='# Seal the new-cluster proof'
$guard='# A listener should not survive shutdown'
if(([regex]::Matches($source,[regex]::Escape($call))).Count -ne 1 -or $source.IndexOf($call) -lt $source.IndexOf($guard) -or $source.IndexOf($call) -gt $source.IndexOf($marker) -or $source.IndexOf($guard) -lt 0 -or $source.IndexOf($marker) -lt 0){throw 'ACL gate ordering incorrect.'}
Write-Output 'PASS common fresh-cluster gate precedes attestation'
Write-Output ('TOTAL '+($cases.Count+1)+' passed; descriptor and static ordering only.')

# Exercise the actual tree walk with synthetic filesystem metadata only.
$walkDefs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Assert-FreshPostgresDataAcl'},$true))
if($walkDefs.Count -ne 1){throw 'Missing or ambiguous tree validator.'}
$walk=$walkDefs[0].Body.GetScriptBlock()
& {
    $fixtureRoot='C:\EsteriumAclFixture'
    $leaf=Join-Path $fixtureRoot 'pg_wal'
    $dataFile=Join-Path $leaf 'synthetic'
    function Test-PostgresDataDescriptor { & $validator @args }
    function Get-Item {
        param([string]$LiteralPath,[switch]$Force,$ErrorAction)
        if($LiteralPath -eq $dataFile -and $mode -eq 'missing'){throw 'synthetic unreadable object'}
        $isFile=$LiteralPath -eq $dataFile
        $attributes=if($isFile){[IO.FileAttributes]::Normal}else{[IO.FileAttributes]::Directory}
        if(($mode -eq 'root-reparse' -and $LiteralPath -eq $fixtureRoot) -or ($mode -eq 'child-reparse' -and $LiteralPath -eq $leaf)){
            $attributes=$attributes -bor [IO.FileAttributes]::ReparsePoint
        }
        [pscustomobject]@{PSIsContainer=(-not $isFile);Attributes=$attributes;FullName=$LiteralPath}
    }
    function Get-ChildItem {
        param([string]$LiteralPath,[switch]$Force,$ErrorAction)
        if($LiteralPath -eq $fixtureRoot){[pscustomobject]@{FullName=$leaf}}
        elseif($LiteralPath -eq $leaf){[pscustomobject]@{FullName=$dataFile}}
    }
    function Get-Acl {
        param([string]$LiteralPath,$ErrorAction)
        if($mode -eq 'acl-unreadable' -and $LiteralPath -eq $dataFile){throw 'synthetic unreadable ACL'}
        $sddl=if($LiteralPath -eq $dataFile){
            if($mode -eq 'bad-owner'){"O:${operator}D:$fileAcl"}else{"O:${svc}D:$fileAcl"}
        }else{"O:BAD:P$rootAcl"}
        $raw=[Security.AccessControl.RawSecurityDescriptor]::new($sddl)
        $bytes=New-Object byte[] $raw.BinaryLength
        $raw.GetBinaryForm($bytes,0)
        $result=[pscustomobject]@{Bytes=$bytes}
        $result | Add-Member -MemberType ScriptMethod -Name GetSecurityDescriptorBinaryForm -Value { ,$this.Bytes }
        $result
    }
    $cases=@(
        @{mode='valid';want='inspected 3 objects'},
        @{mode='bad-owner';want='ACL rejected: owner'},
        @{mode='root-reparse';want='nonphysical directory'},
        @{mode='child-reparse';want='reparse point'},
        @{mode='missing';want='synthetic unreadable object'},
        @{mode='acl-unreadable';want='synthetic unreadable ACL'}
    )
    foreach($case in $cases){
        $mode=$case.mode
        $failed=$false
        try{$result=(& {param($DataDir,$ServiceSid) & $walk} $fixtureRoot $svc) -join ' '}catch{$failed=$true;$result=$_.Exception.Message}
        if($failed -ne ($mode -ne 'valid') -or $result -notmatch [regex]::Escape($case.want)){throw ('Tree case '+$mode+' unexpected: '+$result)}
        Write-Output ('PASS tree '+$mode)
    }
}
Write-Output 'TOTAL 22 passed; synthetic descriptors, static ordering and mocked tree only.'

# Validate the exact proposed descriptor without applying it to any file.
$builders=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'New-FreshPostgresDataDescriptor'},$true))
if($builders.Count -ne 1){throw 'Missing descriptor builder.'}
$builder=$builders[0].Body.GetScriptBlock()
foreach($directory in @($true,$false)){
    $descriptor=& {param($ServiceSid,$Directory) & $builder} $svc $directory
    if((& $validator $descriptor $svc $directory $directory) -ne 'pass'){throw 'Generated descriptor violates policy.'}
    if($descriptor.Owner.Value -ne 'S-1-5-32-544' -or $descriptor.DiscretionaryAcl.Count -ne 3){throw 'Generated owner or ACE count invalid.'}
    if(-not ($descriptor.ControlFlags -band [Security.AccessControl.ControlFlags]::DiscretionaryAclProtected)){throw 'Generated DACL not protected.'}
    $expectedFlags=if($directory){3}else{0}
    foreach($ace in $descriptor.DiscretionaryAcl){
        if([int]$ace.AceFlags -ne $expectedFlags){throw 'Generated inheritance flags invalid.'}
    }
    Write-Output ('PASS descriptor-builder directory='+$directory)
}
foreach($badSid in @('S-1-5-18','S-1-5-32-544','S-1-1-0','malformed')){
    $rejected=$false
    try{& {param($ServiceSid,$Directory) & $builder} $badSid $true | Out-Null}catch{$rejected=$true}
    if(-not $rejected){throw 'Unapproved service SID accepted.'}
    Write-Output ('PASS descriptor-builder rejects '+$badSid)
}
Write-Output 'TOTAL 28 passed; no filesystem ACL modification.'

# Test exact service binding, without inspecting or registering a real service.
$bindingDefs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Test-FreshPostgresServiceCommand'},$true))
if($bindingDefs.Count -ne 1){throw 'Missing service binding validator.'}
$binding=$bindingDefs[0].Body.GetScriptBlock()
$install='C:\Program Files\PostgreSQL\16'
$data='C:\ProgramData\Esterium\PostgresData'
$service='EsteriumPostgreSQL16'
$cmd='"'+(Join-Path $install 'bin\pg_ctl.exe')+'" runservice -N "'+$service+'" -D "'+$data+'"'
$bindings=@(
 @{name='zip-exact';cmd=$cmd+' -w';zip=$true;want=$true},
 @{name='zip-no-wait';cmd=$cmd;zip=$true;want=$false},
 @{name='edb-exact';cmd=$cmd;zip=$false;want=$true},
 @{name='edb-wait';cmd=$cmd+' -w';zip=$false;want=$true},
 @{name='wrong-data';cmd=$cmd.Replace($data,$data+'Other');zip=$false;want=$false},
 @{name='wrong-exe';cmd=$cmd.Replace('pg_ctl.exe','other.exe');zip=$false;want=$false},
 @{name='wrong-service';cmd=$cmd.Replace($service,$service+'Other');zip=$false;want=$false},
 @{name='extra-option';cmd=$cmd+' -o "-c data_directory=C:\Other"';zip=$false;want=$false},
 @{name='duplicate-data';cmd=$cmd+' -D "C:\Other"';zip=$false;want=$false},
 @{name='wrong-flag-case';cmd=$cmd.Replace(' -D ',' -d ');zip=$false;want=$false},
 @{name='unquoted-exe';cmd=$cmd.TrimStart('"').Replace('.exe"','.exe');zip=$false;want=$false},
 @{name='shell-wrapper';cmd='cmd.exe /c '+$cmd;zip=$false;want=$false}
)
foreach($b in $bindings){
 $actual=& {param($Command,$InstallDir,$DataDir,$ServiceName,$BinaryZip) & $binding} $b.cmd $install $data $service $b.zip
 if($actual -ne $b.want){throw ('Binding case failed: '+$b.name)}
 Write-Output ('PASS service-binding '+$b.name)
}
Write-Output 'TOTAL 40 passed; synthetic binding, descriptors and mock tree only.'
# Parent trust boundary: no filesystem mutations.
$parentDefs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Test-FreshPostgresParentDescriptor'},$true))
if($parentDefs.Count -ne 1){throw 'Missing parent descriptor validator.'}
$parentValidator=$parentDefs[0].Body.GetScriptBlock()
$parentBase='O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'
$parentCases=@(
 @{name='private-parent';sddl=$parentBase;immediate=$true;want='pass'},
 @{name='shared-read-parent';sddl=$parentBase+'(A;;FR;;;BU)';immediate=$true;want='pass'},
 @{name='operator-owner';sddl="O:${operator}D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)";immediate=$true;want='parent-owner'},
 @{name='null-dacl';sddl='O:BA';immediate=$true;want='parent-null-dacl'},
 @{name='delete-child';sddl=$parentBase+'(A;;0x40;;;BU)';immediate=$false;want='parent-untrusted-write'},
 @{name='delete-parent';sddl=$parentBase+'(A;;SD;;;BU)';immediate=$false;want='parent-untrusted-write'},
 @{name='write-dacl';sddl=$parentBase+'(A;;WD;;;BU)';immediate=$false;want='parent-untrusted-write'},
 @{name='write-owner';sddl=$parentBase+'(A;;WO;;;BU)';immediate=$false;want='parent-untrusted-write'},
 @{name='generic-write';sddl=$parentBase+'(A;;GW;;;BU)';immediate=$true;want='parent-untrusted-write'},
 @{name='generic-all';sddl=$parentBase+'(A;;GA;;;BU)';immediate=$false;want='parent-untrusted-write'},
 @{name='higher-create-sibling';sddl=$parentBase+'(A;;0x4;;;BU)';immediate=$false;want='pass'},
 @{name='immediate-create-sibling';sddl=$parentBase+'(A;;0x4;;;BU)';immediate=$true;want='parent-untrusted-write'},
 @{name='immediate-write-attributes';sddl=$parentBase+'(A;;0x100;;;BU)';immediate=$true;want='parent-untrusted-write'},
 @{name='inherit-only-creator';sddl=$parentBase+'(A;OICIIO;FA;;;CO)';immediate=$false;want='pass'},
 @{name='conservative-deny';sddl=$parentBase+'(D;;WD;;;BU)(A;;WD;;;BU)';immediate=$false;want='parent-untrusted-write'}
)
foreach($case in $parentCases){
    $raw=[Security.AccessControl.RawSecurityDescriptor]::new($case.sddl)
    $actual=& $parentValidator $raw $case.immediate
    if($actual -cne $case.want){throw ('Parent case '+$case.name+' expected '+$case.want+' got '+$actual)}
    Write-Output ('PASS parent '+$case.name)
}
Write-Output 'TOTAL 55 passed; synthetic policy, service binding, parent descriptors and mocked tree only.'
