$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/ensure-rapidocr.ps1"

function Assert-True($Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function New-OCRFixture([string]$Path) {
    New-Item -ItemType Directory -Force -Path "$Path/models" | Out-Null
    Set-Content -LiteralPath "$Path/RapidOCR-json.exe" -Value 'fixture'
    Set-Content -LiteralPath "$Path/models/model.onnx" -Value 'fixture'
}

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ("snaptrans-ocr-test-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
    $script:downloads = 0
    function Get-RapidOCRArchive([string]$CacheDirectory) {
        $script:downloads++
        return 'fixture.7z'
    }
    function Expand-RapidOCRArchive([string]$Archive, [string]$Destination) {
        New-OCRFixture "$Destination/RapidOCR-json_v0.2.0"
    }

    $destination = Join-Path $testRoot 'path with spaces/OCR'
    Install-RapidOCR -Destination $destination -CacheDirectory "$testRoot/cache"
    Assert-True (Test-RapidOCRDirectory $destination) 'Fresh install must contain executable and models'
    Assert-True ($script:downloads -eq 1) 'Missing runtime must fetch the archive'

    Install-RapidOCR -Destination $destination -CacheDirectory "$testRoot/cache"
    Assert-True ($script:downloads -eq 1) 'Existing runtime must work offline without downloading'

    $source = Join-Path $testRoot 'existing OCR'
    New-OCRFixture $source
    Install-RapidOCR -Destination "$testRoot/copied" -CacheDirectory "$testRoot/cache" -SourceDirectory $source
    Assert-True (Test-RapidOCRDirectory $source) 'Existing source must remain in place'
    Assert-True (Test-RapidOCRDirectory "$testRoot/copied") 'Existing source must be copied to output'
    Assert-True ($script:downloads -eq 1) 'Local source must not download'

    New-Item -ItemType Directory -Path "$testRoot/incomplete" | Out-Null
    Set-Content -LiteralPath "$testRoot/incomplete/user-file.txt" -Value 'keep'
    $failed = $false
    try { Install-RapidOCR -Destination "$testRoot/incomplete" -CacheDirectory "$testRoot/cache" } catch { $failed = $true }
    Assert-True $failed 'An incomplete existing directory must fail without overwriting it'
    Assert-True (Test-Path "$testRoot/incomplete/user-file.txt") 'Existing files must be preserved'

    function Expand-RapidOCRArchive([string]$Archive, [string]$Destination) { throw 'Extraction failed' }
    $failed = $false
    try { Install-RapidOCR -Destination "$testRoot/failed" -CacheDirectory "$testRoot/cache" } catch { $failed = $true }
    Assert-True $failed 'Extraction failure must stop the build'
    Assert-True (-not (Test-Path "$testRoot/failed")) 'Failed extraction must not install partial output'

    Set-Content -LiteralPath "$testRoot/corrupt.7z" -Value 'invalid'
    $failed = $false
    try { Assert-RapidOCRArchive "$testRoot/corrupt.7z" } catch { $failed = $true }
    Assert-True $failed 'A corrupt archive must fail checksum validation'
    Write-Output 'All OCR setup tests passed.'
}
finally {
    $resolved = [IO.Path]::GetFullPath($testRoot)
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolved.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe test cleanup path' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
