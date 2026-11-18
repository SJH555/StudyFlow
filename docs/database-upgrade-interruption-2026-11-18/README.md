# 启动界面简化与升级中断恢复补测

2026-11-18。第二阶段补充工作完成。没有递增应用版本，没有替换固定日常程序，没有更改日常数据库。

## 界面

采用用户确认的文案：

> 检测到版本更新
>
> 需要调整本地数据格式，以适配新版本。学习记录和设置会保留，开始前将自动备份。
>
> 暂时退出 / 确认继续

确认后：

> 正在适配新版本，请稍候
>
> 完成后将自动进入应用，请勿关闭程序。

启动页和标题栏背景、卡片为纯白色。常规确认/执行页面不再展示版本登记、目标版本、文件路径和重复的备份说明；失败/数据库过新时仍提供排查信息。成功后自动进入，无须手动重启。截图：ui-baseline.png、in-progress-close-delete.png。

## 新增测试，不增加生产升级钩子

- `internal/infrastructure/upgrade/interruption_test.go`：真实 Manager → Goose → persistence 共用事务链的取消与子进程强杀；涵盖 DELETE/WAL 两种日志、事务未提交/已提交，并重新打开核对和续跑。
- `desktop/startup_upgrade_gui_test.go`：仅 windows && upgradegui 测试构建启用的真实 Wails 窗口；增加 SQL 内检查点、受限 max_page_count、提交完成后暂停检查点。
- `desktop/startup_upgrade_gui_verify_test.go`：独立 Windows 原生进程核对数据库和所有保留备份。避免用 WSL 的 SQLite 并发读取 Windows WAL。
- `scripts/qa/prepare-upgrade-interruption-windows.py`：新建有专属标记的临时 fixture，填充九张业务表，生成预期数据和测试编译 overlay。
- `scripts/qa/test-upgrade-interruption-windows.mjs`：真实 WebView 中点击确认、关闭/退出，或按已验证路径和 PID 终止自己的 QA 进程；重新运行真实启动流程并调用独立原生校验器。
- 普通七场景启动 QA 同步更新文案、白色背景和确认页信息断言，仍验证取消/备份失败/新版拒绝不写原文件。

正式版本的 TargetVersion 仍为 1。测试通过编译 overlay 在临时二进制内使用目标 3 和 SQL 2、3，没有把合成版本/故障注入/调试端口写入生产路径，也没有新增数据库连接池。日常库仍为单连接配置。

## Windows 界面实际测试：14 条全部通过

以下每条分别在 DELETE、WAL 模式执行，共 14 条。完整结果见 windows-interruption-results.json。

| 场景 | 中断位置/操作 | 验证结果 |
| --- | --- | --- |
| 点 × | 文件 2 已提交，文件 3 已执行 UPDATE、ALTER TABLE、CREATE TABLE 及 8 MiB BLOB 写入，尚未提交；点击真实标题栏关闭按钮 | 取消并等待清理后退出；版本保持 2；文件 3 的数据和 DDL 一并撤销；重开确认后升至 3 |
| 点退出 | 同一真实 SQL 内检查点，点击界面退出按钮 | 同上；未先退出进程再寄希望于恢复 |
| 强杀进程 | 同一检查点，验证脏 journal/WAL 非空后，对专属 QA 进程执行 Windows Stop-Process -Force | 没有正常退出报告，确认未走清理；下次由实际应用打开并恢复后保持 2；确认后升至 3 |
| 连续两次强杀 | 首次在文件 3 中被杀，恢复后再次执行文件 3，再次强杀 | 两次恢复均保持 2，无部分结构/数据残留；第三次成功，版本记录无重复 |
| 提交成功后强杀 | SQL 和版本 3 已提交，业务激活前的测试暂停点强杀 | 重开直接进入工作区，版本 3 保留，不重复执行升级 |
| SQL 执行出错 | 文件 3 的 DML/DDL/大数据写入之后，SQL 调用的测试函数返回错误 | 错误页阻止进入业务；完整回滚文件 3，保持 2；解除测试故障后原 SQL 重试成功 |
| 实际 SQLITE_FULL | 仅在测试连接限制 SQLite max_page_count，文件 3 写入 BLOB 时真正触发磁盘满错误，不填满系统磁盘 | 错误页阻止进入业务；保持 2，文件 3 全回滚；新进程取消测试页数限制后成功升级 |

每条成功后还再次重启，验证同版本直接进入、不重跑升级、不重复登记。

每个检查点由独立 Windows 原生 SQLite 校验：

- subjects、plans、sessions、activity_logs、settings、daily_reviews、daily_review_items、daily_plan_targets、stage_goals 原有所有字段值及记录逐行与预存快照一致。
- `PRAGMA integrity_check` 为 ok，`foreign_key_check` 无结果。
- 版本记录必须严格等于 0…实际已提交版本，全部已应用；拒绝跳号或重复。
- 文件 3 未提交时新增列/表不存在；提交成功时同时存在，验证 DDL 与 DML 原子性。
- 每一份升级前备份可打开、完整、外键正确，九张业务表数据与原始快照一致，未包含文件 3 的部分修改；恢复后的新备份位于已提交版本 2。

强杀后先重新启动实际应用，让它打开/恢复/检查数据库，再运行独立校验器；没有先用测试工具修复数据库再声称应用恢复成功。

## 其他回归

| 检查 | 结果/记录 |
| --- | --- |
| Linux 全仓 `go test ./...` | 通过，linux-regression.txt |
| Linux desktop/persistence/upgrade race | 通过，linux-race.txt |
| 三包 go vet | 无诊断，linux-vet.txt |
| Linux 中断恢复 6 个子场景重复 10 轮 | 全部通过，共 60 次，linux-interruption-repeat.txt |
| Windows 原生 upgrade | 16 个顶层测试通过，1 个子进程辅助入口跳过，upgrade-native-stdout.txt |
| Windows 中断恢复 6 个子场景重复 10 轮 | 全部通过，共 60 次，upgrade-repeat-stdout.txt |
| Windows 原生 persistence | 83 个顶层测试通过、3 个平台/辅助入口跳过，persistence-native-stdout.txt |
| Windows 原生 desktop 全包 | 94 个顶层通过、3 跳过；仅既有 TestAPILogsKeepBindingContract 失败（日志列表为空），desktop-native-stdout.txt |
| Windows desktop 排除已知日志失败 | 通过，desktop-except-known-log-stdout.txt |
| Windows 真实 WebView 普通启动七场景及原文件核对 | 通过，windows-ui-results.json、windows-ui-database-checks.json |
| Vue 类型检查/Vite、Windows 产品构建 | 通过，windows-build.txt；产物 bin/StudyFlow.exe，未增版 |

现有 SQL/语义错误、panic、Goose 版本写入失败、Commit 失败隔离、受控 IOERR/NOMEM/回滚失败及实际 SQLite FULL/NOMEM 测试随相关整包回归运行。原有 reminder 音频查询失败属于历史已知问题，本次未重跑未改动的该包 Windows 测试，不将其宣称已修复。

## 测试工具及复现

UI 测试前已通知用户。computer-use 工具仍因 WSL sandboxCwd URI 无法初始化，采用真实 Windows WebView 的 CDP 操作和截图。测试进程均由独立测试实例守卫启动，不干扰日常 StudyFlow 实例。临时目录清单见 qa-directories.txt。

首次 WAL 校验使用 WSL Python 读取 Windows 正在使用的 WAL，遇到跨系统 I/O 错误；该轮未计作通过。已改成独立 Windows 原生校验，并重新运行全部 14 条通过。另将原生校验通过 PowerShell Start-Process 启动，避开 WSL 直接反复启动 exe 的一次 vsock 超时。最终通过的记录均来自改正后的完整测试。

构建/执行恢复测试（仓库根目录，Go/Node 在 PATH）：

```bash
qa_dir=$(python3 scripts/qa/prepare-upgrade-interruption-windows.py)
GOOS=windows GOARCH=amd64 go test -overlay "$qa_dir/overlay-extreme.json" -tags upgradegui -c -o "$qa_dir/upgrade-ui.exe" ./desktop
node scripts/qa/test-upgrade-interruption-windows.mjs "$qa_dir"
```

普通界面测试：

```bash
qa_dir=$(python3 scripts/qa/prepare-upgrade-startup-windows.py)
GOOS=windows GOARCH=amd64 go test -overlay "$qa_dir/overlay.json" -tags upgradegui -c -o "$qa_dir/upgrade-ui.exe" ./desktop
node scripts/qa/test-upgrade-startup-windows.mjs "$qa_dir" docs/database-upgrade-interruption-2026-11-18
python3 scripts/qa/verify-upgrade-startup-windows.py "$qa_dir"
```

不可同时运行两个 GUI QA（共用仅测试用 CDP 端口）。测试使用全新临时 fixture，不重新初始化日常数据。

## 验证边界

本次实测了进程级强杀、取消、升级失败以及提交前/提交后的恢复；未制造物理断电、硬件损坏、文件系统失真或 Go 运行时致命 OOM。页数限制验证的是实际 SQLITE_FULL 错误处理，不是整块磁盘耗尽。不能据此承诺任何系统级事故都 100% 恢复。没有自动将备份覆盖回原库；常规中断通过事务恢复和已提交版本继续，保留备份供必要时恢复。
