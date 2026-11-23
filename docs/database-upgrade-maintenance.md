# 数据库升级维护

本文件是后续设备/对话共同遵守的仓库规则；升级 SQL、版本目标和测试都保存在仓库中，不依赖聊天记忆。

## 当前状态与目录

当前目标数据库版本为 **2**，由 `internal/infrastructure/upgrade/version.go` 的 `TargetVersion` 手工维护。应用发版号独立；只修界面或 bug、不改变数据库结构/含义时，不增加数据库版本。

```text
desktop/main.go
  → desktop/startup_upgrade.go                 启动检查、确认、备份、成功后开放业务
  → internal/infrastructure/upgrade/           版本判断、SQL 顺序与目标隔离
      version.go                              APP 声明支持/要升级到的目标
      sql/00001_baseline.sql                  冻结的第一版建表与新库初始设置
      baseline.json、baseline.go               无 Goose 旧库的永久识别与结构校验
      manager.go、scripts.go                   UpTo、版本记录、脚本事务规则
  → internal/infrastructure/persistence/       唯一数据库生命周期与访问管理
      transaction.go                          共用事务核心、清理和异常连接隔离
      backup.go                               升级前验证快照、安全发布
      directory_switch_flow.go                 目录交接的版本保护
  → third_party/goose/                        固定上游版本 + 宿主事务适配

vue/src/App.vue                                启动门禁
  → features/startup/StartupScreen.vue          确认/执行/失败界面
  → Workspace.vue                             准备完成后才挂载原工作区
```

生产打开必须走 `prepareStartupUpgrade`，不得改回 `persistence.Open/OpenForStartup` 的旧初始化路径。桌面旧构造器已移到 `desktop/legacy_fixture_test.go`，只供测试使用。底层旧初始化函数仍供历史转换/测试复用，不是新的启动入口。

## 后续新增版本

**实施前置条件：**遵守根 `AGENTS.md` 11.1 的数据库变更告知规则。发现需改变结构或持久化含义时，立即告知用户，明确获得新增增量 SQL 的同意后，才执行以下步骤；普通功能授权不替代该项确认。

以已经批准的 1 → 2 为例，下一次从 00003 起继续编号：

1. 新增 `upgrade/sql/00002_具体改动.sql`，只写本次变化，包含 `-- +goose Up`。前缀 2 就是 Goose 版本 2，无须另写应用版本对照表。保持连续、不重复；下一次为 00003。
2. 将 `TargetVersion` 手工改为 2。只打 bug 补丁时保持 2。即使源目录已有 00003，目标为 2 的 APP 也不会执行它。
3. 同步修改读取/写入变化字段的业务代码，并在升级模块添加新版本的结构/业务不变量校验。版本 1 的冻结结构校验仍负责识别旧库，版本 2 另校验新增表、会话引用列和历史覆盖日期。
4. 增加从 1 → 2、新库直接创建到 2、无 Goose 基线 → 2、同版本不重跑、数据库 2 被目标 1 APP 拒绝、失败后重新打开继续等测试。
5. 运行下述回归与构建，保留记录后再交付。发布过的 SQL 和 baseline.json 不可随意修改；修复已有结构用新编号 SQL。未发布测试脚本的故障修正不等同于修改已发布历史。

字段改名、单位变化、拆表的含义必须明确写在本次 SQL、相邻注释和测试预期中。程序无法从名字自动推断含义；不得靠缺字段就跳过来猜测转换。维护这些文件即可跨设备重现规则。

```sql
-- +goose Up
-- 在此描述本文件的前置条件、单位、字段变化和预期结果。
-- 本文件的全部 SQL 与 Goose 版本记录由宿主共用事务提交。
```

不得手工插入/修改 `goose_db_version`，不使用 user_version，不调用 Down。数据库比 APP 新时必须换用支持该数据库版本或更高目标的 APP，不做数据库降级。

**范围限制：**版本门禁只存在于本次及以后包含该功能的 APP。已发布、未包含 Goose/版本检查的旧可执行文件无法通过更新数据库获得新代码，不能保证它会主动提示不兼容。不要再用这类旧程序打开已纳管数据。今后的受管 APP 版本按目标版本正常隔离。

## 事务规则

- 普通业务和 Goose 使用 persistence 同一个事务核心、同一个 live study.db 单连接池。副本校验临时打开的是独立备份文件，不是为 live study.db 增加连接。
- SQL 文件中不写 BEGIN/COMMIT/ROLLBACK/SAVEPOINT，不使用 NO TRANSACTION、连接级 PRAGMA、ATTACH/DETACH、VACUUM 或环境变量替换；不要在外面再套一层事务。
- 一条 SQL 返回错误就停止，当前文件的全部 DML、DDL 和版本记录一起回滚。panic/取消同样清理；Commit 或清理结果不确定时隔离连接、停止业务，不盲目重试。
- 文件 2 已提交、文件 3 失败，只撤销 3，保持版本 2；下次确认并备份后从 3 继续。不能声称所有文件共用一个总事务。
- SQL 没报错不代表语义符合预期，例如“应更新一行，实际更新零行”。作者要通过约束或 SQL 断言显式表达前置/后置条件，并测试不满足时整个文件回滚。不允许吞掉错误或用幂等重跑掩盖失败。

## 版本 2 的计划历史

- 升级直接把 `subjects`、`plans`、`daily_plan_targets` 合入项目和计划版本表。稳定 ID 与版本行 ID 独立；日常业务和历史统计以生效日和保存顺序选取版本。
- `plan_versions.kind` 区分循环计划、旧版每日目标和历史完整性边界。旧版目标按原日期原样保留，循环排程只读取循环计划记录。
- 创建项目后当天可以手动补录。循环计划及项目状态的后续编辑从北京时间次日生效。移除排程或逻辑删除都追加版本，已发生的实际学习记录继续引用原 ID。
- 迁移当天沿用升级时记录的排程，从次日起具备完整计划历史。更早日期只显示已有的旧复盘目标或「历史计划未知」。现有实际时间戳和已提交复盘原样保留。
- 新会话引用启动时的计划版本；迁移只为未完成会话绑定升级时保留下来的计划配置，旧的已完成记录无需回填版本。

## 无 Goose 旧库与首次启动

完整已发布九表基线：只读识别，用户确认并生成验证备份后，一次事务登记 0、1，跳过基线 SQL 和旧清理逻辑。识别路径永久保留，即使未来用户直接从该旧版升到目标 10，也先登记 1，再执行 2…10。

明确匹配的三种更早五表结构、缺目标表/仅缺验收时间列的结构：备份后才执行原历史转换，再登记版本。其他缺字段、额外约束、陌生对象或写入冲突策略不做猜测修复。

全新库执行基线 SQL；已有的无版本空文件拒绝，避免把异常文件当新库覆盖。创建中断留下的 Goose 版本 0 空库允许继续。业务接口、后台提醒、自动备份和工作区在准备完成前均不可使用。

## 备份与恢复

- 确认后先生成 `当前数据目录/backups/upgrade-起始-to-目标-时间-随机值.db`，验证完整性、外键、结构和版本后发布；成功才开始升级。创建/验证失败不执行升级。
- `upgrade-*.db` 不参与普通 `study-*.db` 自动备份的到期删除；随数据目录切换一起保留。它们可能包含敏感学习数据，按普通本地用户数据管理。
- 正常退出会取消正在执行的升级，等待事务清理再结束进程；强杀后的恢复由 SQLite 在实际下次打开时进行，再检查已提交版本。
- 没有自动将备份覆盖回原库的功能。必要的人工恢复应先完全退出 APP，完整保留当前数据目录及 SQLite sidecar，验证候选备份，再使用离线恢复流程；不要在运行中覆盖 study.db，不要凭名称删除 WAL/journal。
- 恢复到较旧备份会丢弃备份之后的变更，必须由数据所有者明确决定；这与 APP 自动降级是不同操作。本次未新增导入/导出/一键恢复界面。

## 测试与交付

日常开发按改动范围选择验证，遵循根 `AGENTS.md` 12.3。下列命令及历史故障场景供数据库重大改造、相关机制修改或发布验收时选用，不要求每次普通业务修改全部执行。

先完成产品前端构建，再运行包含嵌入资源的 Go 全仓/race，避免一边替换 `desktop/ui/` 一边编译导致资源文件消失。

```bash
# 构建 Windows 测试程序，不递增 APP 版本
wails3 task windows:build ARCH=amd64

go test ./... -count=1 -timeout=180s
go test -race ./desktop ./internal/infrastructure/persistence ./internal/infrastructure/upgrade -count=1 -timeout=180s
go vet ./desktop ./internal/infrastructure/persistence ./internal/infrastructure/upgrade
python3 scripts/qa/check-goose-fork.py
```

Windows 需要原生运行交叉编译的测试 exe，不能用“编译通过”替代。执行中退出/强杀测试与复现脚本见 `docs/database-upgrade-interruption-2026-11-18/README.md`。跨系统可以比较关闭后的数据库结构与数据；不要用 WSL SQLite 同时读取 Windows 正在使用的 WAL，原生 Windows 校验器用于这类测试。

替换程序前先正常退出并保留用户数据。只有用户明确要求发布，才使用会递增版本的 build/package 任务。验收调试构建可用 `scripts/qa/quit-upgrade-fixed-windows.mjs` 调用应用自身的 runtime Quit；正式程序由用户通过托盘菜单正常退出。不要向 Wails 隐藏控制窗口直接发送 WM_CLOSE，它的默认处理可能提前销毁主线程调度窗口。

物理断电、硬件损坏、文件系统失真、Go 致命 OOM 无法由这些测试保证 100% 恢复。当前已有日志列表/音频查询的历史 Windows 测试问题应如实列出，不冒充全仓全部通过。

## 更新 Goose

根 go.mod 的 replace 必须保留，来源、摘要、复制范围见 `third_party/goose-upstream.json`，补丁见 `third_party/goose-studyflow.patch`，适配说明见 `third_party/goose/STUDYFLOW.md`。这不是上游公开配置，不能直接删除 replace 换回原版。

更新时重新固定来源、核对并应用补丁、更新摘要和补丁记录，运行 fork 摘要检查、上游 Provider 回归及宿主事务故障/恢复整套测试。检查脚本用于发现文件漂移，不代替人工审查或证明新上游行为正确。
