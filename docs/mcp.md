# 外部 AI：MCP

桌面应用启动后在本机回环地址提供 Streamable HTTP MCP 服务。不开窗口时用 `./BiblioCell serve` 提供同一服务，适合由 agent 调度；两者默认用同一端口，只能运行一个，`serve -addr 127.0.0.1:PORT` 可换端口，只接受回环地址。


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
| `add_paper` | 向正式 SQLite 数据库添加一篇文献，已存在时报错，需要写权限 |
| `import_papers` | 原子批量导入最多 1000 篇文献，跳过已有文献并返回 `batchId`，支持 `dryRun`，需要写权限 |
| `update_paper` | 局部编辑未经人工确认的文献 metadata，或提出收录建议，保留未提供字段；已筛选的文献只能改文章类型、物种、器官、技术、平台这些分类字段和收录建议，以及还空着的 DOI、PMID、作者、年份、期刊、发表状态、摘要，需要写权限 |
| `delete_paper` | 按 `id` 将单篇文献移入回收站，日常列表隐藏；保留附件、笔记、证据和关联，来源更新与导入仍按保留标识去重；可在「设置 → 记录管理 → 回收站」恢复，需要写权限 |
| `undo_import` | 撤销一个导入批次，保留人已处理过的记录，需要写权限 |
| `list_datasets` / `get_dataset` | 查询、分页读取数据集及其仓库编号和文献关联 |
| `add_dataset` | 新建数据集；按 accession 或题目找到已有数据集时只补充新关联，需要写权限 |
| `update_dataset` | 局部编辑未经人工核实的数据集，需要写权限 |
| `link_dataset` | 记录文献与数据集的关联，必须附依据，需要写权限 |
| `add_evidence` | 按字段记录提取依据：摘录和来源，供人核对，需要写权限 |
| `list_evidence` | 读取证据及人的判断 |
| `attach_file` | 把存放文件夹 `inbox/` 中的文件移入文库并挂到文献上，需要写权限 |
| `find_duplicates` | 按题目找疑似重复的文献组，附建议保留的一篇，只读本地库 |
| `find_published_versions` | 问 Crossref 库里的预印本有没有期刊版（`is-preprint-of`），不写入任何内容；会访问远程来源，需要写权限 |
| `merge_papers` | 把重复文献（通常是预印本）合并进保留的文献并删除它，需填理由；两篇的人工筛选结论不同时拒绝，需要写权限 |
| `list_merged_papers` | 列出合并进某篇文献的原记录：ID、题目、DOI、PMID、理由和时间 |
| `lookup_metadata` | 按 `doi` 或 `pmid` 从 Crossref 和 Europe PMC 查书目信息（期刊、作者、年份、摘要等），不写入任何内容；会访问远程来源，需要写权限 |
| `fetch_fulltext` | 通过 PMC 和 Unpaywall 查找开放获取 PDF 并保存，需要写权限 |
| `list_files` | 列出文献的附件，路径相对存放文件夹 |
| `search_fulltext` | 在已保存的全文里按短语或关键词检索，返回命中的页、逐字片段和 `cite:` 链接地址 |
| `read_fulltext` | 按页读取一篇文献的全文（每次最多 8 页），用于核对上下文和逐字引用 |
| `list_sources` | 读取各来源的检索式、检索语法、最近运行和最近一次检索式修改 |
| `try_source_query` | 试运行检索式并对照文库评估，不写入任何内容；会访问远程来源，需要写权限 |
| `update_source_query` | 保存新检索式并记录理由，需要写权限 |
| `source_query_history` | 读取一个来源的检索式修改记录 |

`list_papers` 可按 `query`、`screening`、`source`、`batchId`、`needsReview` 筛选，用 `limit`（最大 500）和 `offset` 翻页，`total` 是匹配总数。`list_datasets` 同样分页。

## MCP 权限

- **无认证**：只读，可调用所有读取工具（`describe_library`、`library_status`、`list_*`、`get_*`），并读取 Schema 资源。
- **Bearer Token 认证**：额外允许标注“需要写权限”的工具。
- 无 Token 的写调用会被拒绝；错误 Token 返回 HTTP 401。
- Token 是应用首次启动时生成的 256 位随机密钥。可以在“设置 → 外部 AI · MCP”中查看、复制、自定义或重新生成；自定义 Token 保存在 `settings.json`，修改后重启生效。Token 不会通过 MCP 暴露。
- `describe_library` 返回当前连接的 `accessRole`（`read-only` 或 `read-write`）和 `canWrite`。

## 人工核查

收录判断只由人做出，agent 只能提议：

- `screening`（`pending` / `included` / `excluded` / `uncertain` / `irrelevant`）只记录人的决定，MCP 和命令行都不能写入。agent 用 `proposedScreening` 和 `proposedReason` 提出建议，提议排除时必须写理由。
- 界面上 `excluded` 叫“相关不纳入”：有点关系但够不上标准，记录、笔记和附件都保留。`irrelevant` 叫“无关 / 排除”，是清理：逐篇筛选里按 D 把文献标为无关，同时删除已保存的附件副本。记录本身留在库中，所以来源更新和导入会把它当作已有文献跳过，不会再加回来；文献库列表、首页统计、待补事项和 `list_papers` 默认都不显示无关文献，要看或改回来可在文献库按收录状态筛选 `irrelevant`。
- 建议显示在“待补事项”和文献详情的“收录判断”里，人可以采纳、忽略，或自行选择。做出决定后建议随即失效。
- 数据库记录真实来源：只有人的决定会把 `screening_method` 写成 `human` 并记下 `screening_reviewed_at`，接口里对应 `reviewedAt`。未决定的文献是 `unreviewed`。
- 文献一经人工确认，`update_paper` 就只接受建议，不再允许修改 metadata。把判断改回“待判断”即可解除。数据集同理：经人工核实后 `update_dataset` 被拒绝，改回其他状态即可解除。
- 证据（`add_evidence`）、数据集仓库编号和文献关联由 agent 记录，只有人能在桌面端确认、驳回或核实，MCP 和命令行没有对应入口。证据的判断 agent 可以读到；已核实的关联不会被 agent 改动。同一条证据重复提交不会重置人的判断。证据不影响字段本身：驳回只表示人不认可这条依据。
- 移除文件时，来自该文件的证据一并删除。
- 桌面端只保存你改动的字段：agent 在窗口打开期间写入的内容不会被覆盖。同一篇文献的同一字段被两边同时修改时，这次改动不保存并提示，窗口重新加载。窗口回到前台时自动读取最新数据，打开详情或弹窗时不刷新。
- 从旧版本升级时，来源为 MCP 或 CLI 的已判断文献分不清是谁判断的，会退回“待判断”，原判断保留为建议，可一键采纳。其他已判断文献记为人工确认。
- 这是防止误操作的边界，不是安全边界：`sql` 命令和直接修改 SQLite 文件仍可绕过。

## Agent 工作流

1. `describe_library` 读取规则和 `inboxPath`；一次运行用同一个 `batchId`。
2. `import_papers` 导入文献，已有文献被跳过并列在 `skipped`，可以放心重跑。`add_dataset` 同样可重跑。
3. 把 PDF 放进 `inboxPath`，再用 `attach_file` 挂到文献上。只能读取 `inbox/` 内的文件，`..` 和指向外部的符号链接都会被拒绝。文件移入文库后从 inbox 消失。
4. `add_dataset` / `link_dataset` 记录数据集和关联，`add_evidence` 记录每个字段的摘录和来源，`update_paper` 提交 `proposedScreening`。
5. 人在桌面端核查。agent 用 `list_papers` 的 `needsReview` 和 `list_evidence` 查看进度和判断。
6. 这批导入错了就 `undo_import`：删除未经人处理的文献、数据集和它们的文件副本；人已确认、收藏、阅读或核实过的记录保留并列在 `kept`。

`update_paper` 使用 `id` 和 `patch`。例如只修改期刊时传入 `{"id":"P-...","patch":{"journal":"New journal"}}`，未提供的 DOI、作者、笔记等字段保持不变；`clearYear: true` 可以将年份清空。多值字段（`species`、`organs`、`modalities`、`platforms`、`articleTypes`）以数组整体替换，例如 `{"patch":{"platforms":["10x Genomics Chromium","BD Rhapsody"]}}`，空数组表示清空。

## MCP 自描述与 Schema

AI 客户端可以通过标准 MCP 能力发现接口，不需要在提示词里硬编码字段：

- `tools/list` 返回每个工具的 `inputSchema` 和 `outputSchema`。
- `bibliocell://schema/paper` 提供单篇文献的 JSON Schema。
- `bibliocell://schema/sql` 提供完整 SQLite 表、约束、索引和视图 DDL。
- `describe_library` 汇总必填字段、状态枚举、导入规则和上述资源入口。

推荐导入流程：先调用 `describe_library`，读取文献 Schema，再用 `import_papers` 设置 `dryRun: true` 校验，最后将 `dryRun` 设为 `false` 正式导入。每批最多 1000 篇，任何一条无效都会回滚整个批次；按 ID、DOI、PMID、题目识别出的已有文献被跳过，不会覆盖旧记录。

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

---

[← 返回文档目录](README.md)
