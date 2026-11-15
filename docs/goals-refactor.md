# 阶段目标模块的职责与验证

## 修改范围

本次整理阶段目标模块、首页近期目标、公共操作按钮，以及此次修改涉及的检查时间样式和构建版本流程。保留用户指定的卡片结构、背景色、禁用 Tab 移动焦点和设置页驻留托盘图标。

## 后端职责

- `desktop/goals.go` 是桌面请求适配层，只处理路径、请求解析、依赖组装和服务转发，符合 AGENT.md 第 16.2 节。
- `internal/modules/goals/domain.go` 定义领域对象；`dto.go` 定义输入、输出契约和转换。
- `service.go` 处理目标校验和完成状态，不依赖 SQL。
- `repository.go` 定义服务需要的数据访问接口；`sqlite_repository.go` 是 SQLite 实现。
- Repository 使用适配层提供的有效句柄，不创建连接、不关闭连接、不重复借用生命周期锁。内容更新与完整状态回读放在同一事务中。
- 普通删除保持逻辑删除；原有表名、字段、API 路径及 JSON 契约保持兼容。

## 前端职责

- 目标专属的类型、API、组件和样式位于 `features/goals/`；状态及公开操作由 `stores/goals.ts` 持有。
- `shared/ListActionButton.vue` 供目标、计划、项目共用，统一图标、文字、尺寸、圆角及删除语义色。
- `shared/designTokens.css` 管理复用的间距、字号、圆角、尺寸和阴影；颜色复用现有主题和表面变量。
- 首页目标样式归入 `features/today/todayGoals.css`，检查时间样式归入 `features/review/reviewTimeSelect.css`，移除此前新增的压缩全局样式。
- 卡片优先使用 Vuetify `VCard`，保留内容语义和独立操作；单向排列使用 Flex，统计卡的列布局使用 Grid 和容器查询。切换列表不分配额外高度给统计卡。
- 表单复用计划表单的布局约定；日期使用现有的 `VMenu` 和 `VDatePicker`；检查时间输入保留原来要求的白色表面。

## 构建流程

`wails3 task build` 和 `wails3 task package` 在构建前执行 `tools/release prepare`，只递增 D 段，不改变 A.B.C。

版本来源为 `vue/package.json`，同时更新锁文件、Wails 配置、Linux 包版本和 Windows 版本信息。成功构建后输出带版本、平台、架构的 Linux 文件；保留 `bin/StudyFlow` 运行入口。生成新安装包后清理同平台、同架构的旧版本，保留其他平台和架构的产物。

## 验证记录

- `go test ./internal/... ./desktop ./tools/...` 通过。
- `go vet ./internal/modules/goals ./tools/release` 通过。
- 前端类型检查、构建和 Linux 可执行文件、AppImage、DEB、RPM、Arch 安装包构建通过。
- 目标测试覆盖输入校验、编辑保留完成状态、完成与重新开启、逻辑删除、API 契约。
- 数据库启动与历史迁移测试保留原有断言，并增加新目标表的断言。
- 构建工具测试覆盖版本同步、连续构建递增，以及旧产物清理时保留其他平台、架构和数据库文件。
- 已做部分实际桌面操作检查；按用户后续要求停止自动 UI 校验，最终界面效果由用户确认。不将构建通过等同于 UI 验收通过。

## 布局依据

本次查阅 Modern Web Guidance 的 `card` 和 `css-layout`：依据 “keep links and buttons for independent actions as separate native controls” 保留独立操作按钮；依据 “Use flexbox — 1D, content-first” 调整纵向排列，避免空余高度拉伸统计卡。
