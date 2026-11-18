# 数据库事务重构：第二阶段

日期：2026-11-18。用户已明确同意第二阶段在生产代码引入 sql.Conn。
状态：基础封装和访问边界完成，测试结果及限制如下。等待用户确认后才能进入第三阶段。

后续进展：用户已批准第三阶段，业务接入和回归结果见 ../database-transaction-stage3-2026-11-18/README.md。以下内容保留第二阶段结束时的记录，LegacyDB 过渡入口已在第三阶段移除。

## 实际改动

| 文件 | 改动 |
| --- | --- |
| internal/infrastructure/persistence/transaction.go | 新增 Database.WithAccess 和 Access.WithTx。一次独占生命周期借用内执行事务，不重复加锁；固定 sql.Conn 后执行 Begin、SQL、Commit/清理 |
| internal/infrastructure/persistence/executor.go | 新增只暴露 Context SQL 方法的 Executor；业务回调拿不到直接 Commit/Rollback 方法 |
| internal/infrastructure/persistence/sqlite.go | 显式 SetMaxIdleConns(1)，保留 MaxOpenConns(1)、外键 DSN 配置及原有日志/同步/锁等待行为 |
| desktop/api_service.go | 保留任务 Context，构建携带 Context 的请求；普通 API 改为一次 WithAccess 借用。免数据库 API 仍走原快速路径 |
| desktop/api_routes.go | 增加访问范围适配入口，现有模块暂时通过 LegacyDB 过渡；保留现有路由与绑定契约 |
| desktop/api_service_test.go | 更新锁等待断言，增加排队 API 在退出取消后拒绝进入业务的测试 |
| internal/infrastructure/persistence/transaction_test.go | 新增真实 SQLite 测试，以及少量驱动边界故障注入测试 |

未修改前端、数据库结构、应用版本、目录切换协议和日常用户数据。sqlite.go 在开始前已有的其他修改全部保留。

## 事务规则

- 业务错误或 SQL 错误：停止回调，尝试整体回滚，保留原错误；不依赖 SQL 幂等性，也不重跑业务。
- panic/Goexit：defer 执行清理，不吞掉原 panic；清理失败记录日志，同时保存数据库不可用原因。
- Begin/Commit 或真正的 Rollback 失败：移除异常连接，保守地暂停数据库访问，所有 WithRead/WithWrite/WithAccess 都拒绝后续业务。
- 保留业务/Context 错误及可观察到的 Rollback、连接释放和丢弃错误，使用 errors.Join / %w 保持 errors.Is/As 可用。
- 不自动重连恢复，不自动重试 Commit，不把 Commit 错误等同于“肯定没有提交”。测试特意模拟了实际提交成功但驱动返回错误，验证数据确实已保存而回调没有重跑。
- database/sql 的 ErrBadConn 丢弃路径不暴露物理 driver.Close 错误。因此不能承诺捕获该底层关闭错误；即使移除连接报告成功，也暂停整个数据库，不推断物理存储已恢复健康。故障测试覆盖了这种关闭错误被隐藏的情况，并确认后续业务被拒绝。

## Context 和借用约束

WithTx 使用请求 Context 获取连接，SQL 也必须使用调用方 Context。
BeginTx 使用 context.WithoutCancel(ctx)，由封装独占管理事务清理，而不让 database/sql 的后台自动 Rollback 把真实回滚错误掩盖为 ErrTxDone。封装在事务回调前和提交前检查取消；已成功提交后不再因为晚到的取消返回失败。

这意味着业务回调必须同步、可配合取消，仅包含数据库操作。不要在里面启动异步 SQL、执行外部文件操作或等待不响应取消的工作。Context 取消无法强制打断任意 Go 回调；现有任务 drain/关闭协调仍然必要。

WithAccess 复用现有 RWMutex 的独占锁，不增加另一套生命周期锁。锁等待本身不可取消，但获取锁后会再次检查取消，保证取消的排队任务不进入业务。
Access 不得逃逸、并发使用或重入 Database。通过同一个 Access 开嵌套事务、在事务内申请 LegacyDB 都会立即返回错误。

## 测试结果

| 环境 | 范围 | 结果 / 日志 |
| --- | --- | --- |
| Linux/WSL | go test ./internal/... ./desktop -count=1 | 通过，linux-regression.txt |
| Linux/WSL | persistence + desktop 整包 race | 通过，linux-race.txt |
| Linux/WSL | 新增 12 组事务测试连续 10 轮 | 通过，linux-transaction-tests.txt |
| Windows amd64 | persistence 整包，真实 Windows 临时文件系统 | 61 个顶层测试通过，其余平台限制用例按原测试跳过，windows-persistence.txt |
| Windows amd64 | 同一批新增事务测试连续 10 轮 | 通过，windows-transaction-tests.txt |
| Windows amd64 | desktop 整包 | 已有 TestAPILogsKeepBindingContract 失败：日志列表为空。不是全绿，windows-desktop-native.txt 与对应 stderr |
| Windows amd64 | desktop 排除上述已知日志用例 | 66 个顶层测试通过，两个 Linux 专用用例按原测试跳过，windows-desktop-except-known-log.txt |

新增测试覆盖：正常提交、业务/SQL 错误整体回滚、原 panic 传播、取消后回滚、长查询中断、真实外部读锁 Commit BUSY、延期外键 Commit 失败、异常连接移除及所有入口暂停、嵌套/过期范围拒绝、回滚错误合并（含取消和 panic）、提交结果不确定时不重跑、提交后取消仍成功、Begin 失败不运行回调、关闭等待事务清理、取消排队请求、并发请求保持单连接。

真实驱动故障与注入故障分开：锁冲突和延期外键由真实 modernc SQLite 触发；Rollback/Close 故障和“实际提交后返回错误”通过包装真实 SQLite 驱动的测试连接注入，不等同于真实设备 I/O 故障。

## Windows 测试环境与既有失败

测试 exe 在 WSL 交叉编译，实际在 Windows 运行，未替换固定目录程序。
测试目录：C:\Users\jhsong\AppData\Local\Temp\StudyFlow-tx-stage2-NbY9YZd6。

最初直接用 WSL interop 启动 desktop.test.exe，子进程重启时 os.Args[0] 只有文件名而找不到 exe。Windows 原生 PowerShell Start-Process 指定绝对路径及工作目录后解决。
当前主机 PATHEXT 为 .CPL，导致既有 ACL 测试找不到 icacls.exe。仅在测试 PowerShell 进程内补齐 System32 路径和 PATHEXT=.COM;.EXE;.BAT;.CMD，没有修改系统持久环境或应用代码。
windows-desktop.txt 保留最初启动方式的失败记录，最终结果以 native 和 except-known-log 日志为准。

日志列表失败已在 docs/windows-database-qa-2026-11-15/fix-72/README.md 第 21 行记录；该路由仍走原免数据库路径，本阶段未改日志实现，也未把它算作通过。

## 尚未完成的后续工作

- 第三阶段：现有七处业务事务迁移、科目/计划/计时一致性修复、查询错误处理、业务 SQL Context 传递及后台任务接入。当前 LegacyDB 仍是过渡入口，原有业务缺陷尚不能算作已修复。
- 第四阶段：备份、目录切换、退出、启动初始化的针对性适配和故障回归。现有相关整包测试已随本次回归执行，不等于已完成新的事务交错场景验收。
- 第五阶段：磁盘满、进一步 I/O 故障、可控内存错误、强杀恢复和双平台综合验收。
- 未运行 Linux GUI，不需要为本次数据库改造安装 Linux GUI；未执行 Windows 产品 GUI 验收，不将源码 test.exe 当作产品界面测试。
- 不引入 Goose、不执行降级、不递增版本。
