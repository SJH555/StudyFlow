$ErrorActionPreference = 'Stop'
$qaRoot = 'D:\CLink\Desktop\studyflow\db-qa-20261115'
$cases = Get-Content -LiteralPath (Join-Path $qaRoot 'package-manifest.json') -Raw | ConvertFrom-Json
$results = foreach ($case in $cases) {
    $folder = Join-Path $qaRoot ('package-' + $case.name)
    $exe = Join-Path $folder 'StudyFlow.exe'
    $process = Start-Process -FilePath $exe -WorkingDirectory $folder -WindowStyle Hidden -PassThru
    $exited = $process.WaitForExit(4000)
    $exitCode = if ($exited) { $process.ExitCode } else { $null }
    if (-not $exited) { Stop-Process -Id $process.Id; $process.WaitForExit() }
    $db = Join-Path $folder 'data\study.db'
    $hash = if (Test-Path -LiteralPath $db -PathType Leaf) { (Get-FileHash -LiteralPath $db -Algorithm SHA256).Hash.ToLowerInvariant() } else { $null }
    $log = Get-ChildItem -LiteralPath (Join-Path $folder 'logs') -Filter '*.log' -ErrorAction SilentlyContinue | Get-Content -Raw
    [pscustomobject]@{name=$case.name; exited=$exited; exitCode=$exitCode; unchanged=($hash -eq $case.before_hash); log=$log}
}
$results | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $qaRoot 'package-results.json')
$results | Select-Object name,exited,exitCode,unchanged | Format-Table
