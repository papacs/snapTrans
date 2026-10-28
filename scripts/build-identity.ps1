# Shared by release packaging and its isolated tests. Never reads local settings.
function Get-SnapTransSourceDigest {
    param([Parameter(Mandatory = $true)][string]$RepoRoot)
    $root = [IO.Path]::GetFullPath($RepoRoot).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $files = @()
    foreach ($directory in @('internal', 'frontend/src', 'frontend/public', 'build/windows')) {
        $path = Join-Path $root $directory
        if (Test-Path -LiteralPath $path -PathType Container) {
            $files += Get-ChildItem -LiteralPath $path -File -Recurse
        }
    }
    $files += Get-ChildItem -LiteralPath $root -File | Where-Object { $_.Extension -eq '.go' -or $_.Name -in @('go.mod', 'go.sum', 'wails.json') }
    $frontend = Join-Path $root 'frontend'
    if (Test-Path -LiteralPath $frontend) {
        $files += Get-ChildItem -LiteralPath $frontend -File | Where-Object { $_.Extension -in @('.json', '.ts', '.js', '.cjs') }
    }
    # Framework/NLS and modern .NET/ICU sort punctuation differently. Build
    # identity must depend on source bytes, not the invoking PowerShell version.
    $paths = [System.Collections.Generic.SortedSet[string]]::new([StringComparer]::Ordinal)
    foreach ($file in $files) { $null = $paths.Add($file.FullName) }
    $lines = @($paths | ForEach-Object {
        $relative = $_.Substring($root.Length + 1).Replace('\', '/')
        $relative + ':' + (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash.ToLowerInvariant()
    })
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes(($lines -join "`n"))))).Replace('-', '').ToLowerInvariant().Substring(0, 16)
    } finally { $sha.Dispose() }
}

function Assert-SnapTransBuildIdentity {
    param([string]$Metadata, [string]$ExpectedDigest)
    $match = [regex]::Match($Metadata, 'snaptrans/internal/desktop\.buildID=([0-9]{8}T[0-9]{6}Z-([a-f0-9]{16}))')
    if (-not $match.Success -or $match.Groups[2].Value -ne $ExpectedDigest) {
        throw 'Executable is unstamped or does not match the current sources. Run package-release.ps1 without -SkipBuild.'
    }
    return $match.Groups[1].Value
}
