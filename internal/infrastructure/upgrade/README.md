# 数据库升级

三阶段接入及最终验收已完成：先检查版本，现有旧库经用户确认并完成验证备份后升级，成功才开放业务接口和后台任务。当前 APP 目标数据库版本为 1。

## 职责

- version.go：手工维护 TargetVersion，与 APP 发版号独立。当前为 1。补丁发版不必增加数据库版本。
- sql/：编译进 APP 的 Goose SQL。编号和 Goose 版本直接对应，不另写 APP 版本对照表。
- baseline.json、baseline.go：冻结的版本 1 结构，识别无 Goose 记录的第一版已发布数据库，忽略字段顺序，比较字段、默认值、主外键、索引等。不执行猜测性修复。
- manager.go：只读检查、拒绝新版库、接管基线、按文件升级到 APP 声明版本。
- scripts.go：隔离目标版本以上的脚本；禁止关闭事务、显式提交/回滚、连接级 PRAGMA、环境替换等破坏升级规则的操作。支持 Goose StatementBegin 中的触发器和 SQL CASE END。

## 固定规则

版本记录只有 goose_db_version；不维护 user_version。数据库版本大于 APP 目标时拒绝，不提供 Down。当前版本 1 结构未改变，仅增加 Goose 的管理表。

无记录的现有完整基线库：确认和备份后，一次事务创建版本表并登记 0、1，不重新执行基线建表和旧版清理逻辑。以后即使 APP 目标是 10，也先接管为 1，再执行 2 到 10。此识别逻辑永久保留。

新库：执行 00001_baseline.sql 后登记 1。Goose 的初始 0 记录是管理表初始状态，不代表另一个业务结构。

每个文件的 SQL 和该文件版本记录在同一事务中。文件 2 成功、文件 3 失败时保留 2，下次从 3 继续。任何执行错误立即停止，不能靠重跑或幂等性掩盖失败。

upgrade 只通过 persistence.Access 借用既有单连接池。Goose 的定制接口调用 persistence 共用事务核心，无嵌套事务、无额外连接池。Provider.Close 会关闭借来的整个池，因此模块不调用它；Provider 自行释放其临时借用的连接。

Inspect 只读，不让 Goose 提前创建管理表。Apply 要求调用方先确认、生成并验证升级前备份，并把同一次检查得到的 State 传入；进入 Apply 时重新检查。不能把这次重新检查当作能抵御任意外部工具修改数据库的文件锁。

SQL 成功不代表业务含义一定正确。SQL 作者必须用约束或显式检查表达预期，不能指望 Goose 自动发现“应更新 1 行，实际 0 行”。不得修改已经发布的基线、历史 SQL 或它们的既有语义；新变化增加新 SQL 并调整 TargetVersion。新增版本时补充目标结构/数据校验和跨版本测试。

## 启动与目录交接

`desktop/startup_upgrade.go` 管理检查、等待确认、备份、升级、准备完成/阻止启动。`StartupService` 仅暴露状态与确认；所有常规 `APIService` 方法在准备完成前拒绝。`vue/src/App.vue` 是启动门禁，准备完成后才挂载原工作区 `Workspace.vue`。提醒、自动备份等后台任务随后启动。

完整基线之外，只接纳已有代码明确支持并能严格匹配的三种五表历史结构，以及缺 stage_goals 表/仅缺 accepted_at 列的两种明确历史结构。先保留验证备份，再在同一事务执行原历史转换并登记 0、1；其余缺字段/未知对象拒绝，不猜测修复。新库的默认总结设置与基线建表同事务写入。

现有备份创建机制增加保留快照入口：`upgrade-*.db` 不参与普通自动备份到期清理，随数据目录切换复制。备份使用 SQLite VACUUM INTO，在副本上验证完整性、外键、结构及版本状态，关闭并同步后发布；失败不开始升级。副本校验会短暂打开独立备份文件，不增加正在使用的 study.db 的连接数。

数据目录切换在发布前检查源库与候选快照版本；启动恢复在清除待恢复标记/退役旧位置前检查候选库。不能借切换目录或恢复绕过新库版本限制。

第二阶段完整记录与测试结果见 `docs/database-upgrade-stage2-2026-11-18/README.md`。原 `Open/OpenForStartup` 初始化入口保留给历史单元测试/构造器，生产 main 已改为受控 `OpenUnprepared`，不得重新接回业务启动路径。

执行中正常退出、强杀、重复中断及恢复的最终补充验收见 `docs/database-upgrade-interruption-2026-11-18/README.md`；包含 DELETE/WAL 的真实 Windows 界面测试和原有九表逐行核对。

最终集成、固定目录真实数据升级及交付记录见 `docs/database-upgrade-stage3-2026-11-18/README.md`；后续 SQL 与版本维护遵循 `docs/database-upgrade-maintenance.md`。
