$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:Path = "$env:SystemRoot\System32;$env:Path"
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$results = @()
foreach ($suite in @('desktop','app','diagnostics','persistence','reminder','storage','system','goals','plans','schedule','sessions','settings','subjects','studytime','release')) {
 $process = Start-Process -FilePath (Join-Path $PSScriptRoot "$suite.test.exe") -ArgumentList @('-test.v','-test.timeout=180s') -WorkingDirectory $PSScriptRoot -RedirectStandardOutput (Join-Path $PSScriptRoot "$suite-native-stdout.txt") -RedirectStandardError (Join-Path $PSScriptRoot "$suite-native-stderr.txt") -Wait -PassThru
 $results += "$suite=$($process.ExitCode)"
 Write-Output "$suite native test exit=$($process.ExitCode)"
}
$results | Set-Content -Path (Join-Path $PSScriptRoot 'suite-exit-codes.txt')
if (@($results | Where-Object { $_ -notmatch '=0$' }).Count -gt 0) { exit 1 }
exit 0
