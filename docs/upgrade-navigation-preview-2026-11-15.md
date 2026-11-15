# 升级核查、导航标题图标和展示数据

## 修改范围

- `vue/src/shared/SectionSubnav.vue`：增加标题图标参数，灰色图标与文字共用标题颜色；文字宽度、图标间距、起始位置使用现有导航宽度变量。72px 时仅显示图标，提示保留完整名称。
- `vue/src/features/settings/SettingsPage.vue`：传入一级设置导航相同的 `mdi-cog-outline`。
- `vue/src/features/review/ReviewPage.vue`：传入一级学情追踪导航相同的 `mdi-chart-timeline-variant`。
- `internal/infrastructure/persistence/startup_goal_upgrade_test.go`：补测旧八表库新增阶段目标表、旧目标表补验收字段，并校验原八表的业务字段及关联保留。
- `scripts/qa/seed_preview_data.py`：离线追加关联展示数据，保留原记录，使用事务及外键/完整性检查；拒绝重复追加展示项目或覆盖既有历史总结。每日快照保留原冻结项目并合并新增项目。
- 未修改生产数据库启动/迁移逻辑。版本保持 `0.4.0-76`。

## 数据库升级结论

支持识别的历史五表结构会补建新表、补列；已有完整八表但无阶段目标表的库会补建阶段目标表；旧目标表会增加 `accepted_at`。正常升级保留已有业务记录。

并非所有历史 SQLite 文件都可无条件升级：未知或不完整结构拒绝打开。迁移包含已有语义更新：旧默认配色 `#7668e8` 会改用新配色；旧已完成目标补 `accepted_at=completed_at`。因此不能描述为所有字段原封不动。

现有初始化使用逐条 SQL，并非包住全部迁移的单个事务；迁移中途失败会拒绝启动并报告可能部分更新，不能保证这种失败场景数据库完全未变。该行为由既有 `TestOpenForStartupReportsPossiblePartialMigration` 覆盖，本轮未改变。

## 实测

| 环境 | 检查 | 结果 |
| --- | --- | --- |
| WSL Debian | `go test -count=1 ./internal/infrastructure/persistence -run "TestStartupAddsGoal\|TestOpenForStartup"` | 通过：三种历史结构、现代结构、异常拒绝、目标表/字段升级和数据保留 |
| Windows 本机 | WSL 交叉编译的 persistence test.exe，运行同一组测试 | 全部通过，含新增两个升级场景 |
| WSL Debian | `wails3 task windows:build ARCH=amd64` | 通过，包含 vue-tsc、Vite 生产构建与 Windows Go 构建 |
| Windows Wails 实际窗口 | 设置/学情追踪标题图标、展开及72px收窄 | 通过：灰色图标，最小宽度只留图标，展开恢复文字 |
| Windows Wails 实际窗口 | 今日计划、目标列表、进度追因总结 | 可读取关联数据：今日7计划/7记录、完成度81%、目标5进行中/2已验收、7份历史总结 |
| WSL Python SQLite，应用完全退出后 | 测试库写入事务、`integrity_check`、`foreign_key_check` | 通过，完整性ok，无失效外键 |
| WSL Debian | `git diff --check` | 通过 |

未运行 Linux GUI；新增升级测试已在 Linux/Windows 两平台执行。

## 展示数据

目标数据库：`D:\CLink\Desktop\studyflow\StudyFlow-0.4.0-76-portable\data\study.db`。

| 表 | 写入后行数 |
| --- | ---: |
| subjects | 7 |
| plans | 7 |
| sessions | 56 |
| activity_logs | 8 |
| settings | 11 |
| daily_reviews | 7 |
| daily_review_items | 49 |
| daily_plan_targets | 8 |
| stage_goals | 7 |

七项目分别关联一条每日计划；11月8日至15日每天七条已结束学习记录。前七天每日快照与总结逐项目对应，总结实际时长等于当天七条学习记录时长之和。今日计划35分钟含5分钟休息，每项目计划学习30分钟。阶段目标覆盖未来截止、验收成功和验收未完成。

写入前备份位于 WSL `/tmp/studyflow-before-preview-20261115.db`。备份在临时目录，不作为长期归档承诺。

桌面现有便携 exe 和纯程序 zip 已更新；展示数据只在便携目录中，zip 不携带数据。关闭按钮恢复原最小化行为，二级导航展开至240px。exe SHA256：`bd36c1d384d75f01020de7cb861121006e8733ce9df8ef874fe13ed93b8985e0`。
