#requires -Version 5.1
# Pure source policy tests; no event log, SCM or registry mutations.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$p=Join-Path $PSScriptRoot 'provision-esterium-startup-event-source.ps1'
$t=$null;$e=$null;$ast=[Management.Automation.Language.Parser]::ParseFile($p,[ref]$t,[ref]$e)
if($e.Count){throw 'Syntax invalid.'}
$d=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Get-StartupSourcePolicy'},$true))
if($d.Count -ne 1){throw 'Missing source policy.'}
. ([scriptblock]::Create($d[0].Extent.Text))
$resource=Join-Path $env:SystemRoot 'System32\EventCreate.exe'
$cases=@(
 @{Name='missing';Exists=$false;Log='';Resource='';Types=0;Want='missing'},
 @{Name='approved';Exists=$true;Log='Application';Resource=$resource;Types=7;Want='pass'},
 @{Name='expandable';Exists=$true;Log='Application';Resource='%SystemRoot%\System32\EventCreate.exe';Types=1;Want='pass'},
 @{Name='wrong log';Exists=$true;Log='System';Resource=$resource;Types=7;Want='wrong-log'},
 @{Name='wrong resource';Exists=$true;Log='Application';Resource='C:\unapproved.exe';Types=7;Want='wrong-resource'},
 @{Name='wrong type';Exists=$true;Log='Application';Resource=$resource;Types=2;Want='wrong-type'},
 @{Name='empty type';Exists=$true;Log='Application';Resource=$resource;Types=0;Want='wrong-type'}
)
foreach($c in $cases){
    $actual=Get-StartupSourcePolicy $c.Exists $c.Log $c.Resource $c.Types $resource
    if($actual -cne $c.Want){throw ('Unexpected source policy '+$c.Name)}
    Write-Output ('PASS '+$c.Name)
}
$s=[IO.File]::ReadAllText($p)
$approval=$s.IndexOf('& (Join-Path $PSScriptRoot ''check-esterium-signed-release.ps1'')')
$plan=$s.IndexOf('if(-not $Apply)')
$mutation=$s.IndexOf('    New-EventLog')
if($approval -lt 0 -or $plan -le $approval -or $mutation -le $plan -or $s -match '(?m)^\s*(Remove-EventLog|Start-Service|Stop-Service|Set-Service)\b'){throw 'Maintenance boundary invalid.'}
Write-Output 'PASS signed approval and read-only plan precede mutation; no removal or service lifecycle action.'
Write-Output 'TOTAL 8 source policy and static maintenance cases passed.'
