#requires -Version 5.1
<#
  Read-only offline inspector: validates mandatory files and SHA-256 INSIDE
  the generated integrated preview EXE without extracting or executing vendors.
  The standalone EXE is still a non-installable technical preview.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Exe,
    [Parameter(Mandatory=$true)][string]$ExpectedManifestSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedPostgresSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedGarnetSHA256,
    [Parameter(Mandatory=$true)][string]$ExpectedNssmSHA256
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
foreach($value in @($ExpectedManifestSHA256,$ExpectedPostgresSHA256,$ExpectedGarnetSHA256,$ExpectedNssmSHA256)){
    if($value -cnotmatch '^[0-9a-fA-F]{64}$') { throw 'Expected hashes must be SHA-256 values.' }
}
$path=(Resolve-Path -LiteralPath $Exe -ErrorAction Stop).Path
if([IO.Path]::GetExtension($path).ToLowerInvariant() -ne '.exe') { throw 'Expected Windows executable.' }
$assembly=[Reflection.Assembly]::LoadFrom($path)
$resource=$assembly.GetManifestResourceStream('Esterium.Payload')
if($null -eq $resource){ throw 'Executable has no embedded payload.' }
$pgName='vendor/postgresql.exe'
$metaStream=$assembly.GetManifestResourceStream('Esterium.PreviewApprovals')
if($null -ne $metaStream){
    $reader=[IO.StreamReader]::new($metaStream)
    try{$metadata=$reader.ReadToEnd()|ConvertFrom-Json}finally{$reader.Dispose()}
    if($metadata.pgFormat -eq 'zip'){
        $pgName='vendor/postgresql-binaries.zip'
        $official='43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'
        if($ExpectedPostgresSHA256.ToUpperInvariant() -cne $official -or
           [string]$metadata.postgres -cne $official -or
           -not [string]::IsNullOrEmpty([string]$metadata.pgSigner)){
            throw 'PostgreSQL binary ZIP preview does not match pinned EDB version.'
        }
    } elseif([string]$metadata.pgFormat -notin @('','edb')){
        throw 'Unknown PostgreSQL preview package format.'
    }
}
Add-Type -AssemblyName System.IO.Compression
$zip=$null
try {
    $zip=[IO.Compression.ZipArchive]::new($resource,[IO.Compression.ZipArchiveMode]::Read,$false)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    if($zip.Entries.Count -lt 6 -or $zip.Entries.Count -gt 15000) {throw 'Unexpected ZIP entry count.'}
    $files=@{}
    foreach($entry in $zip.Entries){
        $name=$entry.FullName.Replace('\','/')
        if($name.StartsWith('/') -or $name.Contains(':') -or $name.Length -gt 260 -or
           (([int64]$entry.ExternalAttributes -shr 16) -band 0xF000) -eq 0xA000 -or
           -not $seen.Add($name.TrimEnd('/'))){
            throw 'Unsafe embedded ZIP path.'
        }
        $parts=@($name.TrimEnd('/').Split('/'))
        if($parts.Count -lt 2 -or $parts[0] -notin @('erp','vendor') -or
           @($parts | Where-Object { $_ -in @('', '.', '..') }).Count -gt 0){
            throw 'Embedded ZIP has invalid path.'
        }
        if(-not $name.EndsWith('/')){$files[$name]=$entry}
    }
    foreach($pair in @(
        @{name='erp/bundle-manifest.json';hash=$ExpectedManifestSHA256},
        @{name=$pgName;hash=$ExpectedPostgresSHA256},
        @{name='vendor/garnet.zip';hash=$ExpectedGarnetSHA256},
        @{name='vendor/nssm.exe';hash=$ExpectedNssmSHA256}
    )){
        if(-not $files.ContainsKey($pair.name)){ throw "Missing embedded file: $($pair.name)"}
        $stream=$files[$pair.name].Open()
        $sha=[Security.Cryptography.SHA256]::Create()
        try{
            $actual=[BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-','')
        }finally{$stream.Dispose();$sha.Dispose()}
        if($actual -cne $pair.hash.ToUpperInvariant()){ throw "Embedded hash mismatch: $($pair.name)" }
        Write-Output "PASS: embedded $($pair.name) hash matches."
    }
    foreach($file in @('erp/esterium-server.exe','erp/web/index.html','erp/tools/install-esterium-local-complete.ps1')){
        if(-not $files.ContainsKey($file)){throw "ERP bundle missing $file"}
    }
    Write-Output ('PASS: embedded ERP, PostgreSQL, Garnet, NSSM - '+$files.Count+' files, read-only.')
    Write-Output 'NOT RELEASED: preview permanently blocks installation and is not a signed commercial package.'
}finally{
    if($zip){$zip.Dispose()}else{$resource.Dispose()}
}
