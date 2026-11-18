# 阶段目标验收与排序

原因：`GoalsPanel.vue` 的验收成功回调强制将 `showAccepted` 设为 true，切换到另一组目标。排序菜单仅在进行中显示，而已验收分支直接返回截止日期排序的目标，忽略了用户选择的新建优先。因此验收后同时出现列表突变、排序变化和排序菜单消失。

修复：验收成功只关闭确认框，保留当前列表与排序；已验收列表同样展示排序菜单，并应用当前排序。进行中列表仅移除刚验收的目标，其余目标相对顺序不变。

验证：

- `node scripts/qa/test-goal-acceptance.cjs`：实际 Vue 组件 setup 的回归测试，覆盖两种排序、完成/未完成、API 失败、最后一个目标，以及切换已验收列表后的排序。
- `wails3 task windows:build ARCH=amd64`：Vue 类型检查、前端生产构建和 Windows 交叉编译通过，版本保持 0.5.0-5。
- `node scripts/qa/test-goal-sort-windows.mjs`：Windows 实际 Wails WebView 中执行四组验收流程，验证验收保留当前列表与排序；切到已验收后菜单仍显示，且两种排序可切换。结果见 windows-results.json，截图见 accepted-sort.png。
- Windows 验证沿用固定便携目录及其数据库。仅在内存中替换目标和验收 action，测试后恢复；没有调用目标写入 API，没有追加或删除数据库记录。数据库完整性和外键检查通过。

Windows 调试启动器通过临时 Go overlay 开启本地调试端口，正式构建不含调试设置。

交付：旧实例在退出请求后残留，经用户明确允许后结束残留进程，再替换固定目录中的 StudyFlow.exe。替换前已将数据库备份至固定目录 qa-backups/before-goal-sort-fix-20261118-100000.db。交付程序与 bin/StudyFlow.exe 的 SHA256 一致：9bac0578675f11b7d03191f3ff489ab6198da5a5df5b8b944b08c7fbeacb51fa。版本保持 0.5.0-5。
