# Windows 进程显示名称

- 修改 `build/windows/info.json` 的 `FileDescription`：由“学习时间管理与记录”改为 `StudyFlow`。
- 仅修改 Windows 版本资源，未改 Linux 配置、程序业务逻辑或版本号。
- WSL Debian：`wails3 task windows:build ARCH=amd64` 成功，包含前端类型检查和生产构建；通过 objdump 和 UTF-16 字符串检查，生成的 PE 包含版本资源及 `FileDescription = StudyFlow`。
- Windows：更新桌面 76 便携 exe 和压缩包，正常退出旧进程后启动新进程；恢复原关闭时最小化设置。
- 验证边界：已打开的提升权限任务管理器仍显示旧中文名，尚未验证重新打开任务管理器后的名称。Windows FileVersionInfo 本次返回空字段，不以此作为验证通过依据。
