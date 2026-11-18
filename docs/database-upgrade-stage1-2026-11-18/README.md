# Goose 数据库升级：第一阶段

日期：2026-11-18。第一阶段核心模块、事务适配和测试已完成。本文保留第一阶段交付时的范围；第二阶段已获批准并完成接入，后续状态见 [第二阶段记录](../database-upgrade-stage2-2026-11-18/README.md)。

## 本阶段完成

新增 `internal/infrastructure/upgrade/`，与 persistence 并列。当前 APP 目标数据库版本手工固定为 1，与 APP 发版号分离。基线 SQL/结构清单编译入程序，数据库已有字段和业务数据不变。

| 位置 | 职责 |
| --- | --- |
| upgrade/version.go | TargetVersion=1、内嵌 SQL、兼容错误类型 |
| upgrade/baseline.go、baseline.json | 冻结当前九表结构；忽略字段顺序，比较类型/默认值/主外键/索引；拒绝未知对象、额外约束等 |
| upgrade/manager.go | 只读检查、拒绝新版数据库、基线登记、按顺序升级至 APP 目标版本 |
| upgrade/scripts.go | 目标版本隔离、升级脚本事务规则检查；支持触发器 StatementBegin 与 CASE END |
| persistence/transaction.go、queries.go | 复用已有事务核心供 Provider 在已借用连接上调用；普通业务 WithTx 仍用同一核心；拒绝嵌套/过期调用 |
| third_party/goose/ | 固定 v3.26.0；两个文件的定制适配。来源、摘要、补丁和后续维护要求已写入仓库 |
| upgrade/manager_test.go、persistence/provider_transaction_test.go | 核心升级与真实 Goose 调用链的异常测试 |

数据库版本仅以 `goose_db_version` 为准，不设置 user_version，也没有第二张 StudyFlow 自定义版本表。

已有无记录完整基线：一次事务登记 0、1，跳过基线建表，不执行当前旧初始化代码中的颜色/短手动记录/验收状态转换。识别路径永久保留，测试覆盖从未纳管的 1 连续升级至 10。新库通过 Goose 执行基线 SQL。

单文件 SQL 和对应版本记录共用一个事务。文件 2 提交、文件 3 失败时停在 2；关闭重开后从 3 继续。禁止 NO TRANSACTION、脚本显式提交/回滚、连接级配置和环境替换等破坏事务约束的操作。不执行 Down，不提供数据库降级。

## 为什么需要适配 Goose

原版 Provider 自己取得 sql.Conn 并管理 sql.Tx，没有替换事务执行器的公开配置。外套现有 WithTx 会在单连接池上再次申请连接，可能死锁。

本次保留 Goose 的文件解析、执行顺序、UpTo 和版本管理；增加宿主事务回调，把它已借用的连接交给 persistence 的同一事务核心。SQL 与版本记录一起传给回调；管理表创建和初始 0 记录同样经过此入口。

Begin、Commit、真实 Rollback 失败仍隔离异常连接并暂停数据库，不重试业务。panic 仍执行回滚，原 panic 继续传播。另补齐 Provider 在初始化管理表时 panic/Goexit 的连接与互斥锁释放；测试同时覆盖初始化和文件执行中的 panic。

这属于 Goose 的本地定制，并非原版提供的配置；将来更新 Goose 需要核对补丁并重新验证。详见 `third_party/goose/STUDYFLOW.md`、`third_party/goose-studyflow.patch`、`third_party/goose-upstream.json`。

## 已验证的行为

- 完整当前旧库只读识别；Inspect 不建版本表、不修复数据库；列顺序改变仍可识别。
- 数据保持：不足 10 分钟的手动学习记录、已完成但未验收目标等不会因登记基线重新执行旧转换。
- 新库建立、重复调用、从无记录基线连续升至 10、目标版本隔离、新库版本大于 APP 时拒绝。
- 缺表/多列/陌生索引/触发器、约束/默认值/外键不符、缺失/重复/回退的版本记录、升级 SQL 缺号/重复、检查状态过期时拒绝。
- SQL 中途错误：当前文件之前的数据更新和 DDL 全撤销；之前文件已提交部分保留。关闭重开后可从失败文件继续。
- Goose 版本记录插入失败：该文件的业务 SQL 一起回滚。
- 延期外键在 Commit 阶段真实失败：异常连接被隔离，后续入口被拒绝；关闭重开后版本及 DDL 均未残留。
- Goose 实际调用链中的业务校验错误、SQL 错误、取消、文件执行 panic、管理表创建 panic、受控 IOERR/NOMEM，以及回滚失败/提交结果不确定时的保守隔离。
- 实际 SQLite FULL：临时数据库 max_page_count 限制触发，不耗尽系统磁盘。实际 NOMEM：临时连接 VDBE 指令限制触发，不耗尽整机内存。关闭重开验证本文件数据和版本均未残留。
- 借用连接释放、单连接配置不变、外层 SQL/嵌套事务和过期回调拒绝。
- Goose StatementBegin 触发器实际执行；SQL 字符串、注释及 CASE END 不被事务规则误拦截。

受控 IOERR/NOMEM 是故障注入，不声称模拟了硬件故障。现有持久化整包的真实 Linux IOERR、FULL、NOMEM、强杀、目录切换恢复测试也随回归运行。断电、硬件损坏、Go 运行时致命 OOM 不能据此保证恢复。

## 测试记录

| 环境 | 检查 | 结果 / 日志 |
| --- | --- | --- |
| WSL / Linux | `go test ./... -count=1 -timeout=180s` | 全仓通过，linux-regression.txt |
| WSL / Linux | persistence、upgrade 整包 race | 通过，linux-race.txt |
| WSL / Linux | persistence、upgrade go vet | 通过，linux-vet.txt（无诊断） |
| WSL / Linux | Goose 上游 Provider 等相关单元回归 | 通过，goose-provider-regression.txt；未运行依赖未复制 CLI 的测试 |
| Windows amd64 原生 | upgrade 整包 | 15 个顶层测试通过、无跳过，windows-upgrade.txt |
| Windows amd64 原生 | persistence 整包 | 82 个顶层测试通过、3 个平台/辅助入口跳过，windows-persistence.txt |
| Windows amd64 原生 | desktop 整包 | 仅既有 TestAPILogsKeepBindingContract 失败（日志列表 files 为空），windows-desktop.txt；未计为通过 |
| Windows amd64 原生 | desktop 排除既有日志用例 | 通过，windows-desktop-except-known-log.txt |

Windows 测试 exe 在 WSL 交叉编译，在 Windows 本地临时目录由 PowerShell Start-Process 启动。仅测试进程补齐 PATHEXT/System32，不修改系统环境。Windows 运行脚本、stderr 和临时目录位置已保存。

用户提供的 `D:\CLink\Desktop\data\study.db` 只做了 mode=ro 的九表列定义核对，与冻结基线一致（忽略字段顺序）；没有给用户原库写入版本记录。这次核对不替代第二阶段对完整启动流程的验收。

## 尚未实施：第二阶段

本阶段没有接入生产启动流程，没有弹窗，没有自动改写日常数据库，没有部署/替换固定目录程序，没有递增 APP 版本。第一阶段 Apply 是待启动流程调用的内部能力；调用方必须先完成确认及验证备份，不能把它直接挂到启动初始化后就算完成。

第二阶段需接入：受控打开与接管、业务启动前版本检查、确认和保留升级前备份、成功后继续启动、失败时退出、替换旧启动时反复初始化逻辑、新库初始设置，以及数据目录切换和既有历史结构路径的统一保护。不得让后台服务先启动或让目录恢复绕开版本限制。

已查看现有 SettingsDialogs.vue 的 modal-backdrop/modal、confirm-actions、Vuetify 按钮及 style.css 的共享表面/主题变量。启动 UI 必须沿用现有设计，测试正常同版本、无 Goose 旧库、可升级旧版、新版库拒绝、取消、备份/升级失败等界面。进入 Windows UI 测试前通知用户；Linux GUI 暂不安排。本阶段的原生 test.exe 测试不算 UI 测试。

旧的 `docs/database-version-compatibility-design.md` 已标注为历史草案，避免未来跨对话/跨设备开发误采用其中的双重版本记录或数据库回退方案。
