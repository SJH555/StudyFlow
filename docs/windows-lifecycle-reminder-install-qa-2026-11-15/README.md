# Windows 生命周期、提醒、安装升级测试（2026-11-15）

测试目标：已打包的 StudyFlow 0.4.0-72。开发和测试程序编译均在 WSL Debian；Windows 只运行应用、安装包和编译后的测试程序，未安装开发工具。未修改产品代码或安装脚本，未修改系统时间，未进行文件命名重构。

## 环境与隔离

- Windows 11 Pro，10.0.26300，amd64，Asia/Shanghai。
- 正式 72 便携版及安装包来自上一轮 WSL 构建；测试用便携版另复制到桌面 studyflow/lifecycle-reminder-install-qa/portable。
- 71 首次安装测试包在 WSL 用既有 71 可执行文件和当前 NSIS 模板生成，仅在临时脚本中将版本回填为 71；不是重新构建旧版本源码。72 升级使用桌面正式安装包。
- 安装目录为同一 QA 目录下 installed。测试开始前未检测到 StudyFlow 卸载注册信息。用户随后授权由助手操作安装测试。
- 真实便携版 71 数据库使用 SQLite backup 保存；临时修改的 closeBehavior 恢复后，完整 SQL dump 与快照一致。卸载前还备份了整个隔离安装目录和共用的 %APPDATA%/StudyFlow.exe WebView 目录。
- 证据只归档合成数据和测试日志，不将真实数据库或 WebView 备份提交到仓库。

## 测试矩阵

| 项目 | 结果与验证方式 |
| --- | --- |
| 正常启动与退出 | 72 便携版多次启动和 Alt+F4 正常退出；升级后安装版正常启动和退出；有待填总结时按设计拦截退出，提交后可退出。 |
| 后台任务排空 | Windows 原生 desktop 测试验证在途 API/回调排空、延迟任务取消、停止接纳新任务、重复退出竞争、Run 返回与第二次 Quit 的四种先后关系；不是仅 mock 断言调用次数。原生 Wails UI 正常退出另外实测。 |
| 数据库/日志/音频释放 | 正常退出及响铃中退出后进程结束；SQLite、日志、三种 MP3 及 exe 均能以 FileShare.None 独占打开。音频原生测试另验证三种音源播放、停止、Shutdown 等待线程结束、重复 Shutdown 和重新 Init/播放。 |
| 单实例及重启 | 第二次启动立刻退出，原进程 PID 保持 24524，最小化窗口自动恢复；正常退出后多次重新启动成功。 |
| 计划开始/结束/衔接 | GUI 在 19:37 同时结束 A、开始 B，收到合并弹窗；不足 10 分钟不计记录符合规则。原生测试另验证完整 30 分钟计入 1800 秒、当前计划兜底、已启动计划不重复提示，以及结束失败不屏蔽下一计划。 |
| 活动提醒 | 整分钟弹出；GUI 提交合成记录后数据库出现对应记录；关闭提醒、下一周期再次提醒及最小化自动恢复实测。 |
| 休息提醒 | 剩余休息到期弹出“休息时间结束”；点击继续学习恢复计时，数据库通知标记及一次性触发由原生测试覆盖。 |
| 每日总结 | 补填前日总结、关闭拦截、填写和提交实测；数据库核对 day=2026-11-14、planned=3600、actual=0、reason 正确；优先级/排队/释放由原生测试覆盖。另补测设定检查时刻的最小化窗口唤醒，见补充记录。 |
| 每日快照 | Windows 原生测试传入 2026-11-15 00:00，验证关闭时不写、开启时写 11-14、重复调用和计划修改后不覆盖旧快照。实际 GUI 总结也生成昨日快照。未等待真实午夜，未把该项描述为午夜整链路 GUI 实测。 |
| 声音/Wails 事件/窗口唤醒 | 计划、活动、休息均出现真实 Vue 弹窗，验证 Wails 投递与前端处理；活动最小化恢复实测。三种音频 MCI 播放成功且可释放；未通过录音或人工听音确认物理扬声器输出。 |
| 首次安装 | 71 隔离安装成功，exe 哈希与原 71 一致，开始菜单和桌面快捷方式、卸载程序及 HKCU 卸载信息存在。 |
| 升级安装 | 手动选原目录进行 71→72，安装成功，exe 哈希等于正式 72，卸载注册版本更新为 0.4.0-72。 |
| 升级用户数据 | 数据库、备份标记、用户文件升级前后 SHA256 完全一致；升级安装版 GUI 能看到 QA-upgrade-preserve 项目。 |
| 卸载数据保留 | **失败**：72 正式卸载程序 /S 实测将整个安装目录删除，内含默认数据库、备份和用户文件；外部数据库、真实 SQLite 备份和用户文件哈希不变。注册表和快捷方式正确清理。 |

## 确认的产品问题

### 1. 卸载删除默认数据（高优先级）

build/windows/nsis/project.nsi:112 使用 RMDir /r $INSTDIR，而 Windows storage.BaseDir 使用安装目录，默认 data 位于其中。实测 installed/data/study.db、data/backups、用户文件全部被删。不是数据库损坏或打开失败，而是卸载的明确删除行为。

同文件第 110 行还删除 %APPDATA%/StudyFlow.exe，实测确被删除。这是不同目录便携版/安装版共用的 WebView 目录，因此并非仅清理当前安装的缓存。测试前已备份，测试后恢复。

### 2. 升级不记住自定义安装目录（中优先级）

首次安装到 QA 自定义目录后，运行正式 72 升级包，目录页仍默认 C:/Users/jhsong/AppData/Local/Programs/StudyFlow。脚本只有静态 InstallDir，没有读取旧 UninstallString/安装位置的逻辑。

手动选原目录可成功保留数据升级；若按默认目录直接继续，会另建安装位置，并可能使用新的默认数据库，造成“数据不见了”的体验，同时改写同一组快捷方式和卸载注册信息。本次验证了错误的默认路径，未向默认目录实际安装第二份应用。

## 测试本身的问题及边界

- reminder.txt 原始测试有一项失败：TestWindowsAudioWorkerStopsAndCanBeRecreated 调用 sysinfo all quantity，此命令需要字符串输出缓冲区，而 mciCommand 设计为只执行命令、不返回文本，所以 Windows 报缓冲区太小。此失败发生在测试进入 Shutdown 之前，不能证明产品无法退出。没有修改该既有测试或产品音频代码。
- 新增 Windows 专用 qa_windows_test.go 改用真实三种音源，验证播放/停止/响铃中 Shutdown/线程退出/重复退出/重建，实际全部通过。既有播放后确认关闭设备测试也通过。
- desktop 原有一项目录切换后的提醒确认测试只支持 Linux，Windows 跳过；不属于本轮生命周期/提醒测试的通过项。
- app 原有 Unix chmod 模拟清理失败的测试在 Windows 跳过；未用跳过充当通过。
- 午夜宿主轮询本身没有真实午夜 GUI 长时间等待覆盖。未覆盖休眠、机器重启、无 WebView2 环境、管理员范围安装及所有历史版本升级；这些不是本次已有环境中的实测结论。
- 后台排空采用 Windows 原生应用组件测试制造真实在途 API 和回调；没有通过 GUI 刻意延长生产 API 来观察排空。

## 关键代码核对

- desktop/lifecycle.go、desktop/main.go：取消生产者、排空任务后第二次退出；外层清理顺序包括单实例监听、窗口状态、音频、Transport、数据库、日志及实例句柄。
- desktop/main.go、desktop/break.go、desktop/daily_review.go：每秒驱动计划/休息，整分钟检查活动和总结，午夜调用昨日快照，统一播放和事件分发、唤醒窗口。
- vue/src/features/useLifecycle.ts：plan-reminder、activity-reminder、break-reminder、daily-review-due 的真实事件接收及弹窗联动。
- internal/infrastructure/reminder/ringtone_windows.go：独立锁定 OS 线程处理 MCI；Shutdown 等待 worker 完成。
- internal/app 与 internal/modules：计划边界、休息通知、每日总结、快照和提醒优先级。
- build/windows/nsis/project.nsi、wails_tools.nsh、internal/infrastructure/storage：安装覆盖与删除范围、注册信息及 Windows 数据路径。

完整可执行测试程序、脚本、合成数据及安全副本位于桌面 studyflow/lifecycle-reminder-install-qa；仓库 evidence 为精简日志归档。

## 定时总结与恢复补充记录

最后一轮检查时间设为 19:55，19:53 在真实 GUI 确认尚无待填总结弹窗后最小化。期间没有重新激活 StudyFlow；19:55:13 直接获取窗口状态，窗口已自动恢复并出现 11 月 14 日总结弹窗。这覆盖了宿主定时检查、daily-review-due 事件、Vue 弹窗和窗口唤醒的实际链路。见 gui-before-review-tick.txt、daily-review-due.json、gui-review-tick-wake.txt。

测试安装已卸载，默认数据删除的合成样本有完整安全副本；共用 WebView 目录已从备份恢复。测试末尾重新打开原 71 便携版，真实数据库的 SQL dump 核对结果另见 final-preservation.txt。
