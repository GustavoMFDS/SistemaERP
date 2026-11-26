#requires -Version 5.1
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$SetupExe)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
# Exercise only the exact diagnostic branch, never an installer or real service.
$root=Split-Path -Parent $PSScriptRoot
$tokens=$null
$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $root 'scripts\install-esterium-local-complete.ps1'),[ref]$tokens,[ref]$errors)
if ($errors.Count) { throw 'Coordinator AST invalid.' }
$blocks=@($ast.FindAll({param($node)
    $node -is [Management.Automation.Language.IfStatementAst] -and
    $node.Clauses[0].Item1.Extent.Text -ceq '$GuiFailureExitCodes' -and
    $node.Extent.Text.Contains('$stageCodes=@{')
},$true))
if ($blocks.Count -ne 1) { throw 'Expected one GUI phase-two diagnostic branch.' }
$branch=$blocks[0].Extent.Text
$fixture=Join-Path ([IO.Path]::GetTempPath()) ('esterium-diagnostic-'+[Guid]::NewGuid().ToString('N')+'.ps1')
$body=@'
param([string]$Stage,[switch]$Wrapped)
$ErrorActionPreference='Stop'
$GuiFailureExitCodes=$true
$e=[InvalidOperationException]::new('Synthetic input must never appear in output.')
$e.Data['EsteriumInstallStage']=$Stage
if($Wrapped){$e=[Exception]::new('Synthetic outer error',$e)}
try { throw $e } catch {
'@
try {
    [IO.File]::WriteAllText($fixture,($body+[Environment]::NewLine+$branch+[Environment]::NewLine+'}'),[Text.UTF8Encoding]::new($true))
    $expected=[ordered]@{
        'register-service'=22
        'start-postgres-for-admin'=23
        'first-admin'=24
        'postgres-cleanup'=25
        'dependency-probe'=26
        'https-activation'=27
        'erp-cleanup'=28
        'tls-certificate-time'=29
        'unknown-input'=21
    }
    foreach($stage in $expected.Keys){
        foreach($wrapped in @($false,$true)){
            $argsForChild=@('-NoLogo','-NoProfile','-File',$fixture,'-Stage',$stage)
            if($wrapped){$argsForChild+='-Wrapped'}
            $output=@(& (Join-Path $PSHOME 'powershell.exe') @argsForChild 2>&1)
            $code=$LASTEXITCODE
            if($code -ne $expected[$stage] -or $output.Count -ne 0){
                throw ('Unexpected diagnostic result for fixture stage '+$stage)
            }
        }
    }
    $assembly=[Reflection.Assembly]::LoadFrom((Resolve-Path -LiteralPath $SetupExe).Path)
    $type=$assembly.GetType('Esterium.WindowsSetup.Installer',$true)
    $method=$type.GetMethod('ApplyFailureStage',[Reflection.BindingFlags]'Static,NonPublic')
    if($null -eq $method){throw 'GUI diagnostic mapping missing.'}
    $labels=@()
    foreach($code in 20..29){
        $label=[string]$method.Invoke($null,@([int]$code))
        if([string]::IsNullOrWhiteSpace($label)){throw 'GUI failure label missing.'}
        $labels+=$label
    }
    if(@($labels | Sort-Object -Unique).Count -ne 10){throw 'GUI failure labels must be distinct.'}
    if($method.Invoke($null,@([int]999)) -cne $method.Invoke($null,@([int]1))){
        throw 'Unknown code must use the generic failure label.'
    }
    Write-Output 'PASS: 18 synthetic child failure cases and 11 GUI label cases; no real Apply.'
} finally {
    if(Test-Path -LiteralPath $fixture){Remove-Item -LiteralPath $fixture -Force}
}
