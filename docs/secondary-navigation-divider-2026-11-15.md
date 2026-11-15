# 二级导航分隔线

- 修改文件：`vue/src/shared/SectionSubnav.vue`，设置与学情追踪共用。
- 每两个项目之间使用独立的 Vuetify `VDivider`，移除仅相邻未选中项才生成分隔线的伪元素规则；沿用主题边框色。
- 按用户要求保持版本 `0.4.0-76`，直接执行平台构建任务，未执行递增版本的 release prepare。
- WSL Debian：`wails3 task windows:build ARCH=amd64` 成功，包含 vue-tsc、Vite 生产构建和 Windows amd64 Go 构建；`git diff --check` 通过。
- Windows 实际 Wails 窗口：打开设置和学情追踪，检查导航分隔线，切换阶段目标/进度追因，拖动二级导航至收窄和展开状态；页面切换与调整宽度可用。关闭行为测试后恢复原“最小化”设置，导航恢复 240px。
- 桌面现有 76 便携目录中的 exe 和便携 zip 已更新，数据保留。未生成安装包，未运行 Linux GUI。
- exe SHA256：`70badc8d58c96b8b4bd03ccb639803cdd4d3707a15dcc2f202253e6170de6e57`。
