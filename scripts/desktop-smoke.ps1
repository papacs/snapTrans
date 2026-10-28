[CmdletBinding()]
param([switch]$Automated)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ($Automated) {
    Push-Location $repoRoot
    try {
        & go test ./internal/capture ./internal/desktop
        if ($LASTEXITCODE -ne 0) { throw 'Desktop regression tests failed' }
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    } finally { Pop-Location }
}
Write-Output @'
Windows desktop smoke checklist (PENDING until a person performs each step):
1. Open Settings; record the complete version/build identity and input mode.
2. At 100%, 125% and 150% display scaling: capture text, release the mouse,
   check immediate processing, overlay alignment, long-text scrolling and borders.
3. With a second display left/above the primary (negative coordinates), capture
   on each display and across their boundary. Move a pin between mixed-DPI displays.
   Check its suggested physical bounds, visibility, aspect ratio and close action.
4. Translate a short and a multiline image. Copy the TRANSLATED image into Paint;
   inspect its actual pixels for complete translated text, correct crop and wrapping.
5. Start translation, press Esc during OCR and streaming, then capture again.
   Confirm no old tokens appear in the new result and the hotkeys remain responsive.
6. Force an invalid test model, retry; in direct-image mode use OCR retry and
   verify it changes only that request. Restore a working model and translate again.
7. Disable history recording; translate a NEW sentence and check no entry is added.
   Re-enable and check new entries work. Clear recent entries and restart the app.
8. Swap translation/capture/reply hotkeys, save and exercise all three. Try a key
   occupied by another app; failed saving must retain the previous working keys.

Automated unit/native-memory checks do not mark these visual/hardware cases passed.
This script does not capture the desktop, invoke a model or modify local settings.
'@
