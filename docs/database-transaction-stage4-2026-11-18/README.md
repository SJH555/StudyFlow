# 数据库事务重构：第四阶段

日期：2026-11-18。状态：第四阶段完成；等待用户同意后进入第五阶段。

后续进展：用户已批准第五阶段，数据库故障验收完成，见 ../database-transaction-stage5-2026-11-18/README.md。以下保留第四阶段结束时的记录。

本阶段覆盖启动初始化、自动备份、数据目录切换和退出与统一事务的配合。主数据库仍只有原来的一个连接池，MaxOpenConns/MaxIdleConns 均为 1；备份校验和目录候选校验会短暂打开副本文件，不增加主数据库池的连接上限。

## 改动与效果

| 范围 | 实际改动 | 效果 |
| --- | --- | --- |
| persistence/sqlite.go | 新建库和启动升级均复用已有统一事务；schema 检查、建表、补字段、默认设置、旧数据转换和升级后检查放在同一事务；补全初始化查询的迭代/关闭错误检查 | 后续 SQL/校验失败时，前面的 DDL 和数据更新一起回滚；不靠重复执行幂等 SQL 补救 |
| persistence/sqlite.go | 未交给应用的连接池由 openSQLite 负责在错误返回和 panic 时关闭 | 事务清理结束后再关闭池；不向桌面或后台任务交付失败的初始化结果 |
| persistence/backup.go | 增加 Context 入口；VACUUM INTO 仍位于普通 SQL 事务之外；预留临时文件、校验副本、关闭校验连接、同步文件，再以不覆盖已有文件的方式发布 | 取消/失败不进入旧备份清理；同名备份不被覆盖；不清理别人的同名临时文件；临时文件清理按身份核验 |
| app/backup.go、app/scheduler.go | 调度 Context 传至设置读取、VACUUM INTO 和逐个旧备份清理 | 退出取消不再被 Context.Background 截断；仍在已有生命周期独占锁下运行整个备份策略 |
| desktop/api_service.go、desktop/data_directory.go | SetDataDirectory 保留已接纳任务的 Context 并传至 SwitchDirectory | 目录切换响应任务取消，退出仍等待它结束 |
| persistence/directory_snapshot.go、directory_prepare.go | 在线快照分批复制，每批检查 Context；备份文件复制的每次读取检查 Context；准备完成和逐文件处理增加取消检查 | 提交位置前可以取消并清理本次准备资源，不改当前数据库和正式位置 |
| persistence/directory_switch_flow.go | 加锁前/后、准备后、位置暂存前/发布前检查取消；已暂存未发布时通过 Recover 确认旧位置后才能清理恢复记录 | 无法确认时保留新旧文件和恢复记录，暂停数据库访问；位置发布确认成功后不因晚到的取消误报失败 |
| 退出协调 | 保留既有 desktopTasks 的停止接纳、取消、排空和最后关闭数据库的顺序；新增真实事务专项测试 | 不另造退出机制，验证关闭确实等待取消事务回滚结束 |

设置默认值使用 `ON CONFLICT(key) DO NOTHING`，只保留已存在设置这一正常业务行为；不再用宽泛的 INSERT OR IGNORE 吞掉其他约束错误。发生实际错误仍立即返回并撤销本次整批初始化。

启动事务使用启动阶段的独占所有权，尚未发布给桌面，也没有运行后台任务。沿用统一封装的处理：业务/SQL 失败尝试回滚；panic 不吞；Begin/Commit/真正的 Rollback 失败隔离连接。启动失败关闭池，不继续运行应用。Commit 结果不确定时不重跑初始化。

## 测试结果

| 环境 | 命令/范围 | 结果 |
| --- | --- | --- |
| Linux / WSL | go test ./internal/... ./desktop -count=1 -timeout=120s | 通过，linux-regression.txt |
| Linux / WSL | 同范围 -race -count=1 -timeout=180s | 通过，linux-race.txt |
| Linux / WSL | persistence、desktop 的 TestStage4 和 TestOpenForStartupRollsBackEntireMigration，每组 10 轮 | 12 组顶层用例全部通过，linux-repeat.txt |
| Windows amd64 | persistence 整包原生运行 | 72 个顶层用例通过、2 个平台限制跳过，windows-persistence-native.txt |
| Windows amd64 | app 整包原生运行 | 15 个通过、1 个平台限制跳过，windows-app-native.txt |
| Windows amd64 | desktop 整包原生运行 | 81 个通过、2 个平台限制跳过、1 个既有失败，windows-desktop-native.txt |
| Windows amd64 | desktop 排除既有日志列表用例 | 81 个通过、2 个平台限制跳过，windows-desktop-except-known-log.txt |
| Windows amd64 | 同一批 12 组专项用例每组 10 轮 | persistence 90 次、desktop 30 次顶层运行全部通过，windows-*-repeat.txt |

12 组专项用例验证：

1. 启动后期颜色转换失败，前面插入默认设置和补 accepted_at 字段均回滚。该用例替换了原先明确接受“部分迁移残留”的测试。
2. 真实外部 SQLite 读锁使启动 Commit BUSY：不交付可用句柄，解除读锁重新打开后无本次迁移残留。
3. 初始化发生 panic 时，建表和数据更新回滚，原 panic 继续传播，连接仍可正常使用。
4. openSQLite 初始化 panic 后关闭未交付的池，文件可以重新打开。
5. 目录切换在取消发生于操作前、准备后、位置暂存后时保留旧位置；位置发布后取消则保持切换成功。
6. 切换排队等待进行中的业务事务；提交、业务回滚、panic 回滚三种结果下，候选数据库只包含最终已提交的数据。
7. 已暂存位置遇到外部修改并取消时，新旧库和恢复记录保留，后续访问暂停。
8. 真实延期外键造成事务 Commit 失败、数据库暂停后，备份和目录切换都不再执行文件操作。
9. 备份排队等待业务事务，提交的数据出现在副本中、回滚的数据不出现，副本 integrity_check 通过。
10. 退出取消正在写入的事务，排空必须等待回滚；关闭后拒绝借用，重开没有未提交数据。
11. 取消备份和旧备份清理，不删除现有过期备份，也不发布新文件。
12. 同时间戳备份冲突不覆盖已有备份；撞到已有临时文件时不删除其内容。

原有备份创建/发布失败保留旧备份、目录切换故障恢复/文件身份保护、WAL 快照、启动历史 schema 识别、启动恢复及退出协调测试也随整包回归执行。这是临时数据库上的后端/文件协议验收，不代表已完成磁盘满、断电、真实硬件损坏等第五阶段验收。

## Windows 环境与已知失败

测试 exe 在 WSL 编译，实际在 Windows PowerShell Start-Process 下运行，并核对退出码与日志。目录：

`C:\Users\jhsong\AppData\Local\Temp\StudyFlow-tx-stage4-aY9yeyyD`

启动脚本为 run-windows-tests.ps1，仅在该 PowerShell 进程中补齐 System32 路径和 PATHEXT；未更改系统持久环境。stdout 和非空 stderr 已保存至本目录。

整包唯一失败仍为 TestAPILogsKeepBindingContract：日志列表为空。第二、三阶段及 docs/windows-database-qa-2026-11-15/fix-72/README.md:21 已记录同一失败，本阶段未改该日志实现；整包结果不能表述为全绿。

## 范围与边界

- 备份、目录切换涉及文件操作，保留独占锁及已有位置恢复协议，不把它们硬塞进普通 SQL 事务。
- Context 是协作式取消。快照每批页面、文件每次读取和发布前可观察取消，不能承诺强行打断已经进入内核的 I/O。退出继续等待已接纳任务完成清理。
- 已发布且确认成功的目录位置不再取消；备份发布后如目录同步报错，报告错误并保留已发布备份，不继续清理旧备份。
- Linux 目录同步和 Windows MOVEFILE_WRITE_THROUGH 使用既有平台实现，不声称两者具有完全相同的断电持久性保证。
- 未递增版本，未引入 Goose/user_version、数据库降级或新业务表字段。保留之前已有的结构兼容逻辑，只改变本次初始化的事务边界。
- 未替换固定目录程序，未访问或修改日常数据库、用户文件、设置。Windows 为原生测试程序验收，未进行产品 GUI 操作；无需 Linux GUI。
- 第五阶段仍待用户批准：磁盘满、进一步 I/O 故障、可控内存错误、强杀恢复和双平台综合验收。
