# StudyFlow 的 Goose 适配

来源：github.com/pressly/goose/v3 v3.26.0，提交
`3d1474de3049db2e1ba64a4fe724917292ae7790`。
上游许可证见 LICENSE。根 go.mod 通过 replace 固定使用此处源码。

保留库、库测试、testdata、内部依赖及原 go.mod/go.sum；未复制 CLI、examples、assets、scripts 和外部数据库集成测试 tests。因此不运行依赖 CLI 的上游 TestFullBinary/TestLiteBinary/TestSequential/TestFix。StudyFlow 不提供 Goose CLI。

只有 provider_options.go 和 provider_run.go 有定制修改：

- 增加 WithTransactionRunner：借用 Provider 已持有的连接，由 persistence 的同一事务核心负责 Begin、Commit、Rollback、panic 清理及异常连接隔离。不会再从单连接池申请第二条连接。
- SQL 和版本记录一起传给事务回调；创建版本表和插入 0 也经过该入口。
- 使用此回调时只允许带事务的 SQL，禁止 DDL isolation；事务失败后不自动重试版本表初始化。应用层不启用 DDL isolation、Go upgrades、Down 或关闭版本记录。
- 增加 ValidateSQLMigrations：不操作数据库，提前检查全部选定 SQL 的 Goose 注解。
- initialize 在成功交接连接前的任何退出（包括 panic/Goexit）都释放连接和互斥锁。错误返回不重复释放。原 panic 继续传播。

普通上游用法不设置 TransactionRunner 时，仍使用其原事务入口（initialize 的异常清理修复除外）。这不是上游原生 API，不能删掉 replace 后直接改用原版。

可审查补丁见 ../goose-studyflow.patch；原文件摘要和复制范围见 ../goose-upstream.json。更新上游时，重新获取固定版本、核对 Provider 的全部事务入口与连接清理路径，再重新应用/调整补丁。必须通过 StudyFlow 的升级、事务异常测试以及上游 Provider 回归，不能只让代码编译通过。
