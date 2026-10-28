$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'build-identity.ps1')
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('snaptrans-identity-test-' + [guid]::NewGuid().ToString('N'))
try {
    New-Item -ItemType Directory -Path (Join-Path $fixture 'internal') -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $fixture 'internal/sample.go') -Value 'package sample' -Encoding ascii
    $first = Get-SnapTransSourceDigest -RepoRoot $fixture
    if ($first -ne (Get-SnapTransSourceDigest -RepoRoot $fixture)) { throw 'Identical sources changed identity' }
    $id = '20261028T080000Z-' + $first
    $actual = Assert-SnapTransBuildIdentity -Metadata ('build -ldflags="-X snaptrans/internal/desktop.buildID=' + $id + '"') -ExpectedDigest $first
    if ($actual -ne $id) { throw 'Could not recover build stamp' }
    Add-Content -LiteralPath (Join-Path $fixture 'internal/sample.go') -Value '// changed' -Encoding ascii
    $second = Get-SnapTransSourceDigest -RepoRoot $fixture
    if ($first -eq $second) { throw 'Changed source retained old identity' }
    foreach ($metadata in @('old binary without a stamp', ('build -ldflags="-X snaptrans/internal/desktop.buildID=' + $id + '"'))) {
        $rejected = $false
        try { Assert-SnapTransBuildIdentity -Metadata $metadata -ExpectedDigest $second | Out-Null }
        catch { $rejected = $true }
        if (-not $rejected) { throw 'Stale executable was accepted' }
    }
    Write-Output 'All build identity tests passed.'
} finally {
    $resolved = [IO.Path]::GetFullPath($fixture)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($temp, [StringComparison]::OrdinalIgnoreCase) -or -not ([IO.Path]::GetFileName($resolved)).StartsWith('snaptrans-identity-test-')) { throw 'Unsafe cleanup path' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
