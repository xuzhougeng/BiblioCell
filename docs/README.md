# BiblioCell 文档

[← 返回项目首页](../README.md)

## 使用

| 文档 | 内容 |
|---|---|
| [数据模式与数据集资源](data.md) | Demo 与真实数据库模式、文献与数据集字段、数据集资源准备（GEO 文件发现、存储目录、文件校验、参考序列、资源清单） |
| [设置](settings.md) | Demo 开关、数据存放位置与迁移、界面语言与字号、MCP Token、AI 模型、网络代理、旧版数据升级 |
| [文献跟踪](literature-tracking.md) | PubMed / Europe PMC / Crossref / bioRxiv / GEO / SRA / ENA 来源、凭证、书目补全、预印本合并、开放全文、自动更新、去重规则、让助手优化检索式 |
| [内置 AI 助手](ai-assistant.md) | 模型协议配置、对话保存、AI 筛选建议与人工对照、全文检索与可点击引用 |
| [导出](export.md) | BibTeX、CSV、进度报告、数据集汇总、批量下载脚本、accession 列表 |

## 集成与部署

| 文档 | 内容 |
|---|---|
| [外部 AI：MCP](mcp.md) | MCP 工具列表、权限与 Token、人工核查边界、Agent 工作流、Schema 自描述、客户端配置 |
| [命令行](cli.md) | 全部子命令示例与行为说明 |
| [网页版：内网部署](web-deployment.md) | `web` 模式、只读访客与管理员、服务端权限、构建与 Docker 部署 |

## 开发与维护

| 文档 | 内容 |
|---|---|
| [开发指南](development.md) | 开发环境、测试与构建、界面翻译规则、源码目录说明 |
| [数据库版本与迁移](database-migrations.md) | `user_version` 与迁移流程、备份、新增一条迁移的步骤 |
| [发布](release.md) | 版本号、发布 workflow、签名与公证、自动更新包、所需 secrets |
