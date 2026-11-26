#requires -Version 5.1
# Extract only the actual gate; never invoke activation or real SCM commands.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=Split-Path -Parent $PSScriptRoot
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'scripts\activate-esterium-local-first-install.ps1'),[ref]$t,[ref]$e)
if($e.Count){throw 'Activation AST invalid.'}
$fn=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Start-ERPAfterDependencyListeners'},$true))
if($fn.Count -ne 1){throw 'Missing actual startup gate.'}
. ([scriptblock]::Create($fn[0].Extent.Text))
$calls=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -ceq 'Start-ERPAfterDependencyListeners'},$true))
if($calls.Count -ne 1){throw 'Expected a single activation gate call.'}
$source=$ast.Extent.Text
$start=$source.IndexOf('foreach($name in $dependencyNames){'+[Environment]::NewLine+'        $current=Get-Service')
if($start -lt 0){$start=$source.IndexOf('foreach($name in $dependencyNames){'+[char]10+'        $current=Get-Service')}
$health=$source.IndexOf('$healthUri=$expectedOrigin')
if($start -lt 0 -or $calls[0].Extent.StartOffset -le $start -or $calls[0].Extent.EndOffset -ge $health){throw 'Gate must follow dependency startup and precede HTTPS.'}
if(-not $source.Contains("(Join-Path " + '$PSScriptRoot' + " 'check-esterium-local-dependencies.ps1'))){")){throw 'Inventory helper missing from signed helper list.'}
$state=@{mode='ready';events=[Collections.Generic.List[string]]::new()}
function Invoke-InventoryFixture {
    param([string]$PostgresServiceName,[string]$RedisServiceName,[string]$PostgresBin,[int]$PostgresPort,[int]$RedisPort,[int]$ListenerReadyTimeoutSeconds,[switch]$RequireServiceOwnership)
    $state.events.Add('inventory')
    if($PostgresServiceName -cne 'PgFixture' -or $RedisServiceName -cne 'CacheFixture' -or $PostgresBin -cne 'C:\Synthetic\bin' -or $PostgresPort -ne 15432 -or $RedisPort -ne 16379 -or $ListenerReadyTimeoutSeconds -ne 30 -or -not $RequireServiceOwnership){throw 'Invalid forwarded inventory parameters.'}
    if($state.mode -ne 'ready'){throw ('Synthetic '+$state.mode)}
}
function Start-Service {
    [CmdletBinding()]param([string]$Name)
    if($Name -cne 'EsteriumERP'){throw 'Unexpected service start.'}
    $state.events.Add('erp')
}
foreach($mode in @('ready','absent-listener','non-loopback','wrong-owner','inventory-error')){
    $state.mode=$mode;$state.events.Clear();$message=''
    try{Start-ERPAfterDependencyListeners 'Invoke-InventoryFixture' 'PgFixture' 'CacheFixture' 'C:\Synthetic\bin' 15432 16379}catch{$message=$_.Exception.Message}
    if($mode -eq 'ready'){
        if($message -or ($state.events -join ',') -cne 'inventory,erp'){throw 'Ready case did not start ERP after inventory.'}
    }else{
        if($message -cne ('Synthetic '+$mode) -or ($state.events -join ',') -cne 'inventory'){throw ('ERP start not refused: '+$mode)}
    }
}
Write-Output 'PASS: 5 activation listener gate simulations, custom port forwarding, ordering and signature-list anchor. No real SCM operations.'
