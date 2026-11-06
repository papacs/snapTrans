[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][int]$TargetProcessId,
    [Parameter(Mandatory = $true)][string]$ExpectedExecutablePath,
    [Parameter(Mandatory = $true)][string]$ExpectedExecutableSHA256,
    [ValidateRange(1, 1440)][int]$DurationMinutes = 20,
    [ValidateRange(1, 60)][int]$IntervalSeconds = 2,
    [ValidateRange(1, 60)][int]$TreeRefreshSeconds = 5,
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '../build/runtime-tests/20261106-live')
)
$ErrorActionPreference = 'Stop'
$outputDir = [IO.Path]::GetFullPath($OutputDirectory)
$null = New-Item -ItemType Directory -Path $outputDir -Force
$encoding = New-Object Text.UTF8Encoding($false)
$samplesFile = Join-Path $outputDir 'memory.jsonl'
$phasesFile = Join-Path $outputDir 'phases.jsonl'
if (Test-Path -LiteralPath $samplesFile) { throw "Refusing to overwrite $samplesFile" }
$rootInfo = Get-CimInstance Win32_Process -Filter "ProcessId=$TargetProcessId"
if (-not $rootInfo) { throw 'Target process is not running' }
$expectedPath = [IO.Path]::GetFullPath($ExpectedExecutablePath)
if (-not [string]::Equals($rootInfo.ExecutablePath, $expectedPath, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Target executable path does not match the expected runtime'
}
$executableStream = [IO.File]::OpenRead($expectedPath)
$sha256 = [Security.Cryptography.SHA256]::Create()
try { $actualHash = [BitConverter]::ToString($sha256.ComputeHash($executableStream)).Replace('-', '') }
finally { $sha256.Dispose(); $executableStream.Dispose() }
if ($actualHash -ne $ExpectedExecutableSHA256) { throw 'Target executable hash does not match' }
$rootProcess = Get-Process -Id $TargetProcessId
try { $rootStartTicks = $rootProcess.StartTime.ToUniversalTime().Ticks } finally { $rootProcess.Dispose() }
$rootCimTicks = $rootInfo.CreationDate.ToUniversalTime().Ticks

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class SnapTransActiveMemoryGui {
    [DllImport("user32.dll")]
    public static extern uint GetGuiResources(IntPtr process, uint flags);
}
'@

$metadata = [ordered]@{
    startedUtc = [DateTime]::UtcNow.ToString('o')
    targetPid = $TargetProcessId
    targetStartedUtc = $rootInfo.CreationDate.ToUniversalTime().ToString('o')
    executablePath = $expectedPath
    executableSHA256 = $actualHash
    durationMinutes = $DurationMinutes
    intervalSeconds = $IntervalSeconds
    treeRefreshSeconds = $TreeRefreshSeconds
    phaseFile = $phasesFile
    note = 'Read-only counters; no UI actions or forced GC. Ownership uses parent PID plus CIM creation time, retaining verified descendants until exit. Working-set sums can double-count shared pages.'
}
[IO.File]::WriteAllText((Join-Path $outputDir 'metadata.json'), ($metadata | ConvertTo-Json), $encoding)
[IO.File]::WriteAllText($samplesFile, '', $encoding)
if (-not (Test-Path -LiteralPath $phasesFile)) { [IO.File]::WriteAllText($phasesFile, '', $encoding) }

$owned = @{}
$clock = [Diagnostics.Stopwatch]::StartNew()
$nextTreeRefresh = 0.0
$nextSample = 0.0
$phase = 'unmarked'
$sampleCount = 0
$first = $null
$last = $null
$peakPrivateBytes = 0L
$peakHandles = 0L
$peakGdi = 0L
$stopReason = 'duration-complete'
Write-Output "Sampling PID $TargetProcessId; phase markers: $phasesFile"
try {
    while ($clock.Elapsed.TotalMinutes -lt $DurationMinutes) {
        $rootProcess = Get-Process -Id $TargetProcessId -ErrorAction SilentlyContinue
        if (-not $rootProcess) { $stopReason = 'target-exited'; break }
        try { $sameRoot = $rootProcess.StartTime.ToUniversalTime().Ticks -eq $rootStartTicks } finally { $rootProcess.Dispose() }
        if (-not $sameRoot) { $stopReason = 'target-identity-changed'; break }

        if ($clock.Elapsed.TotalSeconds -ge $nextTreeRefresh) {
            $snapshot = @{}
            Get-CimInstance Win32_Process -Property ProcessId, ParentProcessId, Name, CreationDate | ForEach-Object {
                $snapshot[[int]$_.ProcessId] = $_
            }
            $rootRow = $snapshot[$TargetProcessId]
            if (-not $rootRow -or $rootRow.CreationDate.ToUniversalTime().Ticks -ne $rootCimTicks) {
                $stopReason = 'target-identity-changed'; break
            }
            foreach ($memberId in @($owned.Keys)) {
                $row = $snapshot[$memberId]
                if (-not $row -or $row.CreationDate.ToUniversalTime().Ticks -ne $owned[$memberId].cimTicks) {
                    $owned.Remove($memberId)
                }
            }
            if (-not $owned.ContainsKey($TargetProcessId)) {
                $owned[$TargetProcessId] = @{ row = $rootRow; cimTicks = $rootCimTicks; startTicks = $rootStartTicks }
            }
            do {
                $added = $false
                foreach ($row in $snapshot.Values) {
                    $memberId = [int]$row.ProcessId
                    $parent = $owned[[int]$row.ParentProcessId]
                    if (-not $owned.ContainsKey($memberId) -and $parent -and
                        $row.CreationDate.ToUniversalTime().Ticks -ge $parent.cimTicks) {
                        $owned[$memberId] = @{ row = $row; cimTicks = $row.CreationDate.ToUniversalTime().Ticks; startTicks = $null }
                        $added = $true
                    }
                }
            } while ($added)
            $nextTreeRefresh += $TreeRefreshSeconds
        }

        if ($clock.Elapsed.TotalSeconds -lt $nextSample) {
            $nextWake = [Math]::Min($nextSample, $nextTreeRefresh)
            Start-Sleep -Milliseconds ([int][Math]::Max(1, ($nextWake - $clock.Elapsed.TotalSeconds) * 1000))
            continue
        }

        $detail = @()
        $errors = @()
        foreach ($memberId in @($owned.Keys)) {
            $member = Get-Process -Id $memberId -ErrorAction SilentlyContinue
            if (-not $member) { continue }
            try {
                $identity = $owned[$memberId]
                $startedTicks = $member.StartTime.ToUniversalTime().Ticks
                # CIM timestamps have microsecond precision; .NET retains finer ticks.
                if ($null -eq $identity.startTicks -and [Math]::Abs($startedTicks - $identity.cimTicks) -lt 10000) {
                    $identity.startTicks = $startedTicks
                }
                if ($identity.startTicks -ne $startedTicks) { continue }
                $detail += [pscustomobject]@{
                    pid = $memberId; parentPid = [int]$identity.row.ParentProcessId; name = $member.ProcessName
                    startedUtc = $member.StartTime.ToUniversalTime().ToString('o')
                    privateBytes = $member.PrivateMemorySize64; workingSetBytes = $member.WorkingSet64
                    handles = $member.HandleCount
                    gdiObjects = [SnapTransActiveMemoryGui]::GetGuiResources($member.Handle, 0)
                    userObjects = [SnapTransActiveMemoryGui]::GetGuiResources($member.Handle, 1)
                }
            } catch { $errors += "pid=$memberId type=$($_.Exception.GetType().Name)" } finally { $member.Dispose() }
        }
        try {
            $marker = Get-Content -LiteralPath $phasesFile -Tail 1 | ConvertFrom-Json
            if ($marker.phase) { $phase = [string]$marker.phase }
        } catch { } # An incomplete concurrent marker never interrupts measurement.
        $main = $detail | Where-Object pid -eq $TargetProcessId | Select-Object -First 1
        if (-not $main) { $stopReason = 'target-exited-during-sample'; break }
        $sample = [pscustomobject]@{
            utc = [DateTime]::UtcNow.ToString('o'); elapsedSeconds = [Math]::Round($clock.Elapsed.TotalSeconds, 3); phase = $phase
            mainPrivateBytes = $main.privateBytes; mainWorkingSetBytes = $main.workingSetBytes
            mainHandles = $main.handles; mainGdiObjects = $main.gdiObjects; mainUserObjects = $main.userObjects
            treePrivateBytes = [long](($detail | Measure-Object privateBytes -Sum).Sum)
            treeWorkingSetBytes = [long](($detail | Measure-Object workingSetBytes -Sum).Sum)
            treeHandles = [long](($detail | Measure-Object handles -Sum).Sum)
            treeGdiObjects = [long](($detail | Measure-Object gdiObjects -Sum).Sum)
            treeUserObjects = [long](($detail | Measure-Object userObjects -Sum).Sum)
            processCount = $detail.Count; processes = $detail; errors = $errors
        }
        [IO.File]::AppendAllText($samplesFile, ($sample | ConvertTo-Json -Compress -Depth 5) + [Environment]::NewLine, $encoding)
        if ($null -eq $first) { $first = $sample }
        $last = $sample
        $sampleCount++
        $peakPrivateBytes = [Math]::Max($peakPrivateBytes, $sample.treePrivateBytes)
        $peakHandles = [Math]::Max($peakHandles, $sample.treeHandles)
        $peakGdi = [Math]::Max($peakGdi, $sample.treeGdiObjects)
        $nextSample += $IntervalSeconds
        $nextWake = [Math]::Min($nextSample, $nextTreeRefresh)
        Start-Sleep -Milliseconds ([int][Math]::Max(1, ($nextWake - $clock.Elapsed.TotalSeconds) * 1000))
    }
} catch { $stopReason = 'sampler-error'; throw } finally {
    $clock.Stop()
    $summary = [ordered]@{
        endedUtc = [DateTime]::UtcNow.ToString('o'); stopReason = $stopReason
        measuredSeconds = [Math]::Round($clock.Elapsed.TotalSeconds, 3); sampleCount = $sampleCount
        peakTreePrivateBytes = $peakPrivateBytes; peakTreeHandles = $peakHandles; peakTreeGdiObjects = $peakGdi
        first = $first; last = $last
    }
    [IO.File]::WriteAllText((Join-Path $outputDir 'summary.json'), ($summary | ConvertTo-Json -Depth 7), $encoding)
}
Write-Output "Sampling complete ($stopReason): $samplesFile"
