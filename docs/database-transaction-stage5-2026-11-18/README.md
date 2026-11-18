# 数据库事务重构：第五阶段与综合验收

日期：2026-11-18。状态：第五阶段数据库故障验收完成。五个阶段的数据库事务改造已完成；Windows 全仓测试仍有两项与数据库无关的既有失败，详见下文，不将整仓结果表述为全绿。

本阶段新增故障测试和报告，没有新增生产代码、错误分类器、自动重试、自动重连或恢复状态机。现有统一事务封装在本次测试中按设计工作，无需为了应付不同错误码再增加一套回滚逻辑。主数据库仍保持一个连接池，最大打开/空闲连接均为 1。

## 验证方式与实际结果

| 场景 | 方法与环境 | 验证结果与证据边界 |
| --- | --- | --- |
| SQLite 空间不足 | Linux、Windows；在临时主数据库设置 max_page_count，再写入超出页数上限的大字段，SQLite 实际返回 SQLITE_FULL=13；覆盖 DELETE、WAL | 本次事务中前面的更新撤销，已提交基线保留，重开 integrity_check / foreign_key_check 通过。这是真实 SQLite FULL 路径，但不是把主机物理磁盘填满 |
| OS 文件写入失败 | Linux；只在独立子进程内设置 RLIMIT_FSIZE 并忽略 SIGXFSZ，使真实文件写入超过上限 | SQLite 实际返回扩展错误 SQLITE_IOERR_WRITE=778；此前成功的 SQL 回滚，回调不重跑，重开数据和完整性检查通过。只限制测试子进程，不改变主机磁盘配额/挂载/全局限制 |
| 备份文件写入失败 | Linux；同样用子进程文件大小限制阻止 VACUUM INTO 输出完整副本 | 实际返回 IOERR_WRITE=778；原数据库大字段仍完整，旧备份保留，无正式半成品备份或临时文件残留，不进入过期备份清理 |
| SQLite 可控 NOMEM | Linux、Windows；使用驱动公开的 sqlite.Limit，将临时连接的 SQLITE_LIMIT_VDBE_OP 设为 100，生成超过指令上限的 SQL | SQLite 自身在准备语句时返回 SQLITE_NOMEM=7；前面的 UPDATE 整体回滚，重开基线和完整性正常。这是安全的真实错误码路径，不是整机 RAM 耗尽 |
| 语句/回读 I/O、内存错误 | Linux、Windows；测试连接器保留真实 SQLite，仅在指定 SQL 返回受控 IOERR/NOMEM；各覆盖写入失败、回读失败、Rollback 成功/失败，共 8 个子用例 | 停止并保留原始错误/错误码；整体回滚；清理失败同时保留两种错误，隔离连接并暂停后续访问；不重跑业务。明确属于驱动边界故障注入，不代表真实硬件损坏 |
| Commit 返回错误但实际已提交 | Linux、Windows；包装真实 SQLite，在物理提交成功后注入 IOERR/NOMEM | 向上返回失败、暂停访问、不重跑回调，不声称数据必定回滚；另开检查连接确认数据确实已提交，证明没有重复执行和错误推断 |
| 强杀普通事务 | Linux、Windows；真实测试子进程使用 DELETE / WAL，缓存设小并确认存在非空 journal/WAL 后由父进程强杀 | 提交前强杀：原始值保留、整批插入不存在；确认提交后强杀：新值和全部 17 行保留；重启校验通过 |
| 强杀 DDL + 数据事务 | Linux、Windows；在事务内新增字段/表并写数据，确认 dirty sidecar 后强杀 | 重新走 OpenForStartup 后，未提交字段、表和数据均不残留，已提交数据保留 |
| 强杀目录位置协议 | Linux、Windows；子进程执行真实 Stage/Publish/CleanupRetired，在暂存后、发布后、旧库清理后三个节点强杀，再走真实 openStartupDatabase 恢复 | 暂存后恢复旧库，发布后/清理后恢复目标库；校验选中数据、清理 pending、保留用户 notes 文件，再次启动仍选中同一库 |

### 磁盘满和 I/O 错误时为何暂停

实际 FULL 和 IOERR_WRITE 测试中，SQLite 已自行回滚整个事务。封装随后仍执行显式 Rollback，驱动返回“cannot rollback - no transaction is active”。现有策略将真实 Rollback 失败视为无法可靠确认连接状态，隔离该连接并暂停数据库访问。

这属于保守拒绝继续写入，不是测试失败。原始 FULL/IOERR 和清理错误都保留；关闭后重开确认此前已提交的数据完整、本次修改不存在。这里没有通过匹配错误文本或假定 SQL 幂等来放行连接；需要处理环境故障并重新启动应用。

NOMEM 指令限制测试中的 Rollback 成功，不需要暂停。是否隔离取决于事务/清理结果，而不是另写一套逐错误码回滚规则。

## 测试文件

- internal/infrastructure/persistence/stage5_fault_test.go：真实 FULL、真实受限 NOMEM、受控驱动错误、提交不确定性、DELETE/WAL 事务和 DDL 强杀恢复。
- internal/infrastructure/persistence/stage5_linux_test.go：隔离子进程中的真实 OS 写入失败。
- desktop/stage5_recovery_test.go：目录恢复协议强杀后通过桌面启动入口验收。
- desktop/stage5_linux_test.go：实际 VACUUM INTO 写入失败时保留旧备份。

所有强杀只针对本次测试启动并持有 PID 的子进程；未结束日常 StudyFlow 进程。Linux 使用 Process.Kill 对应的 SIGKILL，Windows 使用 TerminateProcess；不执行 defer。SQLite 的启动恢复负责处理遗留日志。

目录协议强杀测试使用预先验证的源/候选副本，验证位置发布和启动恢复；业务事务与目录快照交错、真实在线快照/WAL 的正确性由第四阶段和既有整包用例覆盖，不把两种测试混称为同一个场景。

## 综合测试结果

| 环境 | 范围 | 结果 / 日志 |
| --- | --- | --- |
| Linux / WSL | go test ./... -count=1 -timeout=180s | 全仓通过，linux-regression.txt |
| Linux / WSL | go test -race ./... -count=1 -timeout=240s | 全仓通过，linux-race.txt |
| Linux / WSL | persistence + desktop 的 TestStage5，每组连续 10 轮 | 8 组顶层验收共 80 次通过；40 次无环境参数的子进程辅助入口按预期跳过，linux-repeat.txt |
| Windows amd64 | 全仓 15 个有测试的包，分别编译为原生 test.exe 运行 | 13 个包通过；desktop、reminder 各 1 项既有失败，windows-suite-exit-codes.txt / windows-*-native.txt |
| Windows amd64 | persistence 整包 | 77 个顶层用例通过，3 个平台限制/辅助入口跳过，windows-persistence-native.txt |
| Windows amd64 | desktop 排除既有日志用例 | 82 个通过，3 个平台限制/辅助入口跳过，windows-desktop-except-known-log.txt |
| Windows amd64 | reminder 排除原有不正确的音频查询用例 | 3 个通过，windows-reminder-except-known-audio.txt |
| Windows amd64 | 6 组 TestStage5 验收连续 10 轮 | persistence 50 次、desktop 10 次顶层验收全部通过；20 次辅助入口按预期跳过，windows-*-repeat.txt |

每一轮包含 6 个事务/DDL 强杀状态（2 种日志模式 × 提交前/提交后/DDL 未提交）和 3 个目录协议强杀状态，连续 10 轮在 Linux、Windows 各执行了 90 个实际强杀恢复场景。辅助入口在正常整包运行时跳过，在父测试设置环境变量后实际执行，没有把它们当作未覆盖的业务用例。

此前四阶段的事务、Context、单连接并发、panic、约束错误、回读错误、备份、目录身份保护、启动迁移及退出排空等测试均随全仓回归再次运行。

## Windows 两项既有失败

1. **TestAPILogsKeepBindingContract**：日志列表为空。第二至第四阶段及 docs/windows-database-qa-2026-11-15/fix-72/README.md:21 已记录；本阶段结果相同。
2. **TestWindowsAudioWorkerStopsAndCanBeRecreated**：本阶段扩展到全部测试包后新发现的原有测试问题。它调用 `mciCommand("sysinfo all quantity")`，但 ringtone_windows.go 的 mciSendStringW 调用传入 0 长度返回缓冲区，查询需要返回文本，Windows 返回“输出的字符串太大，超出返回的缓冲区的长度”。测试和音频实现与 HEAD 一致，reminder 包也不访问 persistence/database/sql；本阶段没有修改音频功能或将该用例改为跳过。另行运行时通过命令行明确排除，完整失败日志仍保留。

排除运行用于确认其他用例的结果，不能覆盖或替代整仓失败事实。这两项不属于本次数据库事务故障，也没有宣称它们已修复。

Windows 测试目录：

`C:\Users\jhsong\AppData\Local\Temp\StudyFlow-tx-stage5-dsFW9IDH`

Windows test.exe 在 WSL 交叉编译，再通过真实 Windows PowerShell Start-Process 执行并核对退出码。windows-run-all.ps1、windows-run-tests.ps1 为执行脚本，stdout 与非空 stderr 一并保存。仅修改测试 PowerShell 进程内的 System32 路径及 PATHEXT，未改机器持久配置。

## 可以确认与不能夸大的边界

可以确认：在已验证的真实 SQLite 错误、受控驱动错误和进程强杀场景中，业务不会把失败当作成功；能回滚时撤销整笔事务；清理不确定时隔离连接并暂停访问；不自动重复业务；重开后检查已提交数据和数据库完整性。

不能由这些测试推导出任何程序都能 100% 恢复的承诺：

- 没有将用户磁盘实际填满、故意损坏存储硬件或切断整机电源。max_page_count 验证 FULL；Linux 文件大小上限验证真实 OS 写入错误，Windows 该类低层失败额外通过驱动注入覆盖，不能称为两系统都实测了物理介质故障。
- NOMEM 验证的是 SQLite 的可控分配/编译资源拒绝与驱动错误路径，没有耗尽操作系统 RAM。Go 运行时致命 OOM、OS 强制终止不会执行 defer；进程强杀测试验证相应的 SQLite 日志恢复路径，但不等于物理内存耗尽全场景验收。
- 不模拟坏磁盘谎报同步成功、文件系统破坏原有数据、控制器缓存失电或备份与主库同时丢失。断电持久性仍取决于 SQLite 日志/同步配置及存储设备正确履约。
- Commit 返回错误时，数据可能已经提交；系统会停止继续写，不自动重放，也不会错误声称已回滚。副作用必须继续遵守提交后触发的约束。

未递增版本、未引入 Goose/user_version 或降级、未替换固定目录程序、未读写日常用户数据库。故障只施加在临时文件、临时数据库连接和本次子进程中。Linux 无需 GUI，Windows 本次为原生后端/文件协议验收，不声称进行过产品界面验收。
