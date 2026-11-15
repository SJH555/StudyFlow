param(
    [Parameter(Mandatory = $true)][string]$QaRoot,
    [Parameter(Mandatory = $true)][ValidateSet('upgrade', 'uninstall', 'reinstall', 'external-reinstall', 'external-uninstall', 'empty-uninstall')][string]$Stage
)

$ErrorActionPreference = 'Stop'
$qa = (Resolve-Path -LiteralPath $QaRoot).Path
$installName = if ($Stage -eq 'empty-uninstall') { 'empty-installed' } else { 'installed' }
$installDir = Join-Path $qa $installName
$expectedRoot = 'D:\CLink\Desktop\studyflow\uninstall-fix-qa-73'
if ($qa -ne $expectedRoot) { throw '只能在本次隔离测试目录执行安装和卸载。' }
$installer = Join-Path (Split-Path $qa) 'StudyFlow-0.4.0-73-amd64-installer.exe'
$key = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\StudyFlowStudyFlow'
$originalProcess = @(Get-Process StudyFlow -ErrorAction SilentlyContinue | Where-Object { $_.Path -like '*StudyFlow-0.4.0-71-portable*' })

function Assert-Condition([bool]$Value, [string]$Message) {
    if (-not $Value) { throw $Message }
}

function Assert-Preserved([string]$Case) {
    $manifest = Get-Content -LiteralPath (Join-Path $qa "$Case-before.json") -Raw | ConvertFrom-Json -AsHashtable
    foreach ($entry in $manifest.GetEnumerator()) {
        $windowsPath = $entry.Key.Replace('/mnt/d', 'D:').Replace('/', '\')
        Assert-Condition (Test-Path -LiteralPath $windowsPath -PathType Leaf) "应保留文件已丢失：$windowsPath"
        $hash = (Get-FileHash -LiteralPath $windowsPath -Algorithm SHA256).Hash
        Assert-Condition ($hash -eq $entry.Value) "应保留文件内容变化：$windowsPath"
    }
    return $manifest.Count
}

if ($Stage -in @('upgrade', 'reinstall', 'external-reinstall', 'empty-uninstall')) {
    $process = Start-Process -FilePath $installer -ArgumentList "/S /D=$installDir" -WindowStyle Hidden -Wait -PassThru
    Assert-Condition ($process.ExitCode -eq 0) "安装退出码：$($process.ExitCode)"
    Assert-Condition ((Get-ItemProperty -LiteralPath $key).DisplayVersion -eq '0.4.0-73') '安装版本不正确。'
    $expectedExe = Join-Path (Split-Path $qa) 'StudyFlow-0.4.0-73-portable\StudyFlow.exe'
    Assert-Condition ((Get-FileHash -LiteralPath (Join-Path $installDir 'StudyFlow.exe')).Hash -eq (Get-FileHash -LiteralPath $expectedExe).Hash) '安装的 exe 与正式便携版不一致。'
    Assert-Condition (Test-Path -LiteralPath 'D:\CLink\Desktop\StudyFlow.lnk') '桌面快捷方式未创建。'
    Assert-Condition (Test-Path -LiteralPath (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\StudyFlow.lnk')) '开始菜单快捷方式未创建。'
}

if ($Stage -in @('upgrade', 'reinstall', 'external-reinstall')) {
    $case = if ($Stage -eq 'external-reinstall') { 'external' } else { 'default' }
    $preserved = Assert-Preserved $case
    Assert-Condition (Test-Path -LiteralPath (Join-Path $installDir 'uninstall.exe')) '未生成卸载程序。'
} else {
    $runningTarget = @(Get-Process StudyFlow -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq (Join-Path $installDir 'StudyFlow.exe') })
    Assert-Condition ($runningTarget.Count -eq 0) '测试安装版仍在运行，不能执行卸载。'
    $sharedWebview = Join-Path $env:APPDATA 'StudyFlow.exe'
    $sharedExisted = Test-Path -LiteralPath $sharedWebview
    $process = Start-Process -FilePath (Join-Path $installDir 'uninstall.exe') -ArgumentList '/S' -WindowStyle Hidden -Wait -PassThru
    Assert-Condition ($process.ExitCode -eq 0) "卸载退出码：$($process.ExitCode)"
    Assert-Condition (-not (Test-Path -LiteralPath (Join-Path $installDir 'StudyFlow.exe'))) '程序未删除。'
    Assert-Condition (-not (Test-Path -LiteralPath (Join-Path $installDir 'uninstall.exe'))) '卸载程序未删除。'
    Assert-Condition (-not (Test-Path -LiteralPath (Join-Path $installDir 'window-state.json'))) '窗口状态未删除。'
    Assert-Condition (-not (Test-Path -LiteralPath $key)) '卸载注册信息未删除。'
    Assert-Condition (-not (Test-Path -LiteralPath 'D:\CLink\Desktop\StudyFlow.lnk')) '桌面快捷方式未删除。'
    Assert-Condition (-not (Test-Path -LiteralPath (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\StudyFlow.lnk'))) '开始菜单快捷方式未删除。'
    if ($sharedExisted) { Assert-Condition (Test-Path -LiteralPath $sharedWebview) '共用 WebView 目录被误删。' }
    if ($Stage -eq 'empty-uninstall') {
        Assert-Condition (-not (Test-Path -LiteralPath $installDir)) '无用户数据的空安装目录未清理。'
        $preserved = 0
    } else {
        $case = if ($Stage -eq 'external-uninstall') { 'external' } else { 'default' }
        $preserved = Assert-Preserved $case
    }
}

foreach ($process in $originalProcess) {
    Assert-Condition ($null -ne (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)) '原便携版进程受到影响。'
}
$result = [pscustomobject]@{ Stage = $Stage; Passed = $true; PreservedFiles = $preserved; OriginalPids = @($originalProcess | ForEach-Object { $_.Id }); At = (Get-Date -Format o) }
$result | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $qa "$Stage-result.json")
$result | ConvertTo-Json
