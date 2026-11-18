# 数据库事务重构：第一阶段验证

日期：2026-11-18。状态：第一阶段完成，等待用户明确同意后才能开始第二阶段。

后续进展：用户已明确同意引入 sql.Conn 并开展第二阶段；结果见 ../database-transaction-stage2-2026-11-18/README.md。上面的状态保留为第一阶段完成时记录。

## 范围与结论

仅新增测试和本文档，未修改生产代码、数据库结构、应用版本和日常用户数据。
测试使用 Go database/sql + modernc.org/sqlite v1.44.3，数据库全部位于 testing.T.TempDir。
Linux 数据库位于 WSL Linux 文件系统，Windows 测试程序原生运行，数据库位于 Windows 临时目录。

已验证：

1. 正常 Commit 后数据保留；约束错误后主动 Rollback 撤销此前写入。
2. 普通 panic 展开栈时 defer Rollback 可以撤销写入，原 panic 继续传播至外层 recover。
3. 外部读事务在 DELETE journal 模式下阻止提交，真实返回 SQLITE_BUSY。
4. 延期外键在提交时失败，真实返回 SQLITE_CONSTRAINT_FOREIGNKEY。
5. 上述两个 Commit 错误后，sql.Tx.Rollback 返回 sql.ErrTxDone，但 SQLite 事务仍残留。
6. sql.Conn.Close 会把残留事务所在连接归还池；它不是异常连接丢弃机制。
7. 先 db.Conn 固定连接，再 conn.BeginTx，可以在 Commit 失败后继续持有该连接。此时同一单连接池的另一个调用只能等待或超时，不能借到异常连接。
8. conn.Raw 回调返回 driver.ErrBadConn 后，连接池 OpenConnections/InUse 都变为 0，旧 sql.Conn 不再可用。没有使用驱动私有字段。
9. 在上述两个故障场景中，丢弃连接后新连接看不到未提交数据，可以开启并提交新事务，foreign_keys 仍为 ON，连接池仍最多一个连接。

测试里的错误码判断只用于断言故障确实被触发，不是建议业务按错误码分别决定是否回滚。

## 下一阶段可采用的机制

在生命周期保护内固定 sql.Conn，通过该 Conn 开始事务。
正常失败时尝试 Rollback；Commit 失败或真正的 Rollback 失败时，走明确的连接丢弃路径。
不得把 Conn.Close 当作丢弃，不得用同一个已结束的 sql.Tx 重试回滚或重跑业务。
无法确认清理安全时，由现有 Database 阻止后续访问。

本阶段仅证明上述连接原语在两个真实故障场景下可用；统一事务封装、错误合并和不可用状态尚未实现。
后续需要验证关闭失败/回滚失败注入，不能因为本次成功丢弃就宣称任何 I/O 故障都可安全恢复。

## 测试与日志

测试源码：internal/infrastructure/persistence/connection_safety_test.go。

- Linux：三个顶层测试（共六个叶子场景）连续 10 轮通过，见 linux-results.txt。
- Windows amd64：同一测试程序在 Windows 原生执行，连续 10 轮通过，见 windows-results.txt。
- Linux race：上述测试连续 3 轮通过，见 linux-race-results.txt。

Linux：

```sh
go test ./internal/infrastructure/persistence -run '^TestConnectionSafety' -v -count=10 -timeout=90s
go test -race ./internal/infrastructure/persistence -run '^TestConnectionSafety' -count=3 -timeout=90s
```

Windows 测试程序由 WSL 交叉编译：

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o <Windows临时目录>/persistence.test.exe ./internal/infrastructure/persistence
```

在 Windows 原生运行该 exe（本次通过 WSL interop 直接启动 Windows exe）：

```text
persistence.test.exe -test.run=TestConnectionSafety -test.v -test.count=10 -test.timeout=90s
```

本次 Windows 测试程序位于 C:\Users\jhsong\AppData\Local\Temp\StudyFlow-tx-stage1-CWAiKnFz，不属于应用便携版或另一套日常使用数据。

## 尚未验证

统一事务封装、生产调用点、Context 竞态、真实回滚失败、磁盘满、I/O 故障、OOM、强杀后恢复、备份和目录切换回归均留待后续对应阶段。
未运行 Linux GUI 或 Windows 应用 GUI，本阶段不涉及 UI。没有替换固定目录中的程序。

## 后续阶段及固定约束

2. 统一事务封装，适配访问边界、Context 和锁；测试提交、整体回滚、panic、取消、清理失败。
3. 业务事务迁移，修复吞错、缺少原子性及查询错误；检查设置回调、后台任务、清空数据与历史快照边界。
4. 适配备份、目录切换、退出和启动初始化边界；测试失败恢复与生命周期协调。
5. 系统故障与双平台综合验收。

每阶段完成和测试后先汇报，用户明确同意后才能进入下一阶段。
保持共享单连接（MaxOpenConns=1、MaxIdleConns=1）；事务内任何已发现错误都停止执行并尝试整体回滚，不依赖 SQL 幂等性补救。
不引入 Goose、不改变数据库结构、不递增版本。未来数据库只允许 Up；旧 App 遇到新版数据库拒绝启动，提示升级 App。
所有破坏性测试使用临时数据，不需要 Linux GUI；Windows 必须实际执行，不能仅编译。
