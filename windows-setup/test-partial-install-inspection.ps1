#requires -Version 5.1
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
Add-Type -AssemblyName System.ServiceProcess
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
    $state=@{mode='Disabled';status='Stopped';started=0;stopped=0;called=0;bootstrap=0}
    $ResumeAdmin=$scenario.StartsWith('resume-')
    $bootstrap='Invoke-FixtureBootstrap'
    $DiagnosticPublisher='synthetic'
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
        if($scenario -eq 'bad-output'){'unexpected input'}
        elseif($ResumeAdmin -and $scenario -ne 'resume-existing' -and ($state.called -eq 1 -or $scenario -eq 'resume-unconfirmed')){'ADMIN_ABSENT'}
        else{'ADMIN_COMMITTED'}
    }
    function Invoke-FixtureBootstrap {
        $state.bootstrap++
        if($scenario -eq 'resume-mismatch'){throw 'As senhas nao coincidem.'}
        if($scenario -eq 'resume-business'){throw 'Bootstrap recusado: FAIL: first administrator was not created: database contains business data; first-admin setup refused'}
        if($scenario -eq 'resume-arbitrary'){throw 'synthetic unapproved diagnostic'}
        if($scenario -eq 'resume-commit-lost'){throw 'Bootstrap recusado: FAIL: first administrator outcome not confirmed: commit'}
        if($scenario -eq 'resume-deadline'){throw 'Bootstrap recusado: FAIL: first administrator outcome not confirmed: deadline'}
        if($scenario -eq 'resume-canceled'){throw 'Bootstrap recusado: FAIL: first administrator outcome not confirmed: canceled'}
    }
    $actual=''
    try{$actual=@(& $run) -join '|'}catch{$actual=$_.Exception.Message}
    if(-not $actual.StartsWith($expected)){throw ('Unexpected result for '+$scenario+': '+$actual)}
    if($state.started -ne 1){throw 'Expected exactly one synthetic start.'}
    if($scenario -ne 'cleanup-failure' -and ($state.status -ne 'Stopped' -or $state.mode -ne 'Disabled' -or $state.stopped -ne 1)){throw 'Cleanup not proven.'}
    if($scenario -in @('no-listener','foreign-listener') -and $state.called -ne 0){throw 'Inspector ran before ownership verification.'}
    if($ResumeAdmin){
        $expectedCalls=if($scenario -eq 'resume-existing'){0}else{1}
        if($state.bootstrap -ne $expectedCalls){throw 'Unexpected bootstrap invocation count.'}
        if($scenario -in @('resume-success','resume-unconfirmed') -and $state.called -ne 2){throw 'Post-bootstrap inspection missing.'}
    }
}
Test-Case 'success' 'ADMIN_COMMITTED|PASS:'
Test-Case 'no-listener' 'ADMIN_INDETERMINATE:'
Test-Case 'foreign-listener' 'ADMIN_INDETERMINATE:'
Test-Case 'bad-output' 'ADMIN_INDETERMINATE:'
Test-Case 'cleanup-failure' 'RECOVERY_CLEANUP_FAILED:'
Test-Case 'resume-success' 'ADMIN_COMMITTED|PASS:'
Test-Case 'resume-existing' 'RESUME_REFUSED_NOT_ABSENT:'
Test-Case 'resume-mismatch' 'RESUME_PASSWORD_MISMATCH:'
Test-Case 'resume-business' 'RESUME_NONEMPTY_DATABASE:'
Test-Case 'resume-arbitrary' 'RESUME_BOOTSTRAP_FAILED:'
Test-Case 'resume-unconfirmed' 'RESUME_COMMIT_UNCONFIRMED:'
Test-Case 'resume-commit-lost' 'RESUME_COMMIT_UNCONFIRMED:'
Test-Case 'resume-deadline' 'RESUME_BOOTSTRAP_DEADLINE:'
Test-Case 'resume-canceled' 'RESUME_BOOTSTRAP_CANCELED:'
Write-Output 'PASS: 14 simulated inspection/resume and cleanup cases; no SCM changes.'

# Exercise the actual pipe-write block with an inert child that compares bytes.
# A non-ASCII fixture ensures this does not accidentally use the console code page.
$bootstrapPath=Join-Path (Split-Path -Parent $PSScriptRoot) 'scripts\bootstrap-esterium-first-admin.ps1'
$tokens=$null;$errors=$null
$bootstrapAst=[Management.Automation.Language.Parser]::ParseFile($bootstrapPath,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Bootstrap AST invalid.'}
$pipeAssignments=@($bootstrapAst.FindAll({param($n)
    $n -is [Management.Automation.Language.AssignmentStatementAst] -and
    $n.Extent.Text -ceq '$inputPipe=$process.StandardInput.BaseStream'
},$true))
if($pipeAssignments.Count -ne 1){throw 'Expected unique pipe assignment.'}
$pipeStatements=@($pipeAssignments[0].Parent.Statements)
$pipeIndex=[array]::IndexOf($pipeStatements,$pipeAssignments[0])
if($pipeIndex -lt 0 -or $pipeStatements[$pipeIndex+1] -isnot [Management.Automation.Language.TryStatementAst]){throw 'Expected pipe write block.'}
$writePipe=[scriptblock]::Create($pipeStatements[$pipeIndex].Extent.Text+[Environment]::NewLine+$pipeStatements[$pipeIndex+1].Extent.Text)
$fixture='Synthetic-'+[char]0x00e9+[char]0x65e5+'-Only'
$passwordBytes=[Text.UTF8Encoding]::new($false,$true).GetBytes($fixture)
$child=@'
$inputStream=[Console]::OpenStandardInput()
$memory=[IO.MemoryStream]::new()
$inputStream.CopyTo($memory)
$hex=[BitConverter]::ToString($memory.ToArray())
if($hex -cne '53-79-6E-74-68-65-74-69-63-2D-C3-A9-E6-97-A5-2D-4F-6E-6C-79-0A'){exit 1}
[Console]::WriteLine('PASS')
'@
$info=[Diagnostics.ProcessStartInfo]::new()
$info.FileName=Join-Path $PSHOME 'powershell.exe'
$info.Arguments='-NoProfile -EncodedCommand '+[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($child))
$info.UseShellExecute=$false
$info.CreateNoWindow=$true
$info.RedirectStandardInput=$true
$info.RedirectStandardOutput=$true
$encodingAssignments=@($bootstrapAst.FindAll({param($n)
    $n -is [Management.Automation.Language.AssignmentStatementAst] -and
    $n.Extent.Text -ceq '$previousInputEncoding=[Console]::InputEncoding'
},$true))
if($encodingAssignments.Count -ne 1){throw 'Expected unique encoding scope.'}
$encodingStatements=@($encodingAssignments[0].Parent.Statements)
$encodingIndex=[array]::IndexOf($encodingStatements,$encodingAssignments[0])
if($encodingIndex -lt 0 -or $encodingStatements[$encodingIndex+1] -isnot [Management.Automation.Language.TryStatementAst]){throw 'Expected encoding scope.'}
$startChild=[scriptblock]::Create($encodingStatements[$encodingIndex].Extent.Text+[Environment]::NewLine+$encodingStatements[$encodingIndex+1].Extent.Text)
$utf8=[Text.UTF8Encoding]::new($false,$true)
$originalConsoleEncoding=[Console]::InputEncoding
$process=$null
try {
    # Reproduce the CI runner's BOM-emitting input encoding even on a
    # developer machine with an OEM codepage.
    [Console]::InputEncoding=[Text.UTF8Encoding]::new($true)
    . $startChild
    if([Console]::InputEncoding.GetPreamble().Length -ne 3){throw 'Console input encoding was not restored.'}
    & $writePipe
    if(-not $process.WaitForExit(15000)){throw 'Synthetic pipe child timed out.'}
    if($process.ExitCode -ne 0 -or $process.StandardOutput.ReadToEnd().Trim() -cne 'PASS'){throw 'UTF-8 pipe bytes did not match.'}
    Write-Output 'PASS: real Windows anonymous pipe preserves synthetic UTF-8 bytes without BOM.'
} finally {
    [Array]::Clear($passwordBytes,0,$passwordBytes.Length)
    [Console]::InputEncoding=$originalConsoleEncoding
    if($null -ne $process){
        if(-not $process.HasExited){$process.Kill()}
        $process.Dispose()
    }
}


# Synthetic bootstrap output boundary: exercise the exact failure branch
# without launching the first-admin CLI or accessing any database.
$failureBlocks=@($bootstrapAst.FindAll({param($n)
    $n -is [Management.Automation.Language.IfStatementAst] -and
    $n.Clauses[0].Item1.Extent.Text.Contains('$process.ExitCode -ne 0 -or $output -cne')
},$true))
if($failureBlocks.Count -ne 1){throw 'Expected unique first-admin failure branch.'}
$failureBranch=[scriptblock]::Create($failureBlocks[0].Extent.Text)
$cases=@(
    @{Code='commit';Expected='commit'},
    @{Code='deadline';Expected='deadline'},
    @{Code='canceled';Expected='canceled'},
    @{Code='unexpected synthetic marker';Expected=''}
)
$process=[pscustomobject]@{ExitCode=1}
$output=''
foreach($case in $cases){
    $errorOutput='FAIL: first administrator outcome not confirmed: '+$case.Code
    $actual=''
    try {& $failureBranch}catch{$actual=$_.Exception.Message}
    if($case.Expected){
        if($actual -cne ('Bootstrap recusado: FAIL: first administrator outcome not confirmed: '+$case.Expected)){
            throw 'Expected exact fixed outcome label.'
        }
    } elseif($actual -cne 'Bootstrap recusado. Preserve o banco e consulte o diagnostico protegido.'){
        throw 'Unrecognized diagnostic leaked or accepted.'
    }
}
Write-Output 'PASS: 4 fixed outcome wrapper cases, including unknown diagnostic rejection.'