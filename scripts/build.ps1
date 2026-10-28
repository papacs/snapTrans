[CmdletBinding()]
param(
    [ValidatePattern('^[^\\/:*?"<>|\r\n]+\.exe$')]
    [string]$OutputName = 'snapTrans.exe',
    [switch]$SkipFrontend
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'build-common.ps1')
. (Join-Path $PSScriptRoot 'build-identity.ps1')
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$digest = Get-SnapTransSourceDigest -RepoRoot $repoRoot
$identity = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + $digest
$arguments = @('-o', $OutputName, '-ldflags', "-X snaptrans/internal/desktop.buildID=$identity")
if ($SkipFrontend) { $arguments += '-s' }
Invoke-SnapTransWailsBuild -RepoRoot $repoRoot -BuildArguments $arguments
Write-Output "Build: $identity"
