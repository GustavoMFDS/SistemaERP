#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$path=Join-Path (Split-Path -Parent $PSScriptRoot) 'scripts\inspect-esterium-partial-install.ps1'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Recovery script syntax invalid.'}
# Execute only the exact post-preflight block, with every system action
# replaced by local test functions. Never launch or change real services.
$statements=@($ast.EndBlock.Statements)
$start=-1
for($i=0;$i -lt $statements.Count;$i++){
    if($statements[$i].Extent.Text -ceq '$completed=$false'){$start=$i;break}
}
if($start -lt 0){throw 'Inspection block not found.'}
$run=[scriptblock]::Create(($statements[$start..($statements.Count-1)] | ForEach-Object {$_.Extent.Text}) -join [Environment]::NewLine)
function Test-Case([string]$scenario,[string]$expected){
    $state=@{mode='Disabled';status='Stopped';started=0;stopped=0;called=0}
    $exe='Invoke-FixtureInspector'
    $urlFile='synthetic-url-file'
    $postgres='synthetic-postgres.exe'
    function Set-Service($Name,$StartupType){
        if($Name -ne 'EsteriumPostgreSQL16'){throw 'Unexpected mutation.'}
        if($scenario -eq 'cleanup-failure' -and $StartupType -eq 'Disabled'){throw 'Synthetic cleanup failure.'}
        $state.mode=$StartupType
    }
    function Start-Service($Name){
        if($Name -ne 'EsteriumPostgreSQL16' -or $state.mode -ne 'Manual'){throw 'Unexpected start.'}
        $state.started++;$state.status='Running'
    }
    function Stop-Service($Name){
        if($Name -ne 'EsteriumPostgreSQL16'){throw 'Unexpected stop.'}
        $state.stopped++;$state.status='Stopped'
    }
    function Get-Service($Name){
        $service=[pscustomobject]@{Status=$state.status}
        $service | Add-Member ScriptMethod WaitForStatus {param($s,$t)}
        return $service
    }
    function Get-ExactService($Name){[pscustomobject]@{ProcessId=12}}
    function Get-NetTCPConnection($State,$ErrorAction){
        if($scenario -eq 'no-listener'){return}
        $address=if($scenario -eq 'foreign-listener'){'0.0.0.0'}else{'127.0.0.1'}
        [pscustomobject]@{LocalPort=5432;LocalAddress=$address;OwningProcess=13}
    }
    function Get-CimInstance($ClassName,$Filter){[pscustomobject]@{ExecutablePath=$postgres;ParentProcessId=12}}
    function Assert-Disabled($Name){
        if($Name -eq 'EsteriumPostgreSQL16' -and ($state.status -ne 'Stopped' -or $state.mode -ne 'Disabled')){throw 'Synthetic unsafe state.'}
    }
    function Assert-NoListeners{if($state.status -ne 'Stopped'){throw 'Synthetic listener left behind.'}}
    function Invoke-FixtureInspector{
        $state.called++
        $global:LASTEXITCODE=0
        if($scenario -eq 'bad-output'){'unexpected input'}else{'ADMIN_COMMITTED'}
    }
    $actual=''
    try{$actual=@(& $run) -join '|'}catch{$actual=$_.Exception.Message}
    if(-not $actual.StartsWith($expected)){throw ('Unexpected result for '+$scenario+': '+$actual)}
    if($state.started -ne 1){throw 'Expected exactly one synthetic start.'}
    if($scenario -ne 'cleanup-failure' -and ($state.status -ne 'Stopped' -or $state.mode -ne 'Disabled' -or $state.stopped -ne 1)){throw 'Cleanup not proven.'}
    if($scenario -in @('no-listener','foreign-listener') -and $state.called -ne 0){throw 'Inspector ran before ownership verification.'}
}
Test-Case 'success' 'ADMIN_COMMITTED|PASS:'
Test-Case 'no-listener' 'ADMIN_INDETERMINATE:'
Test-Case 'foreign-listener' 'ADMIN_INDETERMINATE:'
Test-Case 'bad-output' 'ADMIN_INDETERMINATE:'
Test-Case 'cleanup-failure' 'RECOVERY_CLEANUP_FAILED:'
Write-Output 'PASS: 5 simulated inspection and cleanup cases; no SCM changes.'
