param([string]$Suite = 'desktop', [switch]$SkipKnownLog, [switch]$Repeat)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:Path = "$env:SystemRoot\System32;$env:Path"
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$testArguments = @('-test.v', '-test.timeout=180s')
$suffix = 'native'
if ($SkipKnownLog) {
 $testArguments += '-test.skip=^TestAPILogsKeepBindingContract$'
 $suffix = 'except-known-log'
}
if ($Repeat) {
 $testArguments += '-test.run=TestStage4|TestOpenForStartupRollsBackEntireMigration'
 $testArguments += '-test.count=10'
 $suffix = 'repeat'
}
$process = Start-Process -FilePath (Join-Path $PSScriptRoot "$Suite.test.exe") -ArgumentList $testArguments -WorkingDirectory $PSScriptRoot -RedirectStandardOutput (Join-Path $PSScriptRoot "$Suite-$suffix-stdout.txt") -RedirectStandardError (Join-Path $PSScriptRoot "$Suite-$suffix-stderr.txt") -Wait -PassThru
Write-Output "$Suite native test exit=$($process.ExitCode)"
exit $process.ExitCode
