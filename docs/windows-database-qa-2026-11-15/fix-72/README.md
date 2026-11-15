# 0.4.0-72 外键连接修复与回归

用户确认范围：修复目录迁移后外键检查关闭的问题；不修改日志，不处理外部 WAL 连接场景，不改变 Linux 操作链路。手动备份不属于产品需求。

## 修改

唯一业务代码修改位于 `desktop/data_directory_legacy.go`，构建约束是 `!linux || android`。迁移成功、复制失败回退、新库打开失败回退，三个连接打开位置均改用 `persistence.OpenExisting`，统一设置外键检查、单连接池并完成现有数据库校验。回退失败时保留原始错误和回退错误，不再交回已关闭的旧句柄。

未修改 `desktop/data_directory_linux.go`、Linux 的 `SwitchDirectory` 或公共 SQLite 打开实现；日志代码未改。通过 `go list -json ./desktop` 确认 Linux 将旧迁移文件列在 `IgnoredGoFiles` 中，而不是 `GoFiles`。

在 WSL 执行既有 `wails3 task package GOOS=windows ARCH=amd64` 流程，构建号从 71 递增到 72。共同的版本标签、前端包版本和 Windows 构建元数据同步更新；Linux nfpm 配置恢复为本次修改前内容，并按字节比对一致。没有构建或替换 Linux 安装包。

## 验证结果

- WSL：`go test ./desktop ./internal/infrastructure/persistence ./internal/infrastructure/storage ./internal/app` 全部通过。
- Windows：修复前后对照从 `foreign_keys before=1 after=0` 变为 `before=1 after=1`，无效关联记录仍被数据库拒绝。
- Windows：分别通过临时目标目录 ACL 制造“复制失败”和“复制成功但新库打开失败”。两种情况都成功回退，旧库可查询，位置记录还原，外键仍为 1，错误关联记录不能插入；ACL 已恢复。
- Windows：原目标冲突、旧迁移行为、备份与切换并发、备份 WAL 和清理、启动和历史升级相关回归通过。
- 新打包便携版 GUI：界面显示 `v0.4.0-72`；禁止写入的目标迁移失败后，原项目 `qa-72-project` 仍可读取；正常迁移到包含中文、空格、`#`、`%` 的目录后，界面提示成功，位置文件正确，新库 `integrity_check=ok` 且测试项目保留；迁移后重启成功。

完整 Windows 桌面测试有一个既有失败：`TestAPILogsKeepBindingContract`。用修改前保存的测试程序单独执行也失败，表现为日志列表为空，和用户决定暂不修复的日志问题有关。未将完整 Windows 测试描述为全部通过；本次目录迁移相关测试通过。

上轮两项不在本次范围内的审计用例（旧资源保留、外部 WAL 连接）保留在 `desktop/database_windows_audit_test.go`，需显式启用 `-tags database_audit`。它们的历史失败证据仍在上级报告，不代表本次已修复。

## 产物和测试数据

桌面 `studyflow` 下已生成：

- `StudyFlow-0.4.0-72-portable/StudyFlow.exe`
- `StudyFlow-0.4.0-72-windows-amd64-portable.zip`
- `StudyFlow-0.4.0-72-amd64-installer.exe`（仅生成，未安装）

便携版 exe SHA256：`1e68d6872e7828e6d2c181e00b4aa432f5ff54161ef505864b9f84701a828b90`。

开发、编译和打包均在 WSL；Windows 仅运行应用和已交叉编译的测试程序、准备临时目录权限故障，未安装开发工具。

实际 GUI 测试使用 `D:\CLink\Desktop\studyflow\db-fix-qa-72\package` 隔离副本，新发布便携版目录未写入测试数据。原 71 便携版目录与数据保留；原库恢复关闭设置后，与本次测试前安全快照的 SQL 内容逐项一致。旧 71 安装器和旧便携压缩包按仓库打包规则清理。

相邻的文本及 JSON 文件保存 Windows、Linux 和 GUI 校验结果。完整构建日志和隔离数据保留在桌面 `db-fix-qa-72` 目录。
