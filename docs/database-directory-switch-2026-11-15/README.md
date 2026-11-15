# 数据目录迁移修改与验证记录

## 范围与约束

- 仅维护桌面 Linux（不含 Android）与 Windows 的平台实现。
- 在 WSL Debian 开发、编译、打包；Windows 仅执行测试产物。
- 保留现有前端接口、数据库外键修复及安装包 73 的卸载修复。
- 平台文件负责操作系统差异；快照、校验、生命周期锁及恢复协议中相同的规则共用，避免复制两套基础设施。
- 确认提交后才清理旧数据库与实际迁移的备份；未知文件保留。提交前失败保留旧库；不确定时暂停访问。

## 阶段计划

1. 基线检查、明确平台边界，去除当前目录切换中的 legacy 命名。
2. Windows 接入快照、串行切换、位置记录发布、回退与启动恢复。
3. Windows 文件身份与发布检查。
4. 两端增加提交后的安全清理，并覆盖中断恢复与返回原目录。
5. 针对改动运行 Linux 无界面测试、Windows 原生测试及便携版桌面验证，最后打包。

## 实现与影响

### 平台边界

只保留 Linux（不含 Android）与 Windows 的目录切换实现，没有为其他平台添加兼容分支。这里的“两套平台文件”指每项操作系统差异各有 `_linux.go`、`_windows.go` 实现，不是整个迁移链路只有两个文件。

公共状态机负责准备、候选库验证、提交结果分类、暂停访问及清理顺序。系统调用、文件身份、位置文件发布由平台文件实现。数据库初始化、升级、外键配置、快照和自动备份规则继续共用，前端 `APIService.SetDataDirectory` 调用方式不变。没有修改数据库表结构。

### 逐项修改

| 文件或模块 | 修改内容 | Linux 影响 |
| --- | --- | --- |
| desktop/data_directory.go、data_directory_linux.go、data_directory_windows.go | 公共绑定保持薄入口；Linux 仅负责 APPIMAGE 安装位置解析，Windows 使用 exe 位置。删除旧 data_directory_legacy.go 的 SQL/文件迁移实现与 legacy 函数名 | 原入口逻辑提取，安装位置语义保留 |
| persistence/directory_switch_flow.go、directory_switch_linux.go、directory_switch_windows.go | 将原 Linux 切换编排提取为共用状态机；两平台入口调用；Windows 整段切换使用生命周期写锁，位置记录不再提前发布 | 共享代码提取与后述清理行为变化 |
| storage/location_switch.go、location_switch_linux.go、location_switch_windows.go | 共用恢复记录格式及状态分类，系统操作分别实现；Windows 使用 CreateFile 句柄身份、卷号/文件 ID、拒绝 reparse 文件、独占保留名称及 MoveFileEx 直写发布 | 原 Linux 协议提取；Linux 的 openat/O_NOFOLLOW/Renameat2、目录同步、身份判定继续使用 |
| storage/startup_location.go、desktop/startup_database.go | 两端都支持 pending 启动恢复；先校验选中新库，再处理清理记录。仅旧文件清理失败时记录告警并允许已验证的新库启动；位置不确定或数据库验证失败继续拒绝 | 新增“提交后旧文件清理失败”的处理，不放宽原来的位置或数据库验证 |
| persistence/directory_prepare.go | 记录本次成功复制的源备份清单；Windows 复用 SQLite 快照，旧连接保持打开直至确认提交 | 快照规则不变，增加清单 |
| persistence/directory_resources.go | 从打开的文件句柄捕获源数据库身份，避免 Windows 路径型 FileInfo 延迟读取 ID | 多一次只读身份核验 |
| persistence/directory_publish_windows.go | 复核准备目录；文件刷新与直写发布配合位置恢复协议，明确与 Linux 目录 fsync 的差别 | 无平台运行影响 |
| storage/location_retirement.go | 持久化源目录、源库、实际迁移备份身份及 SHA256；删除前复核目标备份身份和内容。提交且旧连接关闭后才逐文件隔离、复核、删除；只删空备份目录，不递归删除用户目录；支持中断后重试 | 成功迁移后清理旧库和已迁移备份，这是用户明确要求的行为变化 |
| 旧 backup_legacy.go、directory_switch_unsupported.go、directory_publish_other.go、startup_location_unsupported.go | 旧复制链路由共用快照/安全复制取代，删除不再支持的平台回退实现 | 不改变 Linux 平台选择 |
| 测试文件 | 原 Linux 切换、位置协议、启动恢复测试移为两平台共用；保留原测试意图，仅更新成功后旧文件删除的预期。ACL 测试改为验证准备阶段失败后原句柄不变、目标文件无残留、外键仍开启；新增退役清理、崩溃恢复、返回原目录及双切换并发测试 | 针对重构与新增行为回归 |
| build/config.yml、Windows info.json / project.nsi、vue/package*.json | 依仓库打包流程更新构建号至 76；未修改安装、卸载逻辑。build/linux/nfpm/nfpm.yaml 已恢复成本轮开始时的字节 | 公用版本标签变化；Linux 打包配置不新增变化 |

真正表示历史数据库迁移的 `legacy_snapshot.go`、`SnapshotLegacyDatabase` 等名称保留。这些不是当前 Windows 目录切换路径。

### 失败和恢复行为

- 准备或校验失败：旧连接继续可用，仅回收本次可确认归属的候选资源。
- 位置发布未生效：关闭候选连接，清除已验证的记录并回收候选资源。
- 无法确认发布结果：暂停数据库访问，保留两端数据和恢复记录；启动时重新判定，不能擅自选库。
- 提交成功：激活新连接，关闭旧连接后再清理旧库和已迁移备份。清理失败只产生告警，不回退新库；保留 pending，阻止叠加下一次迁移，重启时重试。
- 文件被替换、备份内容改变、旧 SQLite sidecar 仍存在：保留相关文件，停止清理。不会自行删除 WAL/SHM/journal。
- 历史版本的 pending 记录没有退役清单，继续按旧规则保留旧文件，避免凭文件名推断删除权限。

## 已执行验证

| 阶段 | 环境 | 命令或场景 | 结果 |
| --- | --- | --- | --- |
| 修改前基线 | WSL Debian，源码位于 /home/jhsong/programming/code/StudyFlow | go test ./internal/infrastructure/persistence ./internal/infrastructure/storage ./desktop | 通过（前两包命中缓存） |
| 重命名阶段 | WSL Debian + Windows 11 amd64 原生 test.exe | Linux 三包非缓存测试；Windows 原绑定、备份迁移、外键、ACL 失败与并发备份测试 | 通过 |
| 最终 Linux 回归 | WSL Debian，Go 1.27.3，TMPDIR=/home/jhsong/.cache/studyflow-directory-qa（Linux 本地文件系统） | go test -race -count=1 ./internal/infrastructure/storage ./internal/infrastructure/persistence ./desktop | 三包通过，未报告竞态，见 linux-race.txt |
| Linux 构建与平台选择 | WSL Debian | go build -o /home/jhsong/.cache/studyflow-directory-qa/StudyFlow-linux ./desktop；两平台 go list GoFiles | 通过，见 linux-build.txt 与 *-platform-files.txt |
| 最终 Windows 文件协议 | Windows 11 Pro 10.0.26300 amd64，本地 NTFS 临时目录；test.exe 在 WSL 交叉编译 | 缺失/空/已有位置文件；pending 写入/同步/关闭/发布失败；身份与内容替换；恢复记录损坏；清理失败；子进程在提交各阶段及退役隔离/删除阶段退出后恢复 | 通过，见 windows-storage.txt；一个依赖符号链接权限的用例跳过 |
| 最终 Windows 数据库基础设施 | 同上 | persistence 整包：WAL 快照、备份冲突、准备失败回收、外键、候选校验、提交告警、暂停、生命周期锁、A→B→A、双切换并发写入、清理完成但 pending 未清除的恢复 | 通过，见 windows-persistence.txt；FIFO、符号链接及 Windows 末尾空格不适用用例跳过 |
| 最终 Windows 绑定与启动恢复 | 同上 | TestWindowsBinding、TestChangingDataDirectory、TestWindowsQA、TestStartupPending、TestStartupCommitted | 通过，见 windows-desktop.txt；含真实 ACL 拒绝、API/备份并发、启动失败保留证据、新库可用但旧文件清理失败 |
| NTFS 修复稳定性 | Windows 11，本地 NTFS | 返回原目录与双切换并发两个测试各 -test.count=20 | 均通过；执行于 75 代码阶段，最终完整套件另已复验 |
| Windows 打包 | WSL Debian，项目既有 Wails/NSIS 工具 | wails3 task package GOOS=windows ARCH=amd64 | 构建 74、75、76 均成功，最终交付 76，见 package-76.txt |
| 最终 76 便携版实际界面 | Windows 11，D: 本地 NTFS，隔离合成数据 | 设置页确认 v0.4.0-76；A→中文/空格/#/% 的 B→正常退出→重启确认 B→迁回 A；迁回后界面读取测试项目 | 通过，见 gui76-observations.txt |
| 最终 76 便携版文件核验 | Windows GUI 实际迁移后，在 WSL 只读检查生成文件 | 两次迁移后的主库和 SQLite 备份 integrity_check=ok、测试项目存在；旧库及迁移备份删除、未知用户文件保留、无 pending 残留 | 通过，见 gui76-files-a-to-b.txt、gui76-files-b-to-a.txt |

## 测试过程中的发现

第一次 Windows 全面复验发现：单次迁移成功，但第二次切换的 pending 发布身份确认失败。原因是 NTFS 文件隧道会在重用近期删除的名称时保留创建时间，导致重命名前后创建时间不稳定。Windows 改用卷号与完整文件 ID；Linux 的身份判定没有随此修复改变。保留 windows-persistence-before-ntfs-fix.txt 作为历史失败证据，最终结果以 windows-persistence.txt 为准。

75 完成了便携版 A→B→重启→B→A 的实际界面测试，检查了中文、空格、#、% 路径、项目读取及数据库/备份 integrity_check。最终检查又加入了删除源备份前复核目标备份的保护，因此重新打包为 76，并再次执行最终原生测试及便携版验证。74、75 是中间测试构建。

## 验证边界

- WSL 无 GUI，本轮没有重复 Linux 桌面显示、声音或完整产品验收。Linux 改动及重构影响已通过无界面测试、竞态检查与桌面程序编译验证；未打 Linux 安装包。
- Windows 测试为本机 NTFS，未覆盖网络共享、其他文件系统、真实断电或磁盘控制器故障。进程退出模拟不等于断电验证；Windows 直写发布不宣称等同 Linux 的目录 fsync。
- Windows 没有为测试开启符号链接权限、安装开发工具或改变执行策略；junction 路径用例实际通过。
- 创建 pending 之前进程退出可能留下已准备的候选文件；恢复不会凭猜测删除这些文件。旧位置仍为活动位置，目标出现 study.db 时仍按保护规则拒绝覆盖。
- 恢复记录或文件归属无法确认时保留现场；外部永久替换文件、残留旧 WAL 等情况不会强制删除。新库已确认可用时继续使用新库，但 pending 未处理完之前不能继续迁移。
- 此次只运行相关 Windows 桌面测试子集，没有声称整套 Windows 产品测试全绿；早前日志/音频测试问题不在本次修复范围。
- 安装包已构建，本次未重新执行安装、升级和卸载场景；上一轮 73 的安装器修复保留。

## 交付与用户数据

最终交付位于 Windows 桌面 studyflow：

- StudyFlow-0.4.0-76-portable/StudyFlow.exe
- StudyFlow-0.4.0-76-windows-amd64-portable.zip
- StudyFlow-0.4.0-76-amd64-installer.exe

干净便携目录只有 exe，不带测试数据库。它与实际 GUI 测试使用的 exe SHA256 相同；完整哈希见 release-sha256.json。旧安装包及便携压缩包已按仓库规范清理，旧便携目录保留。

隔离测试目录仍名为 D:\CLink\Desktop\studyflow\directory-switch-qa-74，因为它创建于第一轮测试；其中最终测试 exe 已更新为 76，测试项目名 QA-directory-74 只是固定合成标记，不代表运行版本。记录明确区分中间构建与最终构建。

两轮 GUI 测试前正常退出原版 71，退出后恢复 closeBehavior 的原始状态。最终关闭测试版后，原库完整 SQL dump 与测试前快照一致（original-data-preserved.txt），原版 71 已重新启动。没有覆盖用户原数据库，没有安装新版本或开发工具。所有代码保持未提交，供用户检查确认。
