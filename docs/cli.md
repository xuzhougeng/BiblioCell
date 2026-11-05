# 命令行

同一个可执行文件也可以当命令行工具用，不开窗口，直接读写正式 SQLite 数据库和 `settings.json`。第一个参数是已知子命令时走命令行，否则照常启动桌面应用。输出一律是 JSON，方便接 `jq`。

```bash
./BiblioCell status
./BiblioCell papers list -q "Arabidopsis" -limit 20
./BiblioCell papers get P-xxxx
echo '{"title":"...","doi":"10.1000/x"}' | ./BiblioCell papers add
./BiblioCell papers import -f papers.json -dry-run
echo '{"journal":"Cell"}' | ./BiblioCell papers update P-xxxx
./BiblioCell papers attach P-xxxx paper.pdf
./BiblioCell papers attach P-xxxx -type supplement table-s1.xlsx
./BiblioCell papers files P-xxxx
./BiblioCell papers list -needs-review -batch run-1 -limit 100 -offset 100
./BiblioCell papers undo run-1

# 预印本正式发表后：找重复、问 Crossref、合并（期刊版在前，预印本在后）
./BiblioCell papers duplicates
./BiblioCell papers published -limit 50
./BiblioCell papers merge P-journal P-preprint -reason "Crossref is-preprint-of 10.1016/j.cell.2019.05.031"
./BiblioCell papers merged P-journal

# 将单篇文献移入回收站（设置中可恢复），成功返回 {"id":"P-...","deleted":true}
./BiblioCell papers delete P-...
./BiblioCell datasets list -status pending
./BiblioCell datasets get DS-xxxx
echo '{"title":"...","repositories":[{"repository":"geo","accession":"GSE1"}]}' | ./BiblioCell datasets add
echo '{"status":"identified"}' | ./BiblioCell datasets update DS-xxxx
echo '{"paperId":"P-xxxx","role":"generated","evidenceText":"..."}' | ./BiblioCell datasets link DS-xxxx
./BiblioCell evidence list -paper P-xxxx
./BiblioCell evidence add -f claims.json
./BiblioCell sources list
./BiblioCell sources set pubmed -enabled -query "plant single cell" -max 50
./BiblioCell sources sync pubmed
./BiblioCell sources auto -enabled=true -hours 24
./BiblioCell settings get
./BiblioCell settings set -font-size 16 -storage /abs/path/folder -mcp-token <TOKEN> -ncbi-api-key <KEY> -contact-email <EMAIL>
./BiblioCell mcp
./BiblioCell serve -addr 127.0.0.1:37841
BIBLIOCELL_ADMIN_PASSWORD=change-me ./BiblioCell web -addr 0.0.0.0:8080
./BiblioCell export bibtex -screening included > included.bib
./BiblioCell export datasets-csv -o datasets.csv
./BiblioCell sql "select id, title from papers order by added_at desc limit 5"
./BiblioCell db version
./BiblioCell db migrate
```

- `papers add` / `import` / `update` 和 `datasets`、`evidence` 使用与 MCP 相同的字段和校验规则；`import` 一批原子提交，`-batch` 指定批次，`-dry-run` 只校验不写入。
- `papers attach` 由本机命令行执行，可以读取任意路径；MCP 的 `attach_file` 只能读 `inbox/`。
- `papers attach` 把文件复制进存放文件夹的 `papers/`，默认记为全文，`-type supplement` 记为补充材料。
- `export KIND` 把文献或数据集写成文件，见[导出](export.md)；不带 `-o` 时内容直接打到 stdout，带 `-o` 时输出写入路径和记录数的 JSON。
- `sql` 直接执行任意语句：`SELECT` / `WITH` / `PRAGMA` 打印行，其他语句打印受影响行数。它绕过应用层校验，改数据前先备份。
- `db version` 只读取数据库文件的版本、已应用和待应用的迁移，不打开、不创建、不修改库；`db migrate` 把库升级到当前版本（先备份），其他子命令和桌面应用在打开库时也会自动做同样的事。见[数据库版本与迁移](database-migrations.md)。
- 数据目录按 `-data-dir`、`$BIBLIOCELL_DATA_DIR`、应用默认用户数据目录的顺序确定；在 macOS 上 `.app` 包内和 `go run` 的默认目录名相同，命令行与桌面应用看到的是同一个库。
- `settings set` 修改的存放文件夹和 MCP Token 在下次启动桌面应用后生效。
- 桌面应用运行中也可以同时使用命令行，SQLite 自带锁，忙等 5 秒后报错。

---

[← 返回文档目录](README.md)
