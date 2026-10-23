#requires -Version 5.1
<#
  Read-only HTTPS reachability and certificate trust probe, executed from
  EACH actual Windows POS terminal. It never installs a CA certificate,
  changes Windows Firewall, stores passwords or modifies the ERP server.
  A green result from CI is NOT evidence about physical LAN computers.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$ERPUrl,
    [Parameter(Mandatory=$true)][string]$ExpectedCertificateSHA256,
    [ValidateRange(1,30)][int]$TimeoutSeconds=8
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'A verificacao de terminal exige Windows.' }
$uri=$null
if (-not [Uri]::TryCreate($ERPUrl,[UriKind]::Absolute,[ref]$uri) -or
    $uri.Scheme -cne 'https' -or
    -not [string]::IsNullOrEmpty($uri.UserInfo) -or
    -not [string]::IsNullOrEmpty($uri.Query) -or
    -not [string]::IsNullOrEmpty($uri.Fragment) -or
    $uri.AbsolutePath -cne '/' -or
    $uri.Host -in @('localhost','127.0.0.1','::1') -or
    $uri.HostNameType -ne [UriHostNameType]::Dns -or
    $uri.Host -notmatch '^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)+$' -or
    $uri.Host -match '(?i)\.(example|invalid|test)$') {
    throw 'Acesso do segundo caixa exige URL HTTPS de nome DNS da loja, sem usuario, query, IP ou localhost.'
}
if ($uri.Port -in @(80,8080,5432,6379) -or $uri.Port -lt 1024) {
    throw 'Acesso do terminal exige porta HTTPS nao privilegiada, fora do banco/cache/HTTP.'
}
if ($ExpectedCertificateSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$') {
    throw 'Hash SHA-256 do certificado do servidor deve vir de canal independente.'
}
# Reject ALL public/link-local/loopback addresses returned for the hostname.
# A split-horizon record with both public and private IPs is unsafe.
$addresses=@([Net.Dns]::GetHostAddresses($uri.DnsSafeHost))
if ($addresses.Count -eq 0) { throw 'Nome DNS da loja nao resolveu para IP de LAN.' }
foreach ($address in $addresses) {
    if ($address.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetwork) {
        $v=$address.GetAddressBytes()
        $private=($v[0] -eq 10) -or
                 ($v[0] -eq 172 -and $v[1] -ge 16 -and $v[1] -le 31) -or
                 ($v[0] -eq 192 -and $v[1] -eq 168)
        if (-not $private) { throw 'DNS da loja inclui endereco publico, loopback ou nao privado.' }
    } elseif ($address.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetworkV6) {
        $v=$address.GetAddressBytes()
        if (($v[0] -band 0xFE) -ne 0xFC) {
            throw 'IPv6 da loja precisa ser ULA privada, nunca publica ou link-local.'
        }
    } else {
        throw 'DNS retornou familia de rede nao suportada.'
    }
}
# The TCP socket connects to a vetted IP, NOT to a second DNS resolution.
# TLS still authenticates the ORIGINAL host name against SAN + trusted chain.
$tcp=$null
$ssl=$null
$validationSeen=$false
$validationOK=$false
$certificateHashOK=$false
$expected=$ExpectedCertificateSHA256.ToUpperInvariant()
try {
    foreach ($address in $addresses) {
        $tcp=[Net.Sockets.TcpClient]::new($address.AddressFamily)
        try {
            $connect=$tcp.ConnectAsync($address,$uri.Port)
            if (-not $connect.Wait([TimeSpan]::FromSeconds($TimeoutSeconds))) {
                throw 'Conexao LAN HTTPS excedeu o prazo.'
            }
            $tcp.ReceiveTimeout=$TimeoutSeconds*1000
            $tcp.SendTimeout=$TimeoutSeconds*1000
            $ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false,
                [Net.Security.RemoteCertificateValidationCallback]{
                    param($sender,$cert,$chain,$policyErrors)
                    $script:validationSeen=$true
                    if ($null -eq $cert -or
                        $policyErrors -ne [Net.Security.SslPolicyErrors]::None) {
                        return $false
                    }
                    $full=[Security.Cryptography.X509Certificates.X509Certificate2]::new($cert)
                    try {
                        $hash=([BitConverter]::ToString(
                            [Security.Cryptography.SHA256]::Create().ComputeHash($full.RawData)
                        )).Replace('-','')
                        $script:certificateHashOK=($hash -ceq $script:expected)
                        $script:validationOK=$script:certificateHashOK
                        return $script:validationOK
                    } finally { $full.Dispose() }
                })
            # SslProtocols.None delegates protocol policy to current Windows.
            # Never use a callback that accepts an untrusted test certificate.
            $ssl.AuthenticateAsClient($uri.DnsSafeHost,$null,
                [Security.Authentication.SslProtocols]::None,$true)
            if ($ssl.SslProtocol.ToString() -notin @('Tls12','Tls13')) {
                throw 'ERP HTTPS negociou TLS obsoleto.'
            }
            $request="GET / HTTP/1.1`r`nHost: $($uri.Authority)`r`nAccept: text/html`r`nConnection: close`r`n`r`n"
            $bytes=[Text.Encoding]::ASCII.GetBytes($request)
            $ssl.Write($bytes,0,$bytes.Length)
            $ssl.Flush()
            $reader=[IO.StreamReader]::new($ssl,[Text.Encoding]::ASCII,$false,4096,$true)
            try { $status=$reader.ReadLine() } finally { $reader.Dispose() }
            if ($status -notmatch '^HTTP/1\.[01] 200(?:\s|$)') {
                throw 'Resposta HTTPS nao confirmou a pagina inicial da loja (HTTP 200).'
            }
            Write-Output ('PASS: ERP respondeu HTTPS com TLS '+$ssl.SslProtocol+
                ' e certificado confiavel/pinado neste terminal Windows.')
            Write-Output 'NOT PROVEN: firewall do servidor, outro terminal fisico, queda da LAN e restauracao externa.'
            return
        } finally {
            if ($null -ne $ssl) { $ssl.Dispose();$ssl=$null }
            if ($null -ne $tcp) { $tcp.Dispose();$tcp=$null }
        }
    }
} catch {
    if ($validationSeen -and -not $validationOK) {
        throw 'Certificado HTTPS nao corresponde ao SHA-256 aprovado ou nao e confiavel no Windows deste caixa.'
    }
    throw
}
throw 'Nenhum IP privado aprovado respondeu HTTPS.'
