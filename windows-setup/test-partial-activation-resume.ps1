#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
Add-Type -AssemblyName System.ServiceProcess
$path=Join-Path (Split-Path -Parent $PSScriptRoot) 'scripts\resume-esterium-partial-activation.ps1'
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$t,[ref]$e)
if($e.Count){throw 'Activation AST invalid.'}
$statements=@($ast.EndBlock.Statements)
$start=-1
for($i=0;$i -lt $statements.Count;$i++){
    if($statements[$i].Extent.Text -ceq '$inspection=@(& $inspector @inspectArgs -Inspect)'){$start=$i;break}
}
if($start -lt 0){throw 'Activation block missing.'}
$run=[scriptblock]::Create(($statements[$start..($statements.Count-1)]|ForEach-Object {$_.Extent.Text}) -join [Environment]::NewLine)
function Test-Case([string]$scenario,[string]$expected){
    $names=@('EsteriumERP','EsteriumGarnet','EsteriumPostgreSQL16')
    $state=@{}
    foreach($name in $names){$state[$name]=@{mode='Disabled';status='Stopped'}}
    $counts=@{probe=0;activation=0;mutation=0}
    $receipts=@{probe=$false;activation=$false}
    $inspector='Invoke-FixtureInspector';$inspectArgs=@{}
    $probe='Invoke-FixtureProbe';$activation='Invoke-FixtureActivation';$argsForOriginal=@{}
    $readiness='C:\synthetic-readiness';$installed='C:\synthetic-installation'
    function Invoke-FixtureInspector{
        if($scenario -eq 'absent'){'ADMIN_ABSENT'}else{'ADMIN_COMMITTED'}
        'PASS: all three services stopped and disabled; no recovery approval granted.'
    }
    function Assert-ServiceState($Name,$StateName,$Mode){
        if($state[$Name].status -ne $StateName -or $state[$Name].mode -ne $Mode){throw 'Synthetic unsafe state.'}
    }
    function Assert-FreePorts{
        foreach($name in $names){if($state[$name].status -ne 'Stopped'){throw 'Synthetic occupied port.'}}
    }
    function Set-Service($Name,$StartupType){
        $counts.mutation++
        if($scenario -eq 'cleanup-failure' -and $Name -eq 'EsteriumERP' -and $StartupType -eq 'Disabled'){throw 'Synthetic cleanup failure.'}
        $state[$Name].mode=$StartupType
    }
    function Get-Service($Name){
        $s=[pscustomobject]@{Status=$state[$Name].status}
        $s|Add-Member ScriptMethod WaitForStatus {param($a,$b)}
        return $s
    }
    function Stop-Service($Name){$state[$Name].status='Stopped'}
    function Test-Path($LiteralPath,$PathType){
        if($LiteralPath.EndsWith('probe-result.json')){return $receipts.probe}
        if($LiteralPath.EndsWith('installation-ready.json')){return $receipts.activation}
        throw 'Unexpected fixture path.'
    }
    function Invoke-FixtureProbe{
        $counts.probe++
        if($state['EsteriumERP'].mode -ne 'Manual'){throw 'Expected manual ERP.'}
        if($scenario -in @('probe-failure','cleanup-failure')){throw 'Synthetic delegate failure.'}
        if($scenario -ne 'missing-probe-receipt'){$receipts.probe=$true}
    }
    function Invoke-FixtureActivation{
        $counts.activation++
        if(-not $receipts.probe){throw 'Activation before receipt.'}
        foreach($name in $names){$state[$name].mode='Auto';$state[$name].status='Running'}
        if($scenario -eq 'https-failure'){throw 'Synthetic HTTPS failure.'}
        if($scenario -ne 'missing-activation-receipt'){$receipts.activation=$true}
    }
    $actual=''
    try{$actual=@(& $run) -join '|'}catch{$actual=$_.Exception.Message}
    if(-not $actual.StartsWith($expected)){throw ('Unexpected result for '+$scenario+': '+$actual)}
    if($scenario -eq 'absent' -and ($counts.mutation -ne 0 -or $counts.probe -ne 0)){throw 'Admin gate did not prevent mutation.'}
    if($scenario -in @('probe-failure','cleanup-failure','missing-probe-receipt') -and $counts.activation -ne 0){throw 'Activation ran without probe evidence.'}
    if($scenario -eq 'success'){
        foreach($name in $names){Assert-ServiceState $name 'Running' 'Auto'}
        if($counts.probe -ne 1 -or $counts.activation -ne 1){throw 'Delegate invoked multiple times.'}
    }elseif($scenario -ne 'cleanup-failure'){
        foreach($name in $names){Assert-ServiceState $name 'Stopped' 'Disabled'}
    }
}
Test-Case 'success' 'PASS: original signed probe'
Test-Case 'absent' 'ACTIVATION_REFUSED_ADMIN_NOT_COMMITTED'
Test-Case 'probe-failure' 'ACTIVATION_PROBE_FAILED:'
Test-Case 'missing-probe-receipt' 'ACTIVATION_PROBE_FAILED:'
Test-Case 'https-failure' 'ACTIVATION_HTTPS_FAILED:'
Test-Case 'missing-activation-receipt' 'ACTIVATION_POSTCHECK_FAILED:'
Test-Case 'cleanup-failure' 'ACTIVATION_CLEANUP_FAILED:'
Write-Output 'PASS: 7 simulated activation gates and cleanup cases; no real services changed.'
