# Goose 接入：第三阶段最终验收

2026-11-18。第三阶段已完成：生产入口收口、结构识别补强、维护说明、两平台回归、固定目录真实数据升级和最终程序交付。仍为应用 0.5.0-5、目标数据库版本 1，没有发版或新建日常便携目录。

## 最终改动

- 将桌面旧构造器移至 `desktop/legacy_fixture_test.go`，只供测试，生产启动统一经过版本检查与确认门禁。
- 修复历史结构识别遗漏：仅检查 PRAGMA 无法发现 ON CONFLICT REPLACE/IGNORE 等写入策略。现在完整基线和明确历史结构均拒绝未知约束/冲突策略；补充 CHECK、AUTOINCREMENT 等拒绝用例，确认前阻止且原库不变、不生成备份。
- 新增 `docs/database-upgrade-maintenance.md`，说明目录、SQL/目标版本维护、旧库永久接管、逐文件事务、恢复边界及 Goose 本地补丁更新要求。
- 新增 Goose fork 摘要检查和固定目录真实主程序 UI/数据验收脚本。生产未开启 CDP 调试端口，仍为既有单连接池及共用事务核心。

## 验收结果

| 检查 | 结果与证据 |
| --- | --- |
| WSL 全仓测试 | 通过，linux-regression.txt |
| desktop / persistence / upgrade race 与 vet | 通过，linux-race.txt、linux-vet.txt |
| Windows 原生全仓 16 个测试包 | 14 包通过；另外两包各一个既有失败，windows-full-summary.json 及 exam_notify_*-native-stdout.txt |
| Goose 上游 Provider 回归与本地 fork 摘要 | 通过，goose-provider-regression.txt、goose-fork-integrity.json |
| Vue 类型检查、Vite、非增版 Windows 构建 | 通过，windows-build.txt |
| 固定目录真实主程序：取消、确认升级、同版本再次启动、学情排序控件 | 通过，fixed-*-results.json、对应截图 |
| 固定目录真实主程序单实例 | 首实例正常运行，重复启动进程自行结束，未开启第二个业务实例 |
| 应用自身 runtime Quit，两次退出 | 正常结束，无残留，fixed-runtime-quit*.txt |
| 实际日常库九张表逐行对比、用户文件、升级备份 | 全部通过，fixed-data-verification.json |
| 最终非调试 StudyFlow.exe | 已替换并正常进入今日计划，final-launch.json、final-product-window.png、deployment.json |

Windows 两项既有失败仍是：`TestAPILogsKeepBindingContract` 的日志文件列表为空；`TestWindowsAudioWorkerStopsAndCanBeRecreated` 的 Windows MCI 音频状态查询返回无效输出。没有将 Windows 全仓记为全部通过。

前两阶段与补充验收的 SQL/提交错误、panic、取消、I/O/内存错误注入、真实磁盘满、DELETE/WAL 下执行中关闭/退出/强杀/重复强杀/提交后强杀均保留，详见相邻阶段记录。没有新增与这些记录重复的硬件恢复承诺；物理断电、硬件损坏、整机内存耗尽仍无法保证 100% 恢复。

## 固定目录数据与交付

实际路径：`D:\CLink\Desktop\studyflow\StudyFlow-0.5.0-5-portable`。

先离线验证原库完整性和外键，并在 `qa-backups/before-goose-stage3-20261118-212150.db` 保留额外快照；九表逐行预期只放本机 qa-backups，不将原始学习内容写入验收文档。

实际点击“暂时退出”后，数据库摘要不变且没有升级备份；重新确认后，自动备份验证成功，登记 Goose 0、1，再进入工作区。原始记录共保留：活动日志 14、每日目标 14、复盘项 84、每日复盘 14、计划 7、学习记录 87、设置 3、阶段目标 9、科目 8。所有九表逐行一致、音频摘要一致。升级前自动快照保留于 `data/backups/upgrade-1-to-1-20261118-212609-1795008369353224300.db`，无 Goose 版本表且数据完整。

重复启动没有再次升级或新增升级备份。最终替换前程序已正常退出；旧可执行文件保留于 `qa-backups/StudyFlow-before-goose-stage3.exe`，只用于保留交付前文件，不代表允许用旧程序打开新版数据库。最终 `StudyFlow.exe` 与 `bin/StudyFlow.exe` SHA-256 相同。最终程序已启动并留给用户查看。Windows 进程查询的 ProductVersion 返回空字符串，因此版本依据仓库构建配置和前端 package.json 均为 0.5.0-5，没有把该空值算作版本资源验证通过。

## UI 与退出测试说明

UI 测试前已通知用户。computer-use 在 WSL 初始化时报 sandboxCwd URI 错误，改用真实 Windows Wails WebView2 的 CDP；固定目录验收使用生产 main、真实单实例守卫，仅编译器 overlay 添加临时调试端口。最终交付使用正式无调试端口产物，额外通过 Windows 原生窗口截图核对启动；未用网页模拟桌面。最终截图窗口部分超出屏幕，只作为已进入工作区的证据，完整 UI 检查见 fixed-*-workspace.png。

最初自行编写的退出探针错误地向 Wails 隐藏调度窗口发送 WM_CLOSE，提前破坏退出调度，造成探针超时。该无效脚本已删除，旧实例及自建 QA 残留依照用户此前明确授权清理；日志 old-app-exit.txt/new-app-exit.txt 属于无效探针诊断，不算应用退出失败或正常退出通过的证据。改用应用 runtime Quit 后两次正常退出，以上通过结果以修正后的记录为准。

## 后续维护限制

当前产品仅发行数据库目标 1。未来增加 2…N 时，维护者提供真实 SQL，同时更新目标结构校验和跨版本测试；合成跨至 10 测试不表示产品已经具备版本 10 业务结构。已经发布且没有版本门禁的第一版旧程序无法被本次代码追溯修复，不应再用它访问后续新版数据库。导入导出和一键恢复界面不属于本次改造。
