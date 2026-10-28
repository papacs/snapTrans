# Only retry Windows sharing violations on transient compiler/binding outputs.
function Invoke-SnapTransWailsBuild {
    param(
        [Parameter(Mandatory = $true)][string]$RepoRoot,
        [string[]]$BuildArguments = @()
    )
    $null = Get-Command wails -ErrorAction Stop
    Push-Location -LiteralPath $RepoRoot
    try {
        for ($attempt = 1; $attempt -le 3; $attempt++) {
            $output = New-Object 'System.Collections.Generic.List[string]'
            $previousPreference = $ErrorActionPreference
            try {
                # Windows PowerShell turns native stderr into ErrorRecords.
                # Capture it without mistaking stderr alone for a failed build.
                $ErrorActionPreference = 'Continue'
                $global:LASTEXITCODE = $null
                & wails build @BuildArguments 2>&1 | ForEach-Object {
                    $line = $_.ToString()
                    $output.Add($line)
                    Write-Host $line
                }
                $exitCode = $global:LASTEXITCODE
            } finally { $ErrorActionPreference = $previousPreference }
            if ($null -eq $exitCode) { throw 'Could not start Wails; check that wails.exe is executable.' }
            if ($exitCode -eq 0) { return }

            $text = ($output -join "`n") -replace '\x1b\[[0-9;]*m', ''
            # Match the path being OPENED, not a temporary source mentioned in a
            # copy error whose actual locked destination is the user's app.exe.
            $temporaryOpen = '(?i)\bopen\s+[^\r\n]*[\\/]go-build[^\\/\r\n]+[\\/][^\r\n]*[\\/]a\.out\.exe\s*:|\bopen\s+[^\r\n]*[\\/]wailsbindings\.exe\s*:'
            $sharingViolation = '(?i)being used by another process|sharing violation|\u53e6\u4e00\u4e2a(?:\u7a0b\u5e8f|\u8fdb\u7a0b).{0,12}\u6b63\u5728\u4f7f\u7528|\u8fdb\u7a0b\u65e0\u6cd5\u8bbf\u95ee'
            $temporaryLock = $false
            foreach ($line in ($text -split "`n")) {
                if ($line -match $temporaryOpen -and $line -match $sharingViolation) {
                    $temporaryLock = $true
                    break
                }
            }
            if (-not $temporaryLock -or $attempt -eq 3) {
                throw "Wails build failed with exit code $exitCode (attempt $attempt)."
            }
            Write-Warning "A temporary build executable is locked. Retrying in $attempt second(s) ($attempt/2); no processes will be stopped."
            Start-Sleep -Seconds $attempt
        }
    } finally { Pop-Location }
}
