[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$')]
    [string]$Version,

    [string]$RapidOCRSource = "build/bin/RapidOCR-json_v0.2.0",

    [string]$OutputDirectory = "dist",

    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$normalizedVersion = $Version.TrimStart("v")
$packageName = "snapTrans-v$normalizedVersion-windows-x64"

$appVersionPath = Join-Path $repoRoot "internal/desktop/app.go"
$appSource = Get-Content -Raw -LiteralPath $appVersionPath
$versionMatch = [regex]::Match($appSource, 'const appVersion = "([^"]+)"')
if (-not $versionMatch.Success) {
    throw "Could not read appVersion from $appVersionPath"
}
if ($versionMatch.Groups[1].Value -ne $normalizedVersion) {
    throw "Requested version $normalizedVersion does not match appVersion $($versionMatch.Groups[1].Value)"
}

if (-not [IO.Path]::IsPathRooted($RapidOCRSource)) {
    $RapidOCRSource = Join-Path $repoRoot $RapidOCRSource
}
$rapidOCRPath = (Resolve-Path -LiteralPath $RapidOCRSource).Path
$rapidOCRExecutable = Join-Path $rapidOCRPath "RapidOCR-json.exe"
$rapidOCRModels = Join-Path $rapidOCRPath "models"
if (-not (Test-Path -LiteralPath $rapidOCRExecutable -PathType Leaf)) {
    throw "RapidOCR-json.exe was not found at $rapidOCRExecutable"
}
if (-not (Test-Path -LiteralPath $rapidOCRModels -PathType Container)) {
    throw "RapidOCR models were not found at $rapidOCRModels"
}

if (-not $SkipBuild) {
    Push-Location $repoRoot
    try {
        & wails build
        if ($LASTEXITCODE -ne 0) {
            throw "wails build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

$executablePath = Join-Path $repoRoot "build/bin/snapTrans.exe"
if (-not (Test-Path -LiteralPath $executablePath -PathType Leaf)) {
    throw "snapTrans.exe was not found at $executablePath"
}

if (-not [IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot $OutputDirectory
}
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$packageRoot = [IO.Path]::GetFullPath((Join-Path $outputRoot $packageName))
$archivePath = [IO.Path]::GetFullPath((Join-Path $outputRoot "$packageName.zip"))
$checksumPath = [IO.Path]::GetFullPath((Join-Path $outputRoot "SHA256SUMS.txt"))

$outputPrefix = $outputRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
if (-not $packageRoot.StartsWith($outputPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Package path escaped the output directory: $packageRoot"
}

New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null
if (Test-Path -LiteralPath $packageRoot) {
    Remove-Item -LiteralPath $packageRoot -Recurse -Force
}
if (Test-Path -LiteralPath $archivePath) {
    Remove-Item -LiteralPath $archivePath -Force
}
if (Test-Path -LiteralPath $checksumPath) {
    Remove-Item -LiteralPath $checksumPath -Force
}

New-Item -ItemType Directory -Path $packageRoot | Out-Null
Copy-Item -LiteralPath $executablePath -Destination (Join-Path $packageRoot "snapTrans.exe")
Copy-Item -LiteralPath $rapidOCRPath -Destination (Join-Path $packageRoot "RapidOCR-json_v0.2.0") -Recurse
Copy-Item -LiteralPath (Join-Path $repoRoot "LICENSE") -Destination $packageRoot
Copy-Item -LiteralPath (Join-Path $repoRoot "README.md") -Destination $packageRoot
Copy-Item -LiteralPath (Join-Path $repoRoot "THIRD-PARTY-NOTICES.md") -Destination $packageRoot
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/RELEASE-NOTES.md") -Destination (Join-Path $packageRoot "RELEASE-NOTES.md")

Compress-Archive -LiteralPath $packageRoot -DestinationPath $archivePath -CompressionLevel Optimal
$archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
"$archiveHash  $([IO.Path]::GetFileName($archivePath))" | Set-Content -LiteralPath $checksumPath -Encoding ascii

Write-Output "Package: $archivePath"
Write-Output "Checksum: $checksumPath"
