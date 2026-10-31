# BiblioCell

基于原始网页原型开发的 Go 桌面应用，使用 [mygo](https://github.com/egoist/mygo) 提供原生窗口和类型化 Go ↔ TypeScript 调用，使用纯 Go SQLite 保存正式数据。

## 两种数据模式

应用默认启用 **Demo 模式**：

- 展示明确标记的虚构文献、数据集与附件。
- 修改保存在浏览器 `localStorage`，不会写入正式数据库。
- 用于体验页面和交互。

在左下角 **设置** 页面关闭 **Demo 演示模式** 后进入 **真实数据库模式**：

- 从本机 `bibliocell.sqlite` 加载文献，初始数据库为空。
- 支持文献新增、编辑、笔记、标签、收藏、阅读进度与收录状态。
- 一篇文献可记录多个物种（学名 + 中文名）、多个组织、多种测序类型（scRNA-seq / snRNA-seq / scATAC-seq 等）、多个测序平台（10x Genomics Chromium / BD Rhapsody 等）和多种文献类型（原创研究 / 方法研究 / 综述 / 相关资料），筛选时按"包含其中一项"匹配。
- 数据集支持真实新增、编辑、文献关联、多物种、多组织、多测序类型、多平台、仓库 accession、整理状态、可用性和备注。
- 数据集仓库编号规范化存入 `repository_records` 和 `dataset_repositories`；文献角色存入 `paper_datasets`，保存时保留关联证据和人工核验状态。
- Demo 与正式数据完全隔离，切换模式不会复制或清空任何一边的数据。
- 文献详情的“数据与资料”可导入全文和补充材料：文件复制到存放文件夹的 `papers/`，原文件保持不动，导入全文后文献状态自动变为“已保存文件”。文件按资源 ID 命名（如 `papers/R-3fa9….pdf`），原文件名保留为标题，这样跨系统复制不受文件名编码和非法字符影响。同一文献重复导入相同内容不会多存一份。移除时文件进废纸篓。
- 资料链接和 JSON 导入目前只在 Demo 中开放。

## 设置

左下角 **设置** 页面提供：

- **Demo 演示模式**：切换 Demo 与真实数据库，切换时重新加载页面，不复制或合并数据。
- **数据存放位置**：选择一个文件夹，SQLite 数据库（`bibliocell.sqlite`）和文献文件（`papers/`）都保存在里面。可输入绝对路径，或通过原生目录对话框选择。新路径在重启后生效；应用不会移动或覆盖旧数据，所选文件夹没有数据库时会创建空库。
- **界面字号**：12–20px，拖动时实时预览，保存后跨重启保留。缩放覆盖导航、表格、详情抽屉和笔记等固定字号，不改变面板宽度。
- **外部 AI · MCP**：集中显示连接地址、工具和 Schema 资源，可查看、复制、自定义或重新生成 Bearer Token。Token 修改在重启后生效。
- 桌面左侧边栏可折叠为图标栏，折叠状态保存在本机；移动端使用抽屉式导航。

设置保存在 mygo 用户数据目录的 `settings.json`。Windows 默认配置文件为 `%APPDATA%\BiblioCell\settings.json`，默认存放文件夹为同一目录。页面分别显示配置的文件夹和本次运行实际使用的文件夹。

存放文件夹可以整体迁移：退出应用后把它复制或移动到其他位置（包括其他电脑），在设置中选中新位置并重启即可继续使用。数据库内不保存文件夹位置，附件的 `resources.local_path` 只存相对存放文件夹的路径，读取时拒绝任何跳出该文件夹的路径。字号和 MCP Token 属于本机偏好，保存在 `settings.json`，不随文件夹迁移。

旧版设置中的 `databasePath` 会在启动时自动转换为存放文件夹。如果数据库用了其他文件名，会在原文件夹内改名为 `bibliocell.sqlite`；同名文件已存在时不覆盖，保留旧设置并改用默认文件夹，需要手动处理。

从旧版 PlantCell Library 升级时，应用会在首次启动时自动把 `%APPDATA%\PlantCell Library` 中的 `settings.json`、`mcp-token` 和 `plantcell.sqlite` 一次性迁入 `%APPDATA%\BiblioCell`，默认数据库同时更名为 `bibliocell.sqlite`。自定义位置的数据库留在原文件夹，不会被移走。

## 更新与来源

真实数据库模式下，“更新与来源”页面可以保存每个来源的启用状态、检索式和单次结果上限（1–100），并手动运行同步：

- **PubMed**：NCBI E-utilities 检索与 XML 元数据，保留 PMID、DOI、作者和摘要。
- **Europe PMC**：核心文献元数据检索。
- **Crossref**：书目关键词检索，保存 DOI 和出版信息。
- **bioRxiv**：扫描最近 30 天的预印本，在本地按题目、摘要和分类匹配关键词。
- **GEO / SRA / ENA**：可选择仓库，保存 accession、标题和官方页面作为数据线索，不自动创建或关联数据集。
- **CNCB GSA**：站点会要求浏览器安全检查，因此仅提供官方手动入口，不绕过验证抓取。

只有点击“同步”才会访问远程公开 API，应用不会后台定时拉取。文献按 DOI、PMID 和规范化题目去重；已存在条目的笔记、标签、收录判断和阅读状态不会被远端结果覆盖。新文献默认进入“待判断”。配置、来源记录快照、每次成功/失败的运行记录和最近成功时间均保存在正式 SQLite 数据库中。Demo 数据不会与同步结果混合。

## 外部 AI：MCP

应用启动后在本机回环地址提供 Streamable HTTP MCP 服务：

```text
http://127.0.0.1:37841/mcp
```

提供以下工具：

| 工具 | 用途 |
|---|---|
| `describe_library` | 获取 Schema 资源、字段枚举和导入规则 |
| `library_status` | 查看正式数据库状态与记录数量 |
| `list_papers` | 查询、搜索正式文献，不返回 Demo 数据 |
| `get_paper` | 按 ID 读取一篇文献及全部可编辑 metadata |
| `add_paper` | 向正式 SQLite 数据库添加一篇文献，需要写权限 |
| `import_papers` | 原子批量导入最多 1000 篇文献，支持 `dryRun`，需要写权限 |
| `update_paper` | 局部编辑未经人工确认的文献 metadata，或提出收录建议，保留未提供字段，需要写权限 |

### MCP 权限

- **无认证**：只读，可调用 `describe_library`、`library_status`、`list_papers`、`get_paper`，并读取 Schema 资源。
- **Bearer Token 认证**：额外允许 `add_paper`、`import_papers` 和 `update_paper`。
- 无 Token 的写调用会被拒绝；错误 Token 返回 HTTP 401。
- Token 是应用首次启动时生成的 256 位随机密钥。可以在“设置 → 外部 AI · MCP”中查看、复制、自定义或重新生成；自定义 Token 保存在 `settings.json`，修改后重启生效。Token 不会通过 MCP 暴露。
- `describe_library` 返回当前连接的 `accessRole`（`read-only` 或 `read-write`）和 `canWrite`。

### 人工核查

收录判断只由人做出，agent 只能提议：

- `screening`（`pending` / `included` / `excluded` / `uncertain`）只记录人的决定，MCP 和命令行都不能写入。agent 用 `proposedScreening` 和 `proposedReason` 提出建议，提议排除时必须写理由。
- 建议显示在“待补事项”和文献详情的“收录判断”里，人可以采纳、忽略，或自行选择。做出决定后建议随即失效。
- 数据库记录真实来源：只有人的决定会把 `screening_method` 写成 `human` 并记下 `screening_reviewed_at`，接口里对应 `reviewedAt`。未决定的文献是 `unreviewed`。
- 文献一经人工确认，`update_paper` 就只接受建议，不再允许修改 metadata。把判断改回“待判断”即可解除。
- 桌面端只保存你改动的字段：agent 在窗口打开期间写入的内容不会被覆盖。同一篇文献的同一字段被两边同时修改时，这次改动不保存并提示，窗口重新加载。窗口回到前台时自动读取最新数据，打开详情或弹窗时不刷新。
- 从旧版本升级时，来源为 MCP 或 CLI 的已判断文献分不清是谁判断的，会退回“待判断”，原判断保留为建议，可一键采纳。其他已判断文献记为人工确认。
- 这是防止误操作的边界，不是安全边界：`sql` 命令和直接修改 SQLite 文件仍可绕过。

`update_paper` 使用 `id` 和 `patch`。例如只修改期刊时传入 `{"id":"P-...","patch":{"journal":"New journal"}}`，未提供的 DOI、作者、笔记等字段保持不变；`clearYear: true` 可以将年份清空。多值字段（`species`、`organs`、`modalities`、`platforms`、`articleTypes`）以数组整体替换，例如 `{"patch":{"platforms":["10x Genomics Chromium","BD Rhapsody"]}}`，空数组表示清空。

### MCP 自描述与 Schema

AI 客户端可以通过标准 MCP 能力发现接口，不需要在提示词里硬编码字段：

- `tools/list` 返回每个工具的 `inputSchema` 和 `outputSchema`。
- `bibliocell://schema/paper` 提供单篇文献的 JSON Schema。
- `bibliocell://schema/sql` 提供完整 SQLite 表、约束、索引和视图 DDL。
- `describe_library` 汇总必填字段、状态枚举、导入规则和上述资源入口。

推荐导入流程：先调用 `describe_library`，读取文献 Schema，再用 `import_papers` 设置 `dryRun: true` 校验，最后将 `dryRun` 设为 `false` 正式导入。每批最多 1000 篇，任何一条失败都会回滚整个批次；已有 ID 或重复 DOI 会报错，不会静默覆盖旧记录。

通用 MCP 客户端配置示例（具体字段以所用 AI 客户端为准）：

```json
{
  "mcpServers": {
    "bibliocell-library": {
      "url": "http://127.0.0.1:37841/mcp",
      "headers": {
        "Authorization": "Bearer <MCP_TOKEN>"
      }
    }
  }
}
```

省略 `headers` 即为只读连接；需要导入或编辑时，将 `<MCP_TOKEN>` 替换为应用页面显示的 Token。

MCP 始终操作正式数据库，与界面是否开启 Demo 无关。MCP 新增或编辑的文献在窗口回到前台时可见。服务仅监听 `127.0.0.1`，不要将端口转发到局域网或公网。

## 命令行

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
./BiblioCell datasets list
./BiblioCell sources list
./BiblioCell sources set pubmed -enabled -query "plant single cell" -max 50
./BiblioCell sources sync pubmed
./BiblioCell settings get
./BiblioCell settings set -font-size 16 -storage /abs/path/folder -mcp-token <TOKEN>
./BiblioCell mcp
./BiblioCell sql "select id, title from papers order by added_at desc limit 5"
```

- `papers add` / `import` / `update` 使用与 MCP 相同的字段和校验规则；`import` 一批原子提交，`-dry-run` 只校验不写入。
- `papers attach` 把文件复制进存放文件夹的 `papers/`，默认记为全文，`-type supplement` 记为补充材料。
- `sql` 直接执行任意语句：`SELECT` / `WITH` / `PRAGMA` 打印行，其他语句打印受影响行数。它绕过应用层校验，改数据前先备份。
- 数据目录按 `-data-dir`、`$BIBLIOCELL_DATA_DIR`、应用默认用户数据目录的顺序确定；在 macOS 上 `.app` 包内和 `go run` 的默认目录名相同，命令行与桌面应用看到的是同一个库。
- `settings set` 修改的存放文件夹和 MCP Token 在下次启动桌面应用后生效。
- 桌面应用运行中也可以同时使用命令行，SQLite 自带锁，忙等 5 秒后报错。

## 目录

```text
BiblioCell/
├── main.go                  # mygo 应用、窗口、SQLite 与 MCP 生命周期
├── service.go               # 暴露给前端的 Library 服务
├── cli.go                   # 命令行子命令：文献、来源、设置、原始 SQL
├── internal/library/
│   ├── store.go             # 数据库打开、迁移与完整性检查
│   ├── auth.go              # MCP 写权限 Token 的生成与持久化
│   ├── paper.go             # 正式文献 CRUD 与原型模型映射
│   ├── dataset.go           # 数据集 CRUD、仓库与文献关联
│   ├── source.go            # 来源配置、同步运行、去重与外部记录快照
│   ├── source_clients.go    # 文献公开 API 客户端与解析器
│   ├── source_repositories.go # GEO / SRA / ENA 仓库检索
│   ├── mcp.go               # 本机 MCP 服务与 AI 工具
│   ├── schema.sql           # 嵌入二进制的数据库结构 v5
│   └── *_test.go            # 数据库与 MCP 端到端测试
├── internal/settings/       # 设置校验、默认值与原子 JSON 持久化
├── src/
│   ├── main.ts              # 入口：Demo / 真实模式判断与启动
│   ├── app.ts               # 页面状态、渲染循环与事件委托
│   ├── model.ts             # 纯逻辑：筛选、待补事项、DOI 规范化、导入校验、CSV
│   ├── store.ts             # 持久化：SQLite（经 Go）与 localStorage（Demo）两种实现
│   ├── views.ts / drawer.ts / forms.ts / sources.ts / settings.ts  # 各页面、抽屉与弹窗
│   ├── ui.ts / icons.ts / shell.ts  # HTML 片段构造、图标与侧栏等静态外壳
│   ├── demo-data.ts         # 明确标记的虚构演示数据
│   ├── style.css            # 单一样式表（外壳与设置页 → 基础 → 组件）
│   └── mygo.ts              # mygo generate 生成的类型化客户端
├── tests/                   # node:test 纯逻辑测试（npm test）
├── public/                  # 图标、设计文档与可下载资料
├── resources/               # 应用图标
├── mygo.config.ts           # mygo 开发、构建和应用元数据
└── package.json             # Vite / TypeScript 前端工具链
```

## 开发环境

- Go 1.27.3 或更新版本（Go 1.26 可通过自动工具链下载运行）
- Node.js 与 npm
- Windows 10/11 的 WebView2；Windows 11 通常已内置

```bash
npm install
npm run dev
```

`mygo dev` 会生成 `src/mygo.ts`、启动 Vite，并打开桌面应用。普通浏览器也可以通过 `npm run dev:web` 预览 Demo，但无法连接 Go 后端、真实数据库或 MCP。`npm run typecheck` 做 TypeScript 严格检查，`npm test` 运行 `tests/` 下的纯逻辑测试（Node 22.6+ 自带类型剥离，无需额外依赖）。

## 验证与构建

```bash
CGO_ENABLED=0 go test ./...
npm run typecheck
npm run build
```

测试覆盖数据库初始化与重开、文献编辑、数据集多值往返、共享仓库记录保护、文献关联证据保留、来源配置持久化、同步去重与人工整理保护、失败运行记录、公开 API 可选冒烟测试、MCP 新增记录不被界面保存覆盖、真实 MCP 客户端调用、工具与资源 Schema、`dryRun` 校验、失败批次整体回滚、无认证只读、错误 Token 拒绝、认证后的 metadata 局部更新，以及设置默认值、校验和持久化。

需要实际访问公开来源时，可额外运行 `BIBLIOCELL_LIVE_TEST=1 go test ./internal/library -run TestRemoteConnectorsLive -v`；默认测试不会访问网络。

打包结果位于 `build/`，前端构建结果位于 `dist/`。正式数据库保存在 mygo 的用户数据目录，不在源码目录中。
