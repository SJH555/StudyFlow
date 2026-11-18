# 数据库事务重构：第三阶段

日期：2026-11-18。状态：第三阶段完成；等待用户确认后进入第四阶段。

后续进展：用户已批准第四阶段，完成结果见 ../database-transaction-stage4-2026-11-18/README.md。以下内容保留第三阶段结束时的记录。

本阶段把业务接入第二阶段的统一事务封装，并修复业务原子性和查询错误遗漏。仍共用原来的 `*sql.DB`，最大打开连接和最大空闲连接均为 1。事务期间借用的 `sql.Conn` 来自这个池，不额外创建连接池。

## 实际改动

| 范围 | 结果 |
| --- | --- |
| internal/infrastructure/persistence/queries.go | 新增带 Context 和借用范围的 Queries，所有 SQL 实际调用 Context 方法；事务内只允许使用事务执行器。拒绝嵌套事务、过期范围、取消后执行以及事务内绕回外层执行器 |
| internal/modules/ | 原有七处业务事务全部改用统一 WithTx；业务模块不再持有原始 sql.DB，不再自行 Begin/Commit/Rollback |
| subjects | 科目修改/删除与关联计划状态更新在同一事务；关联更新失败立即报错，前面的科目更新回滚 |
| plans | 冲突检查和新增/更新在同一事务；更新检查 RowsAffected；为计时读取计划边界时拒绝无效时间、时长和星期数据 |
| sessions | 开始计时、手动补录、调整计时、批量到期结算、休息提醒读写均纳入事务；开始计时只把 sql.ErrNoRows 当作没有旧计时，其他查询和解析错误立即返回 |
| goals | 新建和内容更新通过统一事务；内容写入后的回读失败也撤销写入。单条验收状态更新保持 SQLite 隐式事务 |
| review | 待办检查、生成复盘、保存每日目标、冻结历史目标、单次/批量提交使用统一事务；新增每日目标去掉 INSERT OR IGNORE，约束错误正常返回 |
| settings | 读取旧设置及批量写入在同一事务；清空表的依赖检查与 DELETE 在同一事务；设置回调只在提交成功后登记，并在 API 释放数据库借用后执行 |
| 查询 | 补全 Scan/Rows.Err 检查，停止静默跳过坏行；计划提醒提前返回时关闭 Rows，避免占住唯一连接 |
| desktop + internal/app | API 和主要后台数据库任务传递任务 Context；Scheduler.WithContext 返回任务专用副本，不修改共享调度器 |
| internal/testutil | 测试通过真实 Database 借用创建业务 Queries，配合范围限制；临时数据库与日常数据库隔离 |

借用关系：

```text
API / 后台任务（携带 Context）
  → Database.WithAccess（已有生命周期独占锁）
    → Queries（绑定 Context，只提供 SQL 与 WithTx）
      → 业务方法
        → WithTx（需要多步原子性时）
          → 第二阶段 Access.WithTx
            → 从唯一连接池借用 sql.Conn → Begin → SQL → Commit / 清理
  → 释放借用后执行已提交设置的回调
```

普通 SQL/业务错误立即返回，统一封装尝试整体回滚，不重跑回调或提交。保留原始业务错误，清理失败时合并错误。panic 继续传播并触发清理；Begin/Commit/真正的 Rollback 失败沿用第二阶段的连接隔离和数据库暂停规则。

修改日程前冻结历史目标仍是**独立事务**：冻结成功后，即使新计划因输入错误被拒绝，已冻结的历史目标仍保留。这个既有业务边界由 TestApplyScheduleChangeFreezesBeforeChangeAndKeepsSnapshotOnFailure 验证；单次冻结过程内部发生错误则全部回滚。没有把整个 API 路由强行合并成一个事务。

## 测试结果

| 环境 | 范围 | 结果 / 日志 |
| --- | --- | --- |
| Linux / WSL | go test ./internal/... ./desktop -count=1 -timeout=120s | 通过，linux-regression.txt |
| Linux / WSL | 同范围 -race -count=1 -timeout=180s | 通过，linux-race.txt |
| Linux / WSL | TestQueries / TestBusinessTransaction / TestReminderEarlyReturn 连续 10 轮 | 通过，linux-repeat.txt |
| Windows amd64 | persistence 整包 | 64 个顶层测试通过，2 个平台限制跳过，windows-persistence-native.txt |
| Windows amd64 | app、goals、plans、sessions、settings、subjects 整包 | 分别通过 15、3、3、7、6、1 个顶层测试；app 另有 1 个平台限制跳过，windows-*-native.txt |
| Windows amd64 | desktop 整包 | 78 个通过，2 个平台限制跳过；1 个既有日志列表测试失败，windows-desktop-native.txt |
| Windows amd64 | desktop 排除既有日志列表测试 | 78 个顶层测试通过，2 个平台限制跳过，windows-desktop-except-known-log.txt |
| Windows amd64 | 12 组业务故障测试、3 组 Queries 测试、1 组提醒释放连接测试，各连续 10 轮 | 全部通过，windows-desktop-repeat.txt / windows-persistence-repeat.txt / windows-plans-repeat.txt |

新增故障测试覆盖：

- 科目更新后关联计划 SQL 失败，科目原值不变。
- 旧计时无法扫描/解析时不再创建第二条活动计时。
- 新计划边界数据无效时，撤销前面结束旧计时的修改。
- 列表扫描失败、SQLite 在部分结果之后发生迭代错误，API 报错并释放连接。
- 目标写入后触发器制造不可解析的回读结果，内容更新回滚。
- 批量设置的第二次插入失败，第一次写入回滚且不执行回调。
- 设置成功后的同步回调可以重新借用数据库，无锁重入死锁，且读到已提交值。
- 单次/批量复盘后续明细失败，头表、明细和本次新建快照全部回滚。
- 快照插入遇到重复键，返回错误并撤销本次写入，不使用 IGNORE 吞掉约束错误。
- 到期结算的第二条更新失败，第一条更新也回滚。
- 12 个并发冲突计划创建仅允许一个成功。
- 真实外部 SQLite 读锁使开始计时 API 的 Commit BUSY：旧计时恢复、新记录不存在、后续 API 和借用被拒绝。
- 过期 Queries、延迟 Scan、嵌套事务、外层执行器绕过事务、取消 Context 均被拒绝；原始业务错误保持可识别。
- 计划提醒在找到结果/扫描失败后仍能立即执行下一次写入，避免单连接自等待。

真实读锁故障使用另一个 SQLite 连接模拟外部程序，仅用于测试。业务 SQL/数据错误使用临时库触发器和坏数据注入；不代表已模拟真实设备损坏、断电或进程级 OOM。第二阶段已有的 panic/回滚错误/提交结果不确定测试随本次 persistence 全量回归再次执行。

## Windows 执行与既有失败

测试程序在 WSL 交叉编译，实际由 Windows PowerShell Start-Process 运行，返回退出码并保存 stdout/stderr。测试目录为：

`C:\Users\jhsong\AppData\Local\Temp\StudyFlow-tx-stage3-sUh11qEG`

可复用启动脚本保存在 run-windows-tests.ps1。仅在该 PowerShell 进程中补齐 System32 路径和 PATHEXT，未修改系统持久环境。

TestAPILogsKeepBindingContract 仍因日志文件列表为空失败，完整失败记录和 stderr 均保留。此前 docs/windows-database-qa-2026-11-15/fix-72/README.md:21 及第二阶段已记录同一问题；本阶段未改该免数据库路由的日志实现，不把整包结果描述为全绿。

## 范围与剩余阶段

- 未修改数据库结构、应用版本和前端；保留工作区之前已有的其他修改。
- 未替换固定目录的 StudyFlow.exe，未修改日常数据库、用户文件和设置。Windows 本阶段为原生后端测试，并未进行产品 GUI 验收；无需 Linux GUI。
- 第四阶段仍需处理并专项验证备份、切换数据目录、退出、启动初始化与本次事务边界的配合。为保证编译所做的读取适配和现有测试回归，不等于这些新交错场景已验收。
- 第五阶段仍需验证磁盘满、进一步 I/O 故障、可控内存错误、强杀恢复及双平台综合验收。此时不能承诺系统级故障 100% 恢复。
- 不引入 Goose、不执行数据库降级、不新增错误分类/自动重试体系。
