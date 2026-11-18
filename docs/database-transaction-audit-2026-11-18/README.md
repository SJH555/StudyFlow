# 数据库事务审查

日期：2026-11-18。仅审查与临时数据库测试，未修改业务代码或实际用户数据。

## 结论

生产代码有 7 处显式事务，全部检查 Begin 和 Commit 返回值，6 处有 defer Rollback。手动补录、计时到期结算、目标内容更新、单次/批量追因提交、批量设置保存，基本遵循“事务内错误返回 + 延迟回滚 + 提交错误返回”的结构。

该结构适合普通事务，但当前程序存在错误遗漏和业务原子性缺口，且所用 database/sql + modernc.org/sqlite v1.44.3 的某些 COMMIT 失败不能靠随后调用同一个 sql.Tx.Rollback 清理。

## 已复现问题

### 1. 提交失败后的 SQLite 事务可能仍留在连接上（P1）

位置：internal/modules/sessions/sessions.go:241；其他 Commit 调用也没有针对这类连接状态的处理。

使用正常结构的临时数据库，另开一个读事务模拟 SQLite Browser。调用实际 sessions.Start，提交返回 SQLITE_BUSY；紧接着 db.Begin 返回 cannot start a transaction within a transaction。附带的延期外键探针也证明，即使补上 defer tx.Rollback，Commit 失败后 Rollback 仍返回 sql.ErrTxDone，未触达 SQLite。

原因：Go sql.Tx.Commit 调用驱动前即标记 Tx done；modernc 驱动 Commit 只执行 commit，返回错误后没有自动回滚；连接有效性检查没有排除仍处于事务中的连接。因此不能简单声称“Commit 失败后 defer 必然回滚”。

建议：保留普通事务模板；补充提交/回滚异常时的连接健康处理，在独占持有连接的边界确认清理，无法确认时隔离连接/暂停数据库访问并安全重建。不能在并发共享 db 上盲目 Exec("ROLLBACK")，也不能无条件重试整次业务或宣称提交成功。外部锁虽低概率，已可在当前真实调用路径复现。

### 2. 项目归档的第二次更新失败被吞掉（P1）

位置：internal/modules/subjects/subjects.go:115、124；Delete 在 137、146 有相同的错误忽略写法。

Update 先保存项目状态，再执行 plans.SetSubjectActive，但第二步返回值被丢弃，也没有统一事务。临时触发器让计划更新失败后，Update 返回 nil，数据库留下 subject.archived=1、plan.active=1。

建议：把项目更新和计划状态更新纳入同一事务并检查全部错误。SQL 帮助函数要接受 tx，不能开启事务后又调用使用 db 的帮助函数，否则唯一连接被事务占用会造成自等待。

### 3. 开始学习吞掉旧会话查询错误，且缺少 defer Rollback（P1/P2）

位置：internal/modules/sessions/sessions.go:199、206–226。

QueryRow.Scan 只有 nil 被处理，非 sql.ErrNoRows 的错误也会被后续 plans.EndForStart 覆盖。临时数据库把旧会话 duration_seconds 设为不可扫描成整数的文本，Start 仍返回成功，并留下两条 end_at IS NULL 的活动会话。

建议：只把 sql.ErrNoRows 视为“没有旧会话”；其他错误立即返回。Begin 成功后统一 defer Rollback，补齐 panic/未来新增 return 分支的资源释放路径。defer 不能替代第 1 项中的提交失败连接处理。

### 4. 启动初始化/历史迁移可以部分完成（P1）

位置：internal/infrastructure/persistence/sqlite.go:355 起。

建表、补字段和数据转换逐条使用 db.Exec，没有整体事务。现有 TestOpenForStartupReportsPossiblePartialMigration 已明确验证：后面颜色转换失败时，前面插入的设置仍保留。本次重新运行通过，证明当前行为确实是部分更新后拒绝启动。

建议：适合 SQLite 事务的 DDL 与数据更新按明确整体迁移边界执行；错误时回滚，迁移前保留可验证备份。这属于普通业务事务之外、但当前代码真实存在的遗漏。

### 5. 查询错误被当成空/部分数据返回（P2）

位置：sessions.List（sessions.go:65、88）、subjects.List、plans.List、settings.listSettings 等。

这些列表查询存在 Scan 失败跳过行以及缺少 rows.Err 检查。实际探针证明一条不可扫描的学习记录会被静默省略，List 返回空切片和 nil；这不是物理删除，但页面会表现得像记录消失。

建议：Scan 错误立即返回，遍历后检查 rows.Err，保留及时 Close。现有代码多数查询确实关闭了 Rows，不能笼统称为全部未关闭。事务中两处手动关闭 Rows 的正常路径已检查；仍宜增加 defer 作为异常路径兜底。

## 其他缺口及合理例外

- 所有 7 处事务均使用 Begin/Exec/Query，而不是 BeginTx/ExecContext/QueryContext。桌面 tasks 有取消 Context，但 API beginWork 丢弃它，也未传至 SQL。当前退出策略包含等待任务排空，因此不用取消并非自动等于数据损坏；但不具备请求取消/超时控制，数据库操作卡住时退出等待也可能拖长。建议按业务语义传入 Context，不能把取消等同于立刻结束 SQLite 所有底层操作。
- plans.Create/Update 的冲突检查与写入分离；ChangeTimer 的读状态、计算和更新也未被一个业务事务包围。API 使用 Database.WithRead 允许多个请求同时进入，单连接仅串行每条 SQL。存在 A检查、B检查、A写入、B写入这样的交错可能；本项为代码路径分析，未宣称已复现并发冲突。可用事务或明确的业务独占边界保护。
- 所有延期/手动 Rollback 都没有处理其错误。正常成功 Commit 后出现 ErrTxDone 应忽略；真实回滚错误应保留主错误并记录或触发连接健康处理，不能把失败连接默认为干净。
- 成功事务全部检查 Commit 错误，未发现“已知 Commit 返回错误却直接返回成功”的写法。第 2 项属于事务外多步业务吞错误，不应混为 Commit 漏检。
- 单条 INSERT/UPDATE/DELETE 本身是 SQLite 隐式事务；验收状态用一条 UPDATE 同时修改多个字段是合理设计，无须为了统一外观额外包显式事务。
- 自动备份使用 VACUUM INTO，不能放进普通事务；目录切换涉及文件发布及位置记录，SQL Rollback 无法撤销文件操作，应保留现有独占锁、快照与恢复协议。
- 无须为每一种 SQLite 错误码写不同的普通业务回滚分支；但 Commit/回滚后的连接状态不能被统一模板掩盖。
- 进程正常展开栈或被上层 recover 的 panic 可以依靠 defer 尝试回滚；os.Exit、强制终止或断电不会执行 defer。SQLite 恢复保障受事务边界、日志/同步设置及文件系统正确持久化的约束，不能保证事务外多个步骤全部撤销。

## 验证与复现

WSL 执行 go test ./internal/...，现有测试全部通过；额外 6 个审查探针及部分迁移既有用例见 audit-results.txt。本次未执行 Windows 故障测试、真实断电、磁盘满、OOM 或 panic 故障注入，不将其报告为已验证。

audit_test.go.txt 保存独立探针源码（使用 .txt 避免被当成生产测试包）。所有数据库均由 testing.T.TempDir 创建。审查时通过 Go overlay 把探针附加到 persistence 的已有测试文件，无生产代码变化。

可复现方式：将 audit_test.go.txt 复制至仓库外临时文件，用 Go overlay 替换 internal/infrastructure/persistence/startup_goal_upgrade_test.go（仅替换测试文件），执行 go test -overlay <overlay.json> ./internal/infrastructure/persistence -run TestAudit -v -count=1。探针成功表示问题被观察到，不表示程序没有问题。

优先级：先修复业务吞错及原子性、Start 的错误分支、提交失败连接复用，再改进 Context 和迁移事务；不需要先引入庞大的事务恢复框架。
