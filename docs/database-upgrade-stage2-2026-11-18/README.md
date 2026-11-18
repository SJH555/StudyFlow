# Goose 接入：第二阶段

日期：2026-11-18。启动检查、升级确认、验证备份、业务门禁、目录交接及 Windows UI 已完成；本阶段结束后等待用户批准第三阶段。没有递增版本，没有替换固定日常目录的程序，也没有改写日常数据库。

## 改动与效果

| 文件/目录 | 作用 |
| --- | --- |
| desktop/startup_upgrade.go | 打开后只读检查 → 同版本直接启动；旧库等待确认 → 保留验证备份 → 升级 → 启动；较新/未知/失败时显示原因并阻止业务 |
| desktop/main.go、api_service.go | 所有常规接口、工作区、提醒/自动备份循环在升级完成后才开放；待确认时关闭窗口直接退出；退出等待正在进行的升级清理 |
| desktop/startup_database.go | 复用原数据位置选择与恢复逻辑，但生产入口不再先运行旧初始化 SQL；恢复候选在退役旧位置前检查兼容性 |
| persistence/sqlite.go | 受控打开不建表；严格识别原有历史结构，再由升级事务调用原历史转换 |
| persistence/backup.go | 保留快照共用既有 SQLite 快照与安全发布机制，副本验证通过后才升级；upgrade-*.db 不被自动到期清理 |
| persistence/database.go、directory_switch_flow.go | 安装版本校验器；目录切换检查源库及候选快照，失败不发布新位置 |
| upgrade/ | 增加明确历史结构识别、只读备份/候选校验；新建库默认设置与基线建表同事务 |
| vue/src/App.vue、Workspace.vue、features/startup/ | 启动门禁与现有风格的确认/错误界面；原工作区移至 Workspace.vue，准备完成才挂载 |

现有完整无 Goose 九表库只登记 0、1，不重跑旧清理。明确支持的三种旧五表结构、缺目标表或仅缺验收时间列的历史结构，在备份后转换并原子登记。已有空库文件拒绝，真正首次创建的新库自动初始化；中断留下的版本 0 空库允许继续创建。未知结构不做猜测修复。

数据库版本仍只看 goose_db_version，目标仍手工固定为 1；不维护 user_version、不执行 Down。仍使用同一个 persistence 事务核心和单连接池，本阶段未修改 Goose 定制补丁。逐文件提交语义不变：前一文件已提交，后一文件失败不会撤销前一文件。

## 测试结果

| 环境/检查 | 结果 | 记录 |
| --- | --- | --- |
| WSL 全仓 go test ./... | 通过 | linux-regression.txt |
| WSL desktop、persistence、upgrade race | 通过 | linux-race.txt |
| WSL 三包 go vet | 无诊断 | linux-vet.txt |
| Windows 原生 upgrade | 15 个顶层用例通过，无跳过 | upgrade-native-stdout.txt |
| Windows 原生 persistence | 83 个顶层用例通过，3 个平台/辅助入口跳过 | persistence-native-stdout.txt |
| Windows 原生 desktop 全包 | 94 个顶层用例通过、3 跳过；仅既有日志列表用例失败 | desktop-native-stdout.txt |
| Windows desktop 排除已知日志用例 | 通过 | desktop-except-known-log-stdout.txt |
| Vue 类型检查/Vite 与 Windows 产品构建 | 通过；未增版，产物 bin/StudyFlow.exe | windows-build.txt |
| Windows 真实 WebView 七场景 | 全部通过 | windows-ui-results.json、ui-*.png |
| UI 操作后数据库独立检查 | 通过 | windows-ui-database-checks.json |

既有失败 TestAPILogsKeepBindingContract 的实际断言仍为日志 files 列表为空；没有把全仓 Windows 宣称为通过。本阶段没有重跑无改动的 reminder 音频包，其此前音频查询失败仍保留为历史已知问题。

后端覆盖：只读等待确认与接口阻断、重复启动不重复备份、不重跑旧数据清理、并发确认仅执行一次、取消/备份失败/panic/退出取消、备份路径被普通文件占用、较新/未知/缺失/已有空库拒绝、五种明确历史结构转换前备份、保留备份跨到期清理与目录切换、源库/候选快照异常版本阻止发布、待恢复新库不退役旧库、业务初始化失败保留验证备份并关闭连接。

第一阶段覆盖的逐文件 SQL/版本记录原子性、SQL/Commit/panic/IOERR/NOMEM/FULL 等异常仍随整包回归。没有制造整机断电、硬件损坏或 Go 致命 OOM，不能对这些情况承诺 100% 恢复。

## Windows UI 验证方式

开始 UI 验证前已通知用户，先尝试 computer-use 技能。该工具因 WSL sandboxCwd 文件 URI 错误无法初始化，重置内核重试仍相同；改用 Windows 真实 Wails WebView2 的 CDP 接口，没有用浏览器假页面替代桌面运行。

仅测试构建标签 windows && upgradegui 启用 desktop/startup_upgrade_gui_test.go：调用真实启动控制器、服务注册、内嵌前端与桌面生命周期。测试进程使用空实例守卫，不打断日常实例；fixture 必须有专属标记。编译器 overlay 只为临时测试程序打开 CDP 端口 19348，正式源码/产品没有调试端口后门。go test 与 go build 对 main 包路径不同，测试启动器将真实方法注册为生产前端生成的 ID。

七个场景：当前无记录基线确认并进入工作区、取消退出、备份失败错误页、新版库拒绝、未知结构拒绝、新建库直接进入、同版本直接进入。检查按钮动作、门禁期间无工作区、无前端异常、长路径换行与最小窗口视口布局，截图人工检查现有样式。新库默认关闭到托盘，所以仅新库用正常 runtime Quit 结束测试；其余通过可见退出/关闭按钮。

退出后以独立只读 SQLite 检查：取消、备份失败、新版/未知拒绝场景的原数据库 SHA-256 不变；成功登记 0、1；三分钟手动记录及来源/备注保留；升级前副本无版本表且完整性检查通过；同版本/新库不生成升级备份。临时目录位置见 windows-qa-directory.txt。

复现（在仓库根目录，Go/Node 已加入 PATH）：

```bash
qa_dir=$(python3 scripts/qa/prepare-upgrade-startup-windows.py)
GOOS=windows GOARCH=amd64 go test -overlay "$qa_dir/overlay.json" -tags upgradegui -c -o "$qa_dir/upgrade-ui.exe" ./desktop
node scripts/qa/test-upgrade-startup-windows.mjs "$qa_dir"
python3 scripts/qa/verify-upgrade-startup-windows.py "$qa_dir"
```

准备脚本只新建临时 fixture，不读取/复制/更改日常数据。原生测试仍用第一阶段的 windows-run-tests.ps1 在 Windows 本地启动。测试日志中的预期故障/panic 注入由测试捕获，不代表测试通过之外的恢复承诺。

## 下一阶段

第三阶段进行最终集成验收、维护说明和交付收尾，获用户同意后再开展。当前目标版本只有 1，真实将来 2…N 的 SQL 由维护者维护；新增版本仍需相应结构/数据校验和跨版本测试。跨至 10、逐文件失败续跑等已由核心测试使用合成脚本验证，不表示现在产品已经发行数据库版本 10。

## 第二阶段补充验收

用户随后确认简化文案与纯白背景，并要求补全执行中退出/强杀恢复测试；已完成，最终界面与 14 条 Windows 中断故障场景、两平台重复测试记录见 [补充验收](../database-upgrade-interruption-2026-11-18/README.md)。本文上方截图/文案为最初交付时记录。

## 第三阶段状态

第三阶段已获用户授权并完成，最终结果见 [第三阶段验收](../database-upgrade-stage3-2026-11-18/README.md)。上文等待批准的描述为本阶段结束时的历史状态。
