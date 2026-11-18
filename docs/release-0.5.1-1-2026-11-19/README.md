# 0.5.1 打包记录

2026-11-19，按用户要求生成 Linux AppImage、deb、rpm、Windows exe 安装包及绿色版。遵循项目 A.B.C-D 规则，应用版本为 0.5.1-1，架构为 x64。数据库目标仍为 1，本次没有新增数据库升级 SQL 或改变业务数据结构。

## 产物

位于仓库 bin/：

- StudyFlow-0.5.1-1-linux-amd64.AppImage
- StudyFlow-0.5.1-1-linux-amd64.deb
- StudyFlow-0.5.1-1-linux-amd64.rpm
- StudyFlow-0.5.1-1-amd64-installer.exe
- StudyFlow-0.5.1-1-windows-amd64-portable.zip
- SHA256SUMS-0.5.1-1.txt

标准 Linux 打包任务还生成 Arch 包和 Linux 裸程序；Windows 裸程序亦保留。绿色 ZIP 仅含 StudyFlow.exe 和使用说明，不带日常库、设置、日志或测试数据。Windows 运行需要 WebView2 Runtime，安装包带有运行环境引导程序。

## 验证

- 前端类型检查、生产构建及 Linux/Windows amd64 构建、打包完成。
- Linux 全仓 Go 测试通过，见 linux-regression.txt；persistence/upgrade 定向测试、Goose fork 摘要检查及 release 工具测试通过。
- AppImage、rpm、NSIS 归档检查通过，绿色 ZIP 完整性通过；deb 与 rpm 版本为 0.5.1-1，架构分别为 amd64 / x86_64。
- deb/rpm 中的 Linux 程序与构建产物逐字节相同。AppImage 的 linuxdeploy 会调整运行库搜索路径，代码和 Go 数据段与构建程序一致，WebKit 辅助进程已打包。
- NSIS 和绿色 ZIP 中的 Windows 程序与构建产物逐字节相同；原生 Windows 检查固定文件版本为 0.5.1.1。VersionInfo 的字符串字段为空，因此另核对嵌入的 ProductVersion 为 0.5.1-1，不将空字符串查询宣称为通过。
- 完整产物大小与 SHA-256 见 package-verification.json 及 bin/ 校验文件。

此次未安装软件包或重复进行桌面 UI 验收，也未操作日常数据库。此前 Windows 日志列表、音频查询两项既有测试失败不因本次打包视为修复。

## 打包中处理的问题

Windows 首次打包缺少被忽略的生成文件 wails_tools.nsh。使用当前 Wails 官方生成器在临时目录生成后复制该辅助文件，保留项目自定义安装脚本，再次打包成功。没有修改应用或卸载时保留用户数据的行为。

Linux 回归暴露 TestBusinessTransactionSnapshotConstraintIsNotIgnored 的时间依赖：夹具用 UPDATE 设置检查时间，但键不存在时未实际写入，凌晨会按默认检查时间跳过。改为在临时测试库中插入/更新这两个设置，确保故障场景实际执行；相关用例与全仓重新通过。修改仅涉及测试数据准备，不改变应用设置规则或数据库格式。

详细构建与诊断日志保存在本机 /tmp/studyflow-release-0.5.1-1/。
