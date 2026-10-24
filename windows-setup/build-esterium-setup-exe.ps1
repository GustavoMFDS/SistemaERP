#requires -Version 5.1
<# Build EsteriumERP-Setup.exe with optional all-in-one approved payload.
   No compiler/toolchain is installed on the customer's PC.
   A compiled EXE is NOT an authenticated commercial release until signed. #>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$OutputDir,
    [string]$BundleDir='',
    [string]$ApprovedManifestSHA256='',
    [string]$TrustedPublisherThumbprint='',
    [string]$PostgresInstaller='',
    [string]$ExpectedPostgresSHA256='',
    [string]$ExpectedPostgresPublisherThumbprint='',
    [string]$GarnetPackage='',
    [string]$ExpectedGarnetSHA256='',
    [string]$ServiceWrapperExe='',
    [string]$ExpectedServiceWrapperSHA256='',
    # Test artifact only: embeds real unsigned ERP without enabling Apply.
    [switch]$TechnicalPreviewPayload
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'Windows build machine required.' }
$root=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$csc=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not (Test-Path -LiteralPath $csc)) { throw '.NET Framework C# compiler missing.' }
if (-not [IO.Path]::IsPathRooted($OutputDir) -or $OutputDir.StartsWith('\\') -or (Test-Path -LiteralPath $OutputDir)) {
    throw 'OutputDir must be a new absolute local directory.'
}
$inputs=@($PostgresInstaller,$GarnetPackage,$ServiceWrapperExe,$ApprovedManifestSHA256,
    $ExpectedPostgresSHA256,$ExpectedPostgresPublisherThumbprint,
    $ExpectedGarnetSHA256,$ExpectedServiceWrapperSHA256)
$full=-not [string]::IsNullOrWhiteSpace($BundleDir)
if ($TechnicalPreviewPayload -and -not $full) { throw 'Preview payload requires BundleDir.' }
if ($TechnicalPreviewPayload -and $TrustedPublisherThumbprint -ne '') {
    throw 'Technical preview must not claim a commercial publisher approval.'
}
if (-not $TechnicalPreviewPayload -and $full -and [string]::IsNullOrWhiteSpace($TrustedPublisherThumbprint)) {
    throw 'Commercial bundle requires independently approved signer.'
}
if (-not $full -and $TrustedPublisherThumbprint -ne '') {
    throw 'Publisher input requires commercial bundle.'
}
if (-not $full -and @($inputs | Where-Object { $_ -ne '' }).Count -gt 0) {
    throw 'Vendor inputs require an ERP bundle.'
}
if ($full -and @($inputs | Where-Object { [string]::IsNullOrWhiteSpace($_) }).Count -gt 0) {
    throw 'Complete bundle requires externally approved publisher IDs and SHA-256 values.'
}
$stage=''
$archive=''
$previewApprovals=''
try {
    if ($full) {
        $bundle=(Resolve-Path -LiteralPath $BundleDir).Path
        $verify=@{BundleDir=$bundle;ExpectedManifestSHA256=$ApprovedManifestSHA256;
            ExpectedPublisherThumbprint=$TrustedPublisherThumbprint}
        if ($TechnicalPreviewPayload) {
            & (Join-Path $root 'scripts/verify-esterium-bundle.ps1') -BundleDir $bundle | Out-Null
            if ((Get-FileHash -LiteralPath (Join-Path $bundle 'bundle-manifest.json') -Algorithm SHA256).Hash -cne
                    $ApprovedManifestSHA256.ToUpperInvariant()) { throw 'ERP preview manifest SHA-256 mismatch.' }
        } else {
            & (Join-Path $root 'scripts/check-esterium-signed-release.ps1') @verify | Out-Null
        }
        $vendor=@{PostgresInstaller=$PostgresInstaller;ExpectedPostgresSHA256=$ExpectedPostgresSHA256;
            ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint;
            GarnetPackage=$GarnetPackage;ExpectedGarnetSHA256=$ExpectedGarnetSHA256}
        & (Join-Path $root 'scripts/check-esterium-offline-dependency-packages.ps1') @vendor | Out-Null
        if ($ExpectedServiceWrapperSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
            -not [IO.Path]::IsPathRooted($ServiceWrapperExe) -or
            (Get-FileHash -LiteralPath $ServiceWrapperExe -Algorithm SHA256).Hash -cne
                $ExpectedServiceWrapperSHA256.ToUpperInvariant()) { throw 'NSSM hash mismatch.' }
        $stage=Join-Path ([IO.Path]::GetTempPath()) ('esterium-stage-'+[Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $stage 'vendor') -Force | Out-Null
        Copy-Item -LiteralPath $bundle -Destination (Join-Path $stage 'erp') -Recurse -ErrorAction Stop
        Copy-Item -LiteralPath $PostgresInstaller -Destination (Join-Path $stage 'vendor/postgresql.exe') -ErrorAction Stop
        Copy-Item -LiteralPath $GarnetPackage -Destination (Join-Path $stage 'vendor/garnet.zip') -ErrorAction Stop
        Copy-Item -LiteralPath $ServiceWrapperExe -Destination (Join-Path $stage 'vendor/nssm.exe') -ErrorAction Stop
        $verify.BundleDir=Join-Path $stage 'erp'
        if ($TechnicalPreviewPayload) {
            & (Join-Path $root 'scripts/verify-esterium-bundle.ps1') -BundleDir $verify.BundleDir | Out-Null
            if ((Get-FileHash -LiteralPath (Join-Path $verify.BundleDir 'bundle-manifest.json') -Algorithm SHA256).Hash -cne
                    $ApprovedManifestSHA256.ToUpperInvariant()) { throw 'Staged ERP preview manifest SHA-256 mismatch.' }
        } else {
            & (Join-Path $root 'scripts/check-esterium-signed-release.ps1') @verify | Out-Null
        }
        foreach ($item in @(@('postgresql.exe',$ExpectedPostgresSHA256),
                            @('garnet.zip',$ExpectedGarnetSHA256),
                            @('nssm.exe',$ExpectedServiceWrapperSHA256))) {
            $digest=(Get-FileHash -LiteralPath (Join-Path (Join-Path $stage 'vendor') $item[0]) -Algorithm SHA256).Hash
            if ($digest -cne $item[1].ToUpperInvariant()) { throw 'Staged vendor hash mismatch.' }
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive=Join-Path ([IO.Path]::GetTempPath()) ('esterium-payload-'+[Guid]::NewGuid().ToString('N')+'.zip')
        [IO.Compression.ZipFile]::CreateFromDirectory($stage,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
    }
    New-Item -ItemType Directory -Path $OutputDir -ErrorAction Stop | Out-Null
    $exeName=if ($TechnicalPreviewPayload) { 'EsteriumERP-Setup-TECHNICAL-PREVIEW.exe' } else { 'EsteriumERP-Setup.exe' }
    $exe=Join-Path $OutputDir $exeName
    $arguments=@('/nologo','/target:winexe','/platform:x64','/optimize+',
        '/r:System.Windows.Forms.dll','/r:System.Drawing.dll',
        '/r:System.IO.Compression.dll','/r:System.Web.Extensions.dll',('/out:'+$exe))
    if ($full) { $arguments+=('/resource:'+$archive+',Esterium.Payload') }
    if ($TechnicalPreviewPayload) {
        $previewApprovals=Join-Path ([IO.Path]::GetTempPath()) ('esterium-approvals-'+[Guid]::NewGuid().ToString('N')+'.json')
        @{
            manifest=$ApprovedManifestSHA256.ToUpperInvariant()
            postgres=$ExpectedPostgresSHA256.ToUpperInvariant()
            pgSigner=$ExpectedPostgresPublisherThumbprint.ToUpperInvariant()
            garnet=$ExpectedGarnetSHA256.ToUpperInvariant()
            nssm=$ExpectedServiceWrapperSHA256.ToUpperInvariant()
        } | ConvertTo-Json -Compress | Set-Content -LiteralPath $previewApprovals -Encoding UTF8
        $arguments+=('/resource:'+$previewApprovals+',Esterium.PreviewApprovals')
    }
    if ($TechnicalPreviewPayload -or -not $full) { $arguments+='/define:ESTERIUM_PREVIEW' }
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumSetup.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourceStage.cs')
    & $csc @arguments
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $exe)) { throw 'Windows EXE compilation failed.' }
    $hash=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash
    "SHA256 $hash  $exeName" | Set-Content -LiteralPath (Join-Path $OutputDir 'EsteriumERP-Setup.sha256.txt') -Encoding Ascii
    Write-Output "PASS: $exe"
    Write-Output "SHA256: $hash"
    if ($TechnicalPreviewPayload) {
        Write-Output 'TECHNICAL PREVIEW: ERP and vendors embedded; one-click verified PLAN available; Apply permanently disabled.'
    } elseif ($full) {
        Write-Output 'BUNDLE: ERP, PostgreSQL, Garnet and NSSM embedded in one executable.'
        Write-Output 'NOT RELEASED: Authenticode-sign the FINAL EXE and validate clean-Windows physical pilot.'
    } else {
        Write-Output 'PREVIEW ONLY: executable has no embedded payload; retail installation blocked.'
    }
} finally {
    if ($stage -and (Test-Path -LiteralPath $stage)) {
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    }
    if ($archive -and (Test-Path -LiteralPath $archive)) {
        Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
    }
    if ($previewApprovals -and (Test-Path -LiteralPath $previewApprovals)) {
        Remove-Item -LiteralPath $previewApprovals -Force -ErrorAction SilentlyContinue
    }
}
