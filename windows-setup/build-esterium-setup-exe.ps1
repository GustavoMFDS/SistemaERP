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
    [switch]$TechnicalPreviewPayload,
    # Use PostgreSQL's official binary ZIP in the signed retail installer.
    [switch]$PostgresBinaryZip,
    # Backward-compatible alias for the unsigned ZIP staging preview.
    [switch]$TechnicalPreviewPostgresBinaryZip,
    [switch]$VMHomologationBuild
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
    $ExpectedPostgresSHA256,$ExpectedGarnetSHA256,$ExpectedServiceWrapperSHA256)
$usePostgresZip=$PostgresBinaryZip -or $TechnicalPreviewPostgresBinaryZip
if (-not $usePostgresZip) { $inputs+=$ExpectedPostgresPublisherThumbprint }
if ($TechnicalPreviewPostgresBinaryZip -and -not $TechnicalPreviewPayload) { throw 'The legacy ZIP-preview switch requires a technical preview payload.' }
if ($usePostgresZip -and $ExpectedPostgresPublisherThumbprint) { throw 'A PostgreSQL ZIP cannot claim the EDB EXE Authenticode signer.' }
$full=-not [string]::IsNullOrWhiteSpace($BundleDir)
if ($TechnicalPreviewPayload -and -not $full) { throw 'Preview payload requires BundleDir.' }
if ($VMHomologationBuild -and (-not $full -or $TechnicalPreviewPayload -or $usePostgresZip)) {
    throw 'VM homologation build requires the full signed-mode bundle and EDB installer.'
}
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
function Export-ArchiveNoticeFiles {
    param([string]$ArchivePath,[string]$Destination,[string]$Component)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive=[IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $entries=@($archive.Entries | Where-Object {
            -not [string]::IsNullOrEmpty($_.Name) -and
            [Regex]::IsMatch($_.Name,'(?i)^(?:(license|licence|notice|copyright|copying|patents?)(?:[._ -].*)?|third.?party.*(?:notice|license|licence).*)$')
        } | Sort-Object FullName)
        if ($entries.Count -gt 300) { throw ($Component+' archive has too many license/notice entries.') }
        $total=[long]0
        $inventory=@($Component+' license/notice files from the exact pinned archive:')
        if ($entries.Count -eq 0) { $inventory+= 'No license-like file names found; static upstream notices are included separately.' }
        foreach ($entry in $entries) {
            $name=$entry.FullName.Replace('\','/')
            $parts=$name.TrimEnd('/').Split('/')
            if ($name.StartsWith('/') -or $name.Contains(':') -or $name.Contains('\') -or
                @($parts | Where-Object { $_ -in @('', '.', '..') }).Count -gt 0 -or
                (($entry.ExternalAttributes -shr 16) -band 0xf000) -eq 0xa000) {
                throw ($Component+' archive contains an unsafe notice path.')
            }
            if ($entry.Length -gt 8MB) { throw ($Component+' notice file exceeds the size limit.') }
            $total+=$entry.Length
            if ($total -gt 32MB) { throw ($Component+' notices exceed the total size limit.') }
            $target=Join-Path $Destination ($name.Replace('/',[IO.Path]::DirectorySeparatorChar))
            $target=[IO.Path]::GetFullPath($target)
            $base=[IO.Path]::GetFullPath($Destination).TrimEnd('\')+'\'
            if (-not $target.StartsWith($base,[StringComparison]::OrdinalIgnoreCase)) {
                throw ($Component+' notice path escaped its output folder.')
            }
            $parent=[IO.Path]::GetDirectoryName($target)
            [IO.Directory]::CreateDirectory($parent) | Out-Null
            $input=$entry.Open()
            $output=[IO.File]::Open($target,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
            try { $input.CopyTo($output) } finally { $input.Dispose(); $output.Dispose() }
            $inventory+=$name
        }
        $inventoryPath=Join-Path $Destination 'archive-notice-inventory.txt'
        [IO.File]::WriteAllLines($inventoryPath,[string[]]$inventory,[Text.UTF8Encoding]::new($false))
        Write-Output ('LICENSE INVENTORY '+$Component+': '+$entries.Count+' candidate files.')
        foreach ($entry in $entries) { Write-Output ('LICENSE FILE '+$Component+': '+$entry.FullName) }
    } finally { $archive.Dispose() }
}
$stage=''
$archive=''
$previewApprovals=''
$releaseApprovals=''
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
        $pgName='postgresql.exe'
        if ($usePostgresZip) {
            $pgName='postgresql-binaries.zip'
            $official='43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281'
            if (-not [IO.Path]::IsPathRooted($PostgresInstaller) -or $PostgresInstaller.StartsWith('\\') -or
                [IO.Path]::GetExtension($PostgresInstaller).ToLowerInvariant() -ne '.zip' -or
                -not (Test-Path -LiteralPath $PostgresInstaller -PathType Leaf) -or
                $ExpectedPostgresSHA256.ToUpperInvariant() -cne $official -or
                (Get-FileHash -LiteralPath $PostgresInstaller -Algorithm SHA256).Hash -cne $official) {
                throw 'PostgreSQL ZIP does not match the independently pinned official SHA-256.'
            }
            if (-not [IO.Path]::IsPathRooted($GarnetPackage) -or
                -not (Test-Path -LiteralPath $GarnetPackage -PathType Leaf) -or
                $ExpectedGarnetSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
                (Get-FileHash -LiteralPath $GarnetPackage -Algorithm SHA256).Hash -cne
                    $ExpectedGarnetSHA256.ToUpperInvariant()) {
                throw 'Garnet ZIP hash mismatch.'
            }
        } else {
            $vendor=@{PostgresInstaller=$PostgresInstaller;ExpectedPostgresSHA256=$ExpectedPostgresSHA256;
                ExpectedPostgresPublisherThumbprint=$ExpectedPostgresPublisherThumbprint;
                GarnetPackage=$GarnetPackage;ExpectedGarnetSHA256=$ExpectedGarnetSHA256}
            & (Join-Path $root 'scripts/check-esterium-offline-dependency-packages.ps1') @vendor | Out-Null
        }
        if ($usePostgresZip) {
            $vendor=@{PostgresInstaller=$PostgresInstaller;ExpectedPostgresSHA256=$ExpectedPostgresSHA256;
                ExpectedPostgresPublisherThumbprint='';PostgresBinaryZip=$true;
                GarnetPackage=$GarnetPackage;ExpectedGarnetSHA256=$ExpectedGarnetSHA256}
            & (Join-Path $root 'scripts/check-esterium-offline-dependency-packages.ps1') @vendor | Out-Null
        }
        if ($ExpectedServiceWrapperSHA256 -cnotmatch '^[A-Fa-f0-9]{64}$' -or
            -not [IO.Path]::IsPathRooted($ServiceWrapperExe) -or
            (Get-FileHash -LiteralPath $ServiceWrapperExe -Algorithm SHA256).Hash -cne
                $ExpectedServiceWrapperSHA256.ToUpperInvariant()) { throw 'NSSM hash mismatch.' }
        $stage=Join-Path ([IO.Path]::GetTempPath()) ('esterium-stage-'+[Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $stage 'vendor') -Force | Out-Null
        Copy-Item -LiteralPath $bundle -Destination (Join-Path $stage 'erp') -Recurse -ErrorAction Stop
        Copy-Item -LiteralPath $PostgresInstaller -Destination (Join-Path (Join-Path $stage 'vendor') $pgName) -ErrorAction Stop
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
        foreach ($item in @(@($pgName,$ExpectedPostgresSHA256),
                            @('garnet.zip',$ExpectedGarnetSHA256),
                            @('nssm.exe',$ExpectedServiceWrapperSHA256))) {
            $digest=(Get-FileHash -LiteralPath (Join-Path (Join-Path $stage 'vendor') $item[0]) -Algorithm SHA256).Hash
            if ($digest -cne $item[1].ToUpperInvariant()) { throw 'Staged vendor hash mismatch.' }
        }
        $embeddedNotices=Join-Path $stage 'third-party-notices'
        $staticNotices=Join-Path $embeddedNotices 'static'
        [IO.Directory]::CreateDirectory($staticNotices) | Out-Null
        Copy-Item -Path (Join-Path $PSScriptRoot 'vendor-notices\*') -Destination $staticNotices -ErrorAction Stop
        $runtimeNotices=Join-Path $bundle 'tools\dotnet'
        foreach ($name in @('LICENSE.txt','ThirdPartyNotices.txt')) {
            $source=Join-Path $runtimeNotices $name
            if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
                throw 'The full ERP bundle is missing .NET runtime license notices.'
            }
            $target=Join-Path (Join-Path $embeddedNotices 'dotnet') $name
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
            Copy-Item -LiteralPath $source -Destination $target -ErrorAction Stop
        }
        $archiveNotices=Join-Path $embeddedNotices 'archives'
        if ($usePostgresZip) {
            Export-ArchiveNoticeFiles -ArchivePath (Join-Path (Join-Path $stage 'vendor') $pgName) `
                -Destination (Join-Path $archiveNotices 'postgresql') -Component 'PostgreSQL 16.15-5 ZIP'
        }
        Export-ArchiveNoticeFiles -ArchivePath (Join-Path (Join-Path $stage 'vendor') 'garnet.zip') `
            -Destination (Join-Path $archiveNotices 'garnet') -Component 'Garnet 2.2.3'
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive=Join-Path ([IO.Path]::GetTempPath()) ('esterium-payload-'+[Guid]::NewGuid().ToString('N')+'.zip')
        [IO.Compression.ZipFile]::CreateFromDirectory($stage,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
    }
    New-Item -ItemType Directory -Path $OutputDir -ErrorAction Stop | Out-Null
    $exeName=if ($TechnicalPreviewPayload) { 'EsteriumERP-Setup-TECHNICAL-PREVIEW.exe' } elseif ($VMHomologationBuild) { 'EsteriumERP-Setup-VM-LAB.exe' } else { 'EsteriumERP-Setup.exe' }
    $pgFormat=if ($usePostgresZip) { 'zip' } else { 'edb' }
    $exe=Join-Path $OutputDir $exeName
    $arguments=@('/nologo','/target:winexe','/platform:x64','/optimize+',
        '/r:System.Windows.Forms.dll','/r:System.Drawing.dll',
        '/r:System.IO.Compression.dll','/r:System.IO.Compression.FileSystem.dll','/r:System.Web.Extensions.dll',
        '/r:System.ServiceProcess.dll','/r:System.Management.dll',('/out:'+$exe))
    if ($full) { $arguments+=('/resource:'+$archive+',Esterium.Payload') }
    if ($TechnicalPreviewPayload) {
        $previewApprovals=Join-Path ([IO.Path]::GetTempPath()) ('esterium-approvals-'+[Guid]::NewGuid().ToString('N')+'.json')
        @{
            manifest=$ApprovedManifestSHA256.ToUpperInvariant()
            pgFormat=$pgFormat
            postgres=$ExpectedPostgresSHA256.ToUpperInvariant()
            pgSigner=if ($usePostgresZip) { '' } else { $ExpectedPostgresPublisherThumbprint.ToUpperInvariant() }
            garnet=$ExpectedGarnetSHA256.ToUpperInvariant()
            nssm=$ExpectedServiceWrapperSHA256.ToUpperInvariant()
        } | ConvertTo-Json -Compress | Set-Content -LiteralPath $previewApprovals -Encoding UTF8
        $arguments+=('/resource:'+$previewApprovals+',Esterium.PreviewApprovals')
    }
    if ($full -and -not $TechnicalPreviewPayload) {
        $releaseApprovals=Join-Path ([IO.Path]::GetTempPath()) ('esterium-release-approvals-'+[Guid]::NewGuid().ToString('N')+'.json')
        @{
            manifest=$ApprovedManifestSHA256.ToUpperInvariant()
            editor=$TrustedPublisherThumbprint.ToUpperInvariant()
            pgFormat=$pgFormat
            postgres=$ExpectedPostgresSHA256.ToUpperInvariant()
            pgSigner=if ($usePostgresZip) { '' } else { $ExpectedPostgresPublisherThumbprint.ToUpperInvariant() }
            garnet=$ExpectedGarnetSHA256.ToUpperInvariant()
            nssm=$ExpectedServiceWrapperSHA256.ToUpperInvariant()
        } | ConvertTo-Json -Compress | Set-Content -LiteralPath $releaseApprovals -Encoding UTF8
        $arguments+=('/resource:'+$releaseApprovals+',Esterium.ReleaseApprovals')
    }
    if ($TechnicalPreviewPayload -or -not $full) { $arguments+='/define:ESTERIUM_PREVIEW' }
    if ($VMHomologationBuild) { $arguments+='/define:ESTERIUM_VM_LAB' }
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumSetup.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourceStage.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourceVendors.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourcePreflight.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumPostgresZipMode.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourcePostgresStage.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourcePostgresAudit.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourceDataStage.cs')
    $arguments+=(Join-Path $PSScriptRoot 'EsteriumOpenSourceSecretsStage.cs')
    & $csc @arguments
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $exe)) { throw 'Windows EXE compilation failed.' }
    $hash=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash
    "SHA256 $hash  $exeName" | Set-Content -LiteralPath (Join-Path $OutputDir 'EsteriumERP-Setup.sha256.txt') -Encoding Ascii
    # The MIT copyright notice travels with the binary; it does not relicense vendors.
    Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination (Join-Path $OutputDir 'LICENSE.txt') -ErrorAction Stop
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'THIRD_PARTY_NOTICES.md') -Destination (Join-Path $OutputDir 'THIRD_PARTY_NOTICES.md') -ErrorAction Stop
    $releaseNotices=Join-Path $OutputDir 'third-party-notices'
    [IO.Directory]::CreateDirectory($releaseNotices) | Out-Null
    if ($full) {
        Copy-Item -Path (Join-Path (Join-Path $stage 'third-party-notices') '*') `
            -Destination $releaseNotices -Recurse -ErrorAction Stop
    } else {
        Copy-Item -Path (Join-Path $PSScriptRoot 'vendor-notices\*') `
            -Destination $releaseNotices -Recurse -ErrorAction Stop
    }
    foreach ($name in @('PostgreSQL-LICENSE.txt','Garnet-2.2.3-LICENSE.txt',
                        'Garnet-2.2.3-NOTICE.md','NSSM-2.24-LICENSE.txt')) {
        if (-not (Test-Path -LiteralPath (Join-Path (Join-Path $releaseNotices 'static') $name) -PathType Leaf) -and
            -not (Test-Path -LiteralPath (Join-Path $releaseNotices $name) -PathType Leaf)) {
            throw ('Required third-party notice was not packaged: '+$name)
        }
    }
    if ($full) {
        foreach ($name in @('LICENSE.txt','ThirdPartyNotices.txt')) {
            if (-not (Test-Path -LiteralPath (Join-Path (Join-Path $releaseNotices 'dotnet') $name) -PathType Leaf)) {
                throw 'Required .NET runtime notice was not packaged.'
            }
        }
    }
    Write-Output "PASS: $exe"
    Write-Output ('PASS: third-party license and notice files packaged in '+$releaseNotices)
    Write-Output "SHA256: $hash"
    if ($TechnicalPreviewPayload) {
        if ($TechnicalPreviewPostgresBinaryZip) {
            Write-Output 'MIT ZIP PREVIEW: ERP and vendors embedded; protected PostgreSQL binary staging only. No initdb, SQL or services; Apply BLOCKED.'
        } else {
            Write-Output 'TECHNICAL PREVIEW: ERP and vendors embedded; one-click verified PLAN available; Apply permanently disabled.'
        }
    } elseif ($VMHomologationBuild) {
        Write-Output 'VM HOMOLOGATION ONLY: VirtualBox-gated, temporary test publisher, not retail, not a release.'
    } elseif ($full) {
        Write-Output ('BUNDLE: ERP, PostgreSQL '+$(if($usePostgresZip){'official ZIP'}else{'EDB installer'})+', Garnet, NSSM and verified release approvals embedded in one executable.')
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
    if ($releaseApprovals -and (Test-Path -LiteralPath $releaseApprovals)) {
        Remove-Item -LiteralPath $releaseApprovals -Force -ErrorAction SilentlyContinue
    }
}
