[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][int]$TargetProcessId,
    [ValidateRange(1, 1440)][int]$DurationMinutes = 30,
    [ValidateRange(1, 60)][int]$IntervalSeconds = 15,
    [string]$OutputPath = (Join-Path $PSScriptRoot '../build/runtime-tests/memory.jsonl')
)
$ErrorActionPreference = 'Stop'
$outputFile = [IO.Path]::GetFullPath($OutputPath)
$null = New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($outputFile)) -Force
$encoding = New-Object Text.UTF8Encoding($false)
[IO.File]::WriteAllText($outputFile, '', $encoding)
$target = Get-Process -Id $TargetProcessId
$targetStarted = $target.StartTime
$target.Dispose()
$clock = [Diagnostics.Stopwatch]::StartNew()
$samples = New-Object 'System.Collections.Generic.List[object]'
Write-Output "Sampling runtime $TargetProcessId for $DurationMinutes minute(s): $outputFile"
while ($clock.Elapsed.TotalMinutes -lt $DurationMinutes) {
    $target = Get-Process -Id $TargetProcessId -ErrorAction SilentlyContinue
    if (-not $target) { break }
    if ($target.StartTime -ne $targetStarted) { $target.Dispose(); break }
    $processInfo = Get-CimInstance Win32_Process -Property ProcessId, ParentProcessId, Name
    $scopeIds = @($TargetProcessId)
    do {
        $nextIds = @($processInfo | Where-Object { $scopeIds -contains $_.ParentProcessId -and $scopeIds -notcontains $_.ProcessId } | ForEach-Object { [int]$_.ProcessId })
        $scopeIds += $nextIds
    } while ($nextIds.Count -gt 0)
    $members = @(Get-Process -Id $scopeIds -ErrorAction SilentlyContinue)
    $detail = @($members | ForEach-Object {
        [pscustomobject]@{ name = $_.ProcessName; pid = $_.Id; privateBytes = $_.PrivateMemorySize64; workingSetBytes = $_.WorkingSet64; handles = $_.HandleCount }
    })
    $sample = [pscustomobject]@{
        utc = [DateTime]::UtcNow.ToString('o')
        elapsedSeconds = [Math]::Round($clock.Elapsed.TotalSeconds, 2)
        mainPrivateBytes = $target.PrivateMemorySize64
        mainWorkingSetBytes = $target.WorkingSet64
        totalPrivateBytes = [long](($detail | Measure-Object privateBytes -Sum).Sum)
        totalWorkingSetBytes = [long](($detail | Measure-Object workingSetBytes -Sum).Sum)
        processes = $detail
    }
    $samples.Add($sample)
    [IO.File]::AppendAllText($outputFile, ($sample | ConvertTo-Json -Compress -Depth 5) + [Environment]::NewLine, $encoding)
    $target.Dispose()
    foreach ($member in $members) { $member.Dispose() }
    Start-Sleep -Seconds $IntervalSeconds
}
$clock.Stop()
$summary = [pscustomobject]@{
    targetProcessId = $TargetProcessId
    measuredSeconds = [Math]::Round($clock.Elapsed.TotalSeconds, 2)
    sampleCount = $samples.Count
    first = if ($samples.Count) { $samples[0] } else { $null }
    last = if ($samples.Count) { $samples[$samples.Count - 1] } else { $null }
    mainPrivateRange = ($samples | Measure-Object mainPrivateBytes -Minimum -Maximum)
    totalPrivateRange = ($samples | Measure-Object totalPrivateBytes -Minimum -Maximum)
    note = 'Private memory sums include the app and current descendants. Working-set sums may double-count shared pages. This sampler does not drive UI or force garbage collection.'
}
$summaryFile = [IO.Path]::ChangeExtension($outputFile, '.summary.json')
[IO.File]::WriteAllText($summaryFile, ($summary | ConvertTo-Json -Depth 7), $encoding)
Write-Output "Runtime memory sampling complete: $summaryFile"
