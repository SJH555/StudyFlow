# Windows 数据库测试报告

后续状态：用户已明确手动备份不属于需求，外部 WAL 连接场景暂不处理，日志保持不变。本报告保留首次审计记录；目录迁移外键问题已在 0.4.0-72 修复，验证结果见 [修复与回归报告](fix-72/README.md)。

测试时间：2026-11-15，Asia/Shanghai。

结论：数据库启动和备份的主要路径通过，Windows 目录切换尚不满足验收要求。确认 3 项数据安全/连接问题、1 项启动诊断问题；手动备份缺少产品入口。

## 环境和方法

- Windows 11 专业版，10.0.26300，amd64。
- 实测便携版：`D:\CLink\Desktop\studyflow\StudyFlow-0.4.0-71-portable\StudyFlow.exe`，界面版本 `0.4.0-71`。
- 可执行文件 SHA256：`D101EF83183B02002192D5C834FB7330E139E8FA8337FF58F4DC94E6C4909D46`。
- 源码基准提交：`62e1349335f30757421643652e82176d875af181`。开始时已有版本/构建配置和前端 package 文件改动，均未修改。
- 在 WSL 使用已有 Go 1.27.3 交叉编译 Windows 测试程序；在 Windows 原生执行。没有安装工具或使用安装版。
- GUI 测试使用原便携版 exe 的隔离副本。测试路径：`D:\CLink\Desktop\studyflow\db-qa-20261115`。
- 区分两种证据：**打包程序实测**验证桌面产品行为；**Windows 源码测试**直接调用当前源码中的启动、绑定、调度器和持久化逻辑。后者不代表已对打包 exe 内部执行相同调用。
- 历史数据使用现有测试中三个已知历史 DDL 生成，加入合成记录；不是覆盖所有可能的历史用户文件。

## 验收矩阵

| 验收项 | 结果 | 证据和边界 |
|---|---|---|
| 首次创建数据库 | 通过 | 打包程序干净启动，9 张表，`integrity_check=ok`；Windows 启动测试通过 |
| 已有数据库打开 | 通过 | 打包程序运行超过观察窗口；测试记录保留，库文件哈希不变 |
| 历史数据库升级 | 通过已知三种结构 | 打包程序分别升级为 9 张表，保留 `legacy subject`，完整性检查通过；源码迁移测试也通过 |
| 异常数据库拒绝打开 | 通过 | 损坏、0 字节、陌生结构、正式位置缺库、缺少必要列：打包程序退出码均为 1，原文件/缺失状态不变 |
| Windows 旧切换链路成功 | 基本路径通过 | GUI 目录选择、确认迁移、路径更新，以及切换后重新启动成功；测试了中文、空格、`#`、`%` 路径 |
| 切换失败后旧库可用 | 部分通过 | 目标冲突：旧 API 和位置记录正常；关闭旧库后目标禁止创建文件：能回退并查询原数据，但外键配置丢失 |
| 旧数据库、备份、用户文件保留 | **失败** | 打包程序和源码测试均发现旧主库、旧备份目录删除；备份目录里的非 `.db` 用户文件没有迁移且被删除。旧数据目录根部的用户文件仍在 |
| 并发 API 及切换后连接正确 | **部分失败** | 20 个创建 API、一次备份和一次切换并发，连续 30 轮通过；切换后外键约束关闭；外部连接保留 WAL 时丢失最新提交记录 |
| 自动备份 | 通过所测路径 | GUI 开启后生成完整备份；Windows 测试覆盖启动策略、48 小时间隔边界、轮换、关闭开关时清理和启用时无死锁 |
| 手动备份 | **缺少入口** | 设置页和桌面绑定未找到手动备份操作。底层 `CreateBackup` 能生成快照，不等于产品手动备份功能已实现 |
| 备份完整性和 WAL | 通过 | `VACUUM INTO` 生成可打开的完整库；未 checkpoint 的已提交 `wal-marker` 存在于备份。目录切换的 WAL 问题单列为失败 |
| 过期备份清理 | 通过所测路径 | 早于和恰好到达 48 小时边界的命名备份被清理；`user.db`、`notes.txt` 保留；快照失败时不清理旧备份 |
| 备份与切换并发 | 通过所测进程内场景 | 30 轮并发后新目录有一个可打开、完整性为 `ok` 的备份，API 创建的 20 条记录都在活动库中 |

## 缺陷与代码分析

### P1：成功切换删除旧资源，且会丢失备份目录中的用户文件

位置：`desktop/data_directory_legacy.go:75`、`:95`、`:96`，`internal/infrastructure/persistence/backup_legacy.go:29`。

步骤：在旧库的 `backups` 中放一个合法命名 `.db` 备份和 `user-notes.txt`，根部放 `user.txt`，切换到空目录。

实际结果：新库和 `.db` 备份迁移成功；旧 `study.db` 和整个 `backups` 被删除；`user-notes.txt` 未迁移且丢失；根部 `user.txt` 保留。打包程序 GUI 和新增 Windows 测试均复现。

原因：备份复制只处理 `.db`，随后无条件 `os.RemoveAll(oldBackups)`。现有 `TestLegacyBindingKeepsDataDirectoryBehavior` 和 `TestChangingDataDirectoryMovesBackupFiles` 反而断言旧资源消失，因此原测试通过不代表符合本次要求。

建议：切换应保留旧数据库及旧备份目录；不要递归删除混有用户文件的目录。新版本应调整现有测试的验收预期。

### P1：切换及复制失败回退后外键约束关闭

位置：`desktop/data_directory_legacy.go:82`、`:88`、`:91`；对照 `internal/infrastructure/persistence/sqlite.go:39`。

Windows 源码测试：切换前 `PRAGMA foreign_keys=1`，切换后为 `0`，插入不存在的 `subject_id=99999` 的计划成功。

进一步只对临时目标目录设置拒绝创建文件的 ACL，复制报 `Access is denied`，位置文件回滚且旧记录可查询，但恢复连接的 `foreign_keys` 同样为 `0`。临时目录 ACL 在测试结束前已恢复。

原因：正常启动经过带 `_pragma=foreign_keys(ON)` 的统一打开函数；切换及回退使用裸 `sql.Open("sqlite", path)`，只在成功路径限制连接数。`sql.Open` 还是惰性打开，未确认新连接可用就交接、清理源文件。

建议：成功和回退都使用统一的、已验证的连接打开流程，验证连接配置和库结构后再交接。

### P1：外部 SQLite 连接保持打开时，切换丢失已提交 WAL 记录

位置：`desktop/data_directory_legacy.go:78` 至 `:94`。

Windows 源码复现：启用 WAL 并关闭自动 checkpoint；建立第二个连接并保持打开；通过应用连接提交 `committed-in-wal`；执行目录切换。切换返回成功，但新库中该记录数量为 `0`。

原因：关闭应用自己的连接不能保证其他连接仍存在时完成 WAL checkpoint。旧链路只复制主 `.db`，没有生成包含 WAL 的 SQLite 一致性快照。

建议：使用 SQLite 在线快照（现有备份采用的 `VACUUM INTO` 或专用快照函数），校验快照后打开新连接；保留源文件。此处不能靠进程内读写锁处理其他进程/连接。

### P2：打包程序拒绝异常库时没有落盘错误日志

位置：`desktop/main.go:114`。

打包程序默认启动测试中，五种异常都退出码 1，`logs/error-2026-11-15.log` 均为 0 字节。对同一损坏库改用有效的 stderr 重定向后，stderr 捕获到 `file is not a database (26)`，应用日志也写入 149 字节。

代码使用 `io.MultiWriter(os.Stderr, errorLog)`。它在前一个 writer 失败时停止写入；Windows GUI 程序没有有效 stderr 时会阻断文件日志。证据支持这一原因，未对打包 exe 的 stderr 句柄做内部探测。

建议：让文件日志不依赖 stderr 的写入成功。异常库仍应拒绝打开，同时提供可定位的日志或错误界面。

## 原始结果

- `windows-qa.txt`：7 项新增验收测试，4 失败、3 通过。失败分别是资源保留、切换后外键、复制失败回退外键、外部 WAL 记录。当前故意保留失败断言，作为待修复问题的回归用例。
- `concurrency-30.txt`：20 个写 API + 备份 + 切换，30 轮通过（非 race detector 运行）。
- `baseline-desktop.txt`、`desktop-backup.txt`：已有启动、旧切换行为、备份和开关测试。Linux 专属入口测试在 Windows 跳过。
- `persistence.txt`：Windows 上可编译的 31 项顶层持久化测试通过。
- `app-backup.txt`：备份策略通过；Unix chmod 删除权限用例在 Windows 跳过，不能据此声明 Windows 删除权限故障已覆盖。
- `storage.txt`：3 项顶层测试通过，`TestStorageBaseDir` 失败。失败来自 Windows 的 `filepath.Join` 产生反斜杠，而测试对模拟 Linux 参数硬编码正斜杠预期；属于测试跨平台适配问题，不是本次 Windows 存储位置故障。
- `package-results.json`：9 个真实 exe 启动场景，每个成功场景观察 4 秒后停止隔离测试进程。`log:null` 表示当时正常启动方式下日志为空；后续 stderr 对照实验单独记录在上文。
- `package-database-checks.json`：迁移后表数、完整性和测试记录。
- `package-restart.json`：GUI 切换后的真实 exe 再次启动和位置文件。

新增回归用例：`desktop/database_windows_qa_test.go`。复现编译方式（WSL）：

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /mnt/d/CLink/Desktop/studyflow/db-qa-20261115/desktop.test.exe ./desktop
```

随后在 Windows 执行：

```powershell
& 'D:\CLink\Desktop\studyflow\db-qa-20261115\desktop.test.exe' '-test.run=TestWindowsQA' '-test.v' '-test.timeout=60s'
```

`scripts/qa/prepare_windows_database_fixtures.py` 和 `run-package-startup.ps1` 保存了本次真实 exe 启动测试的准备/执行方法，含本机测试路径；准备脚本拒绝覆盖已有 fixture 目录，避免误用现有数据。

## 未覆盖及收尾

未使用安装版，未覆盖 Program Files 权限、安装/升级/卸载；未模拟断电、磁盘满、物理损坏或切换过程中的进程崩溃；未完整穷举位置文件写入/恢复失败。权限故障仅对合成目标目录拒绝创建文件。历史升级仅覆盖三个已知结构。正常并发通过不代表外部连接、并发两次目录切换或崩溃恢复已通过。

原便携版在测试前创建安全快照，临时通过界面将关闭行为设为退出以正常关闭；随后还原了原先不存在的 `closeBehavior` 键。重新启动前，原数据库完整 SQL 导出与测试前快照逐项相同；最后已重新打开原便携版并确认正常主界面。没有修改业务实现或重新打包，没有使用安装器；保留了隔离数据和测试程序供复查。
