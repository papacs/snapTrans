[CmdletBinding()]
param(
    [string]$Destination = '',
    [string]$CacheDirectory = '',
    [string]$SourceDirectory = ''
)

$ErrorActionPreference = 'Stop'
$script:ocrArchiveHash = '7ad9b283d03436c6cd0296723188699299cb4e5cf9140b410c59543aa5793c40'

function Test-RapidOCRDirectory([string]$Path) {
    return ((Test-Path -LiteralPath "$Path/RapidOCR-json.exe" -PathType Leaf) -and
        (Test-Path -LiteralPath "$Path/models" -PathType Container) -and
        (@(Get-ChildItem -LiteralPath "$Path/models" -Filter '*.onnx' -File -ErrorAction SilentlyContinue).Count -gt 0))
}

function Assert-FileHash([string]$Path, [string]$Expected) {
    # Wails may inherit PSModulePath from PowerShell 7 while invoking Windows
    # PowerShell. Avoid Get-FileHash's module autoload dependency in that case.
    $stream = [IO.File]::OpenRead($Path)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    try { $actual = [BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '') }
    finally { $stream.Dispose(); $sha256.Dispose() }
    if ($actual -ne $Expected) {
        throw "Checksum mismatch: $Path. Remove this cached archive and retry, or provide a complete local OCR directory."
    }
}

function Assert-RapidOCRArchive([string]$Path) {
    Assert-FileHash $Path $script:ocrArchiveHash
}

function Get-VerifiedDownload([string]$Url, [string]$Path, [string]$Hash) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
        $partial = "$Path.$([guid]::NewGuid()).partial"
        try {
            Write-Host "Downloading $Url"
            [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
            $ProgressPreference = 'SilentlyContinue'
            Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $partial -TimeoutSec 600
            Assert-FileHash $partial $Hash
            # Copy only after validation; an interrupted download never becomes a cache hit.
            Copy-Item -LiteralPath $partial -Destination $Path
        }
        catch {
            throw "Could not prepare build dependency from $Url. Check network access and retry. You may also run scripts/ensure-rapidocr.ps1 -SourceDirectory <existing OCR folder>. Details: $_"
        }
        finally {
            if (Test-Path -LiteralPath $partial) { Remove-Item -LiteralPath $partial -Force }
        }
    }
    Assert-FileHash $Path $Hash
    return $Path
}

function Get-RapidOCRArchive([string]$CacheDirectory) {
    return Get-VerifiedDownload `
        'https://github.com/hiroi-sora/RapidOCR-json/releases/download/v0.2.0/RapidOCR-json_v0.2.0.7z' `
        (Join-Path $CacheDirectory 'RapidOCR-json_v0.2.0.7z') $script:ocrArchiveHash
}

function Expand-RapidOCRArchive([string]$Archive, [string]$Destination) {
    # Windows inbox tar cannot decode this archive's LZMA stream. Use a pinned
    # 7-Zip binary package; tar is only used for its gzip-compressed container.
    $cache = Split-Path -Parent $Archive
    $toolArchive = Get-VerifiedDownload 'https://registry.npmjs.org/7zip-bin/-/7zip-bin-5.2.0.tgz' `
        (Join-Path $cache '7zip-bin-5.2.0.tgz') '04f9738ee00be8be53f5d06254a9e6e4b5117b17f8b74058477d870e62764a7d'
    $toolDirectory = Join-Path $Destination 'extractor'
    New-Item -ItemType Directory -Force -Path $toolDirectory | Out-Null
    & tar.exe -xf $toolArchive -C $toolDirectory
    if ($LASTEXITCODE -ne 0) { throw 'Could not unpack the OCR extraction tool.' }
    & "$toolDirectory/package/win/x64/7za.exe" x $Archive "-o$Destination" -y | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not extract RapidOCR.' }
}

function Install-RapidOCR([string]$Destination, [string]$CacheDirectory, [string]$SourceDirectory = '') {
    if (Test-RapidOCRDirectory $Destination) {
        Write-Host "Using existing RapidOCR: $Destination"
        return
    }
    if (Test-Path -LiteralPath $Destination) {
        throw "Incomplete OCR directory: $Destination. Restore its executable and models, or rename it before retrying. Existing files have been preserved."
    }
    if ($SourceDirectory -and -not (Test-RapidOCRDirectory $SourceDirectory)) {
        throw "OCR source must contain RapidOCR-json.exe and models/*.onnx: $SourceDirectory"
    }

    $cacheRoot = [IO.Path]::GetFullPath($CacheDirectory)
    $stage = Join-Path $cacheRoot ("extract-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    try {
        $prepared = Join-Path $stage 'RapidOCR-json_v0.2.0'
        if ($SourceDirectory) {
            Copy-Item -LiteralPath $SourceDirectory -Destination $prepared -Recurse
        } else {
            $archive = Get-RapidOCRArchive $cacheRoot
            Expand-RapidOCRArchive $archive $stage
        }
        if (-not (Test-RapidOCRDirectory $prepared)) { throw 'OCR archive is missing its executable or models.' }
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
        Copy-Item -LiteralPath $prepared -Destination $Destination -Recurse
        Write-Host "RapidOCR ready: $Destination"
    }
    finally {
        $resolvedStage = [IO.Path]::GetFullPath($stage)
        $cachePrefix = $cacheRoot.TrimEnd('\') + '\'
        if (-not $resolvedStage.StartsWith($cachePrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe extraction cleanup path' }
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    if (-not $Destination) { $Destination = Join-Path $repoRoot 'build/bin/RapidOCR-json_v0.2.0' }
    if (-not $CacheDirectory) { $CacheDirectory = Join-Path $repoRoot 'build/ocr-cache' }
    if (-not $SourceDirectory -and (Test-RapidOCRDirectory "$repoRoot/RapidOCR-json_v0.2.0")) {
        $SourceDirectory = "$repoRoot/RapidOCR-json_v0.2.0"
    }
    Install-RapidOCR -Destination ([IO.Path]::GetFullPath($Destination)) -CacheDirectory $CacheDirectory -SourceDirectory $SourceDirectory
}
