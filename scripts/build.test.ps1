$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'build-common.ps1')

# Exercise the real build helper, replacing only external execution and waiting.
# No compiler, Wails process, user configuration or network is involved.
$script:attempts = @()
$script:waits = @()
$script:responses = @()
function wails {
    $script:attempts += [pscustomobject]@{ Arguments = @($args); Directory = (Get-Location).Path }
    $index = [Math]::Min($script:attempts.Count - 1, $script:responses.Count - 1)
    $response = $script:responses[$index]
    Write-Output ('fake build attempt ' + $script:attempts.Count + ':')
    if ($response.PSObject.Properties['StandardError']) { Write-Error $response.StandardError }
    Write-Output $response.Message
    $global:LASTEXITCODE = $response.Code
}
function Start-Sleep {
    param([int]$Seconds)
    $script:waits += $Seconds
}
function Assert-BuildTest {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}
function Invoke-BuildTest {
    param([object[]]$Responses, [string[]]$Arguments = @('-s'), [int]$ExpectedAttempts, [bool]$ShouldFail)
    $script:attempts = @()
    $script:waits = @()
    $script:responses = $Responses
    $script:trace = @()
    $caught = $null
    $beforeDirectory = (Get-Location).Path
    $beforePreference = $ErrorActionPreference
    try {
        Invoke-SnapTransWailsBuild -RepoRoot $script:fixture -BuildArguments $Arguments *>&1 | ForEach-Object { $script:trace += $_.ToString() }
    } catch { $caught = $_ }
    Assert-BuildTest ($ShouldFail -eq ($null -ne $caught)) 'Unexpected build success/failure'
    Assert-BuildTest ($script:attempts.Count -eq $ExpectedAttempts) ('Unexpected retry count: ' + $script:attempts.Count)
    Assert-BuildTest ((Get-Location).Path -eq $beforeDirectory) 'Caller working directory was not restored'
    Assert-BuildTest ($ErrorActionPreference -eq $beforePreference) 'Caller ErrorActionPreference was not restored'
    $expectedArguments = @('build') + $Arguments
    foreach ($attempt in $script:attempts) {
        Assert-BuildTest ($attempt.Directory -eq $script:fixture) 'Build ran outside RepoRoot'
        Assert-BuildTest (($attempt.Arguments -join [char]0) -eq ($expectedArguments -join [char]0)) 'Build arguments were changed or split'
    }
    for ($i = 1; $i -le $ExpectedAttempts; $i++) {
        Assert-BuildTest (($script:trace -join "`n") -match ('fake build attempt ' + $i + ':')) 'An attempt output was swallowed'
    }
    if ($ShouldFail) {
        $lastResponse = $Responses[[Math]::Min($ExpectedAttempts - 1, $Responses.Count - 1)]
        Assert-BuildTest ($caught.ToString() -match ('\b' + $lastResponse.Code + '\b')) 'Failure omitted the native exit code'
    }
}

$script:fixture = Join-Path ([IO.Path]::GetTempPath()) ('snaptrans-build-test-' + [guid]::NewGuid().ToString('N'))
$originalExitCode = Get-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue
$originalExitCodeValue = if ($null -ne $originalExitCode) { $originalExitCode.Value }
try {
    New-Item -ItemType Directory -Path $script:fixture | Out-Null
    $sharing = 'The process cannot access the file because it is being used by another process.'
    $temporaryLock = 'snaptrans: open C:\Temp\go-build123456\b001\exe\a.out.exe: ' + $sharing
    $success = [pscustomobject]@{ Code = 0; Message = 'build succeeded' }
    $locked = [pscustomobject]@{ Code = 1; Message = $temporaryLock }
    Invoke-BuildTest -Responses @($locked, $success) -ExpectedAttempts 2 -ShouldFail $false -Arguments @('-s', '-o', 'snapTrans quoted.exe', '-ldflags', '-X snaptrans/internal/desktop.buildID=test')
    Assert-BuildTest (($script:waits -join ',') -eq '1') 'One transient failure must wait once for 1 second'

    $bindingLock = [pscustomobject]@{ Code = 1; Message = ('open C:\Temp\wailsbindings.exe: ' + $sharing) }
    Invoke-BuildTest -Responses @($bindingLock, $success) -ExpectedAttempts 2 -ShouldFail $false
    Assert-BuildTest (($script:waits -join ',') -eq '1') 'Binding executable lock was not retried with bounded waiting'

    $ansiLock = [pscustomobject]@{ Code = 1; Message = ($temporaryLock.Replace('a.out.exe:', ('a.out.exe' + [char]27 + '[0m:'))) }
    Invoke-BuildTest -Responses @($ansiLock, $success) -ExpectedAttempts 2 -ShouldFail $false
    $localized = -join @(0x53e6,0x4e00,0x4e2a,0x7a0b,0x5e8f,0x6b63,0x5728,0x4f7f,0x7528,0x6b64,0x6587,0x4ef6,0xff0c,0x8fdb,0x7a0b,0x65e0,0x6cd5,0x8bbf,0x95ee,0x3002 | ForEach-Object { [char]$_ })
    $localizedLock = [pscustomobject]@{ Code = 1; Message = ('open C:\Temp\wailsbindings.exe: ' + $localized) }
    Invoke-BuildTest -Responses @($localizedLock, $success) -ExpectedAttempts 2 -ShouldFail $false

    $warningSuccess = [pscustomobject]@{ Code = 0; Message = 'build succeeded'; StandardError = 'nonfatal native warning' }
    Invoke-BuildTest -Responses @($warningSuccess) -ExpectedAttempts 1 -ShouldFail $false
    Assert-BuildTest (($script:trace -join "`n") -match 'nonfatal native warning') 'Successful native stderr was swallowed'
    Assert-BuildTest ($script:waits.Count -eq 0) 'Native stderr alone was treated as a failed build'

    foreach ($failure in @(
        [pscustomobject]@{ Code = 2; Message = '# snaptrans: main.go:4: syntax error: unexpected token' },
        [pscustomobject]@{ Code = 5; Message = 'snaptrans: open C:\Temp\go-build123456\b001\exe\a.out.exe: Access is denied.' },
        [pscustomobject]@{ Code = 17; Message = ('snaptrans: open C:\Temp\go-build123456\b001\exe\a.out.exe: Access is denied.' + "`n" + 'open E:\workspace\snapTrans\build\bin\snapTrans.exe: ' + $sharing) },
        [pscustomobject]@{ Code = 17; Message = ('snaptrans: copying C:\Temp\go-build123456\b001\exe\a.out.exe: open E:\workspace\snapTrans\build\bin\snapTrans.exe: ' + $sharing) },
        [pscustomobject]@{ Code = 1; Message = ('open E:\workspace\snapTrans\build\bin\a.out.exe: ' + $sharing) }
    )) {
        Invoke-BuildTest -Responses @($failure) -ExpectedAttempts 1 -ShouldFail $true
        Assert-BuildTest ($script:waits.Count -eq 0) 'An ordinary failure was retried'
    }

    Invoke-BuildTest -Responses @($locked) -ExpectedAttempts 3 -ShouldFail $true
    Assert-BuildTest (($script:waits -join ',') -eq '1,2') 'Persistent transient locks must have exactly 1s and 2s waits'
    Write-Output 'All build retry tests passed.'
} finally {
    if ($null -eq $originalExitCode) { Remove-Variable LASTEXITCODE -Scope Global -ErrorAction SilentlyContinue }
    else { $global:LASTEXITCODE = $originalExitCodeValue }
    $resolved = [IO.Path]::GetFullPath($script:fixture)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($temp, [StringComparison]::OrdinalIgnoreCase) -or -not ([IO.Path]::GetFileName($resolved)).StartsWith('snaptrans-build-test-')) { throw 'Unsafe cleanup path' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
