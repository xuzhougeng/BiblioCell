# 数据库版本与迁移

数据库结构的版本写在 SQLite 的 `PRAGMA user_version` 里，每一步升级在 `schema_migrations` 表留一行（版本号、说明、时间）。当前版本是 `library.SchemaVersion`，由 `internal/library/migrate.go` 里的基线版本加迁移条数算出；`./BiblioCell db version` 可以查看某个库处在哪个版本、还差哪些迁移。

打开数据库（桌面应用启动、任何命令行子命令、`db migrate`）时自动升级，流程固定：

1. 读取 `user_version`。比当前构建还新的库直接拒绝打开，提示更新 BiblioCell。
2. 库落后时，先用 `VACUUM INTO` 把整个文件复制为同目录下的 `bibliocell.sqlite.bak-v<旧版本>`，再动原库。出问题时把备份复制回原文件名即可恢复。
3. 低于基线（v13）的库，比如 v0.1.0 发布版的 v11，先经过 `store.go` 里按实际表结构修复的旧路径（补列、回填 JSON 多值字段、重建仓库名称约束、重建视图），再盖上基线版本号。
4. 基线之后的迁移按编号顺序逐条执行。每条迁移在自己的事务里完成：执行 SQL、执行 Go 函数、写入 `schema_migrations`、更新 `user_version`，然后提交。失败即回滚这一条，库停在上一条完成的版本，错误信息写明失败的迁移、库停在的版本和备份路径；修好后再次打开会从停下的地方继续。
5. 最后执行幂等的 `schema.sql`（新库在这一步一次建成；升级过的库在这里重建迁移中 DROP 掉的视图），并由 Go 盖上当前版本号。

## 新增一条迁移

改表结构时只改两处，再跑测试：

1. 在 `migrate.go` 的 `migrations` 列表末尾追加一条，`Version` 是前一条加一（第一条是 14）。只改表结构写 `SQL`，可以含多条语句；需要用 Go 回填数据写 `Run`，它拿到的是同一个事务。改视图时在迁移里 `DROP VIEW IF EXISTS`，视图由随后的 `schema.sql` 重建。

   ```go
   var migrations = []Migration{
       {Version: 14, Description: "数据集记录下载清单", SQL: `
           ALTER TABLE datasets ADD COLUMN download_json TEXT NOT NULL DEFAULT '[]'
               CHECK (json_valid(download_json) AND json_type(download_json) = 'array');
           DROP VIEW IF EXISTS v_dataset_overview;`},
   }
   ```

2. 把同样的改动写进 `schema.sql`，让新库直接建成最终形态：改 `CREATE TABLE` / `CREATE VIEW`，文件头的 `schema vN` 和末尾的 `PRAGMA user_version = N` 改成新版本号。`schema_migrations` 的初始行不用加，Go 会为新库补齐迁移记录。

`go test ./internal/library` 里的 `TestMigrationsMatchSchema` 用 `testdata/schema_v13.sql` 建一个基线库跑完全部迁移，逐表逐列对比 `schema.sql` 新建的库，两边不一致或版本号没同步都会失败；`TestOpenUpgradesV010Database` 则保证从 v0.1.0 的库一路升上来数据不丢。迁移一旦合入 main 就不要再改，追加新的一条即可。几个分支各加迁移时，合并后按合并顺序重新编号，保证编号连续。

---

[← 返回文档目录](README.md)
