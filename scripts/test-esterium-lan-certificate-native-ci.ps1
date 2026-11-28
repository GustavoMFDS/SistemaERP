#requires -Version 5.1
# Native callback exercise against the existing signed complete-install
# fixture. Does not invoke the LAN DNS validator (CI hostname is reserved).
param(
    [Parameter(Mandatory=$true)][string]$ToolPath,
    [Parameter(Mandatory=$true)][string]$PrivateAddress,
    [Parameter(Mandatory=$true)][int]$Port,
    [Parameter(Mandatory=$true)][string]$ExpectedSHA256
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($env:OS -ne 'Windows_NT' -or $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted'){throw 'Native TLS callback fixture requires hosted disposable Windows CI.'}
if($ExpectedSHA256 -cnotmatch '^[A-F0-9]{64}$' -or $Port -lt 1024 -or $Port -gt 65535){throw 'Invalid fixture pin or port.'}
$ip=[Net.IPAddress]::Parse($PrivateAddress)
$v=$ip.GetAddressBytes()
if($ip.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork -or -not (($v[0] -eq 10) -or ($v[0] -eq 172 -and $v[1] -ge 16 -and $v[1] -le 31) -or ($v[0] -eq 192 -and $v[1] -eq 168))){throw 'Expected private CI IPv4 address.'}
$t=$null;$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($ToolPath,[ref]$t,[ref]$e)
if($e.Count){throw 'Packaged LAN checker syntax invalid.'}
$defs=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Get-LanCertificateRejection'},$true))
$callbacks=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.ScriptBlockExpressionAst] -and $n.Extent.Text -match '^\{\s*param\(\$sender,\$cert,\$chain,\$policyErrors\)'},$true))
if($defs.Count -ne 1 -or $callbacks.Count -ne 1){throw 'Expected unique packaged policy and TLS callback.'}
. ([scriptblock]::Create($defs[0].Extent.Text))
$callbackText=$callbacks[0].Extent.Text
$callback=[Net.Security.RemoteCertificateValidationCallback]([scriptblock]::Create($callbackText.Substring(1,$callbackText.Length-2)))
function Invoke-NativeHandshake([bool]$UseCallback,[string]$Pin) {
    $script:expected=$Pin
    $script:validationSeen=$false
    $script:validationOK=$false
    $script:certificateHashOK=$false
    $script:certificateRejection='unclassified'
    $tcp=[Net.Sockets.TcpClient]::new($ip.AddressFamily)
    $ssl=$null
    try{
        if(-not $tcp.ConnectAsync($ip,$Port).Wait([TimeSpan]::FromSeconds(8))){throw 'CI native TLS connection timed out.'}
        $tcp.ReceiveTimeout=8000
        $tcp.SendTimeout=8000
        if($UseCallback){$ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false,$callback)}
        else{$ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false)}
        try{
            # Same strict OS TLS policy and mandatory revocation as packaged tool.
            $ssl.AuthenticateAsClient('esterium.ci.invalid',$null,[Security.Authentication.SslProtocols]::None,$true)
            return [pscustomobject]@{Accepted=$true;Seen=$script:validationSeen;Reason=$script:certificateRejection}
        }catch{
            # Only authentication failures are classified; transport errors remain failures.
            $ex=$_.Exception
            while($null -ne $ex.InnerException){if($ex -is [Security.Authentication.AuthenticationException]){break};$ex=$ex.InnerException}
            if($ex -isnot [Security.Authentication.AuthenticationException]){throw}
            return [pscustomobject]@{Accepted=$false;Seen=$script:validationSeen;Reason=$script:certificateRejection}
        }
    }finally{if($null -ne $ssl){$ssl.Dispose()};$tcp.Dispose()}
}
$baseline=Invoke-NativeHandshake $false $ExpectedSHA256
$correct=Invoke-NativeHandshake $true $ExpectedSHA256
if(-not $correct.Seen -or $correct.Accepted -ne $baseline.Accepted){throw 'Packaged TLS callback diverged from strict native trust for approved pin.'}
if($baseline.Accepted){
    if($correct.Reason -cne 'pass'){throw 'Native callback failed to classify approved connection.'}
    $wrong=if($ExpectedSHA256 -ceq ('0'*64)){'1'*64}else{'0'*64}
    $rejected=Invoke-NativeHandshake $true $wrong
    if($rejected.Accepted -or -not $rejected.Seen -or $rejected.Reason -cne 'pin-mismatch'){throw 'Native callback did not reject incorrect pin.'}
    Write-Output 'PASS: actual strict TLS callback accepted CI fixture and rejected incorrect pin.'
}else{
    if($correct.Reason -cne 'revocation-unavailable'){throw ('Unexpected native CI certificate rejection category: '+$correct.Reason)}
    Write-Output 'PASS: actual strict TLS callback rejected CI fixture with unavailable revocation; no positive LAN trust evidence.'
}
Write-Output 'NOT PROVEN: commercial PKI, physical LAN or certificate renewal.'
