# 开发指南

## 开发环境

- Go 1.27.3 或更新版本（Go 1.26 可通过自动工具链下载运行）
- Node.js 与 npm
- Windows 10/11 的 WebView2；Windows 11 通常已内置

```bash
npm install
npm run dev
```

`mygo dev` 会生成 `src/mygo.ts`、启动 Vite，并打开桌面应用。普通浏览器也可以通过 `npm run dev:web` 预览 Demo，但无法连接 Go 后端、真实数据库或 MCP。`npm run typecheck` 做 TypeScript 严格检查，`npm test` 运行 `tests/` 下的全部测试（Node 22.6+ 自带类型剥离）。纯逻辑测试直接 import `src/` 的模块；`*-dom.test.ts` 通过 `tests/dom.ts` 装一个 jsdom 窗口（唯一的测试依赖），用真实 DOM 和事件驱动 `App`，覆盖文献库的分页、排序、筛选胶囊与方向键，以及逐篇筛选页在 AI 建议、判断、元数据编辑和刷新时保住 PDF 阅读器。写新的界面测试时先 `import { resetDom } from "./dom.ts"`（必须是第一个 import），每个测试开头调用 `resetDom()`。

## 验证与构建

```bash
CGO_ENABLED=0 go test ./...
npm run typecheck
npm run build
```

测试覆盖数据库初始化与重开、从 v0.1.0 数据库升级与备份、迁移失败回滚、迁移链与 schema.sql 一致性、文献编辑、AI 筛选建议记录与一致率 / 混淆矩阵、证据页码定位、数据集多值往返、共享仓库记录保护、文献关联证据保留、来源配置持久化、跨来源去重与人工整理保护、定时更新到期判断与补跑、同一时间单次运行、已有数据迁移、失败运行记录、公开 API 可选冒烟测试、MCP 新增记录不被界面保存覆盖、真实 MCP 客户端调用、工具与资源 Schema、`dryRun` 校验、失败批次整体回滚、无认证只读、错误 Token 拒绝、认证后的 metadata 局部更新，以及设置默认值、校验和持久化；导出的 BibTeX 转义、CSV 多值字段、accession 分类、下载脚本和进度报告在 `internal/library/export_test.go` 与 `tests/export.test.ts` 中各有一套。

需要实际访问公开来源时，可额外运行 `BIBLIOCELL_LIVE_TEST=1 go test ./internal/library -run TestRemoteConnectorsLive -v`；默认测试不会访问网络。

打包结果位于 `build/`，前端构建结果位于 `dist/`。正式数据库保存在 mygo 的用户数据目录，不在源码目录中。

## 界面翻译

界面文字以中文为源语言：代码里的中文字符串本身就是翻译键，`t("共 {n} 篇", { n })` 在中文界面原样返回，在英文界面查 `src/locales/en/` 下的词典，缺失时回退中文。新增界面文字时用 `t()` 包裹并在对应词典补一条英文；`npm test` 会检查每个 `t()` 都有英文、占位符一致，且没有绕过 `t()` 的中文。保存进数据库的值（文献类型、来源名称等）保持中文，显示时用 `tv()` 翻译。Go 后端返回的错误信息和 Demo 示例数据暂时只有中文。

## 目录

```text
BiblioCell/
├── cmd/bibliocell/          # Go 主包（mygo.config.ts 的 main），go run ./cmd/bibliocell
│   ├── main.go              # mygo 应用、窗口、SQLite 与 MCP 生命周期
│   ├── service.go           # 暴露给前端的 Library 服务（service_*.go 按功能拆分）
│   ├── background.go        # 托盘图标与关窗后后台运行
│   ├── update.go            # 应用自动更新：检查 Release、后台下载与重启替换
│   ├── cli.go               # 命令行子命令：文献、来源、设置、原始 SQL
│   ├── web.go               # 网页版：HTTP 调用分发、方法权限表、管理员会话、事件流、静态文件
│   └── *_test.go            # 服务、命令行、网页版与自动更新测试
├── assets.go / assets_webdist.go  # 根包 bibliocell：嵌入托盘图标，-tags webdist 时嵌入 dist/
├── internal/library/
│   ├── store.go             # 数据库打开、基线前的旧库修复与完整性检查
│   ├── migrate.go           # 版本化迁移：迁移列表、备份、db version / migrate 的实现
│   ├── auth.go              # MCP 写权限 Token 的生成与持久化
│   ├── paper.go             # 正式文献 CRUD 与原型模型映射
│   ├── dataset.go           # 数据集 CRUD、仓库与文献关联
│   ├── source.go            # 来源配置、同步运行、去重与外部记录快照
│   ├── source_automation.go # 自动更新设置、到期判断与后台调度
│   ├── source_clients.go    # 文献公开 API 客户端与解析器
│   ├── source_repositories.go # GEO / SRA / ENA 仓库检索
│   ├── source_query.go      # 试检索评估、助手更新检索式与修改记录
│   ├── export.go            # BibTeX / CSV / 下载脚本 / 进度报告导出与 accession 分类
│   ├── mcp.go               # 本机 MCP 服务与 AI 工具
│   ├── suggestion.go / calibration.go  # AI 筛选建议记录、一致率与混淆矩阵
│   ├── schema.sql           # 嵌入二进制的当前数据库结构（新库一次建成）
│   ├── testdata/            # 冻结的历史 schema：v11（v0.1.0 发布版）与 v13（迁移基线）
│   └── *_test.go            # 数据库、迁移与 MCP 端到端测试
├── internal/agent/          # 内置 AI 助手：agent loop、Anthropic / OpenAI 兼容协议、进程内 MCP 工具
├── internal/proxy/          # 联网请求的代理选择：系统 / 直连 / 自定义，及单项覆盖
├── internal/settings/       # 设置校验、默认值与原子 JSON 持久化
├── src/
│   ├── main.ts              # 入口：桌面 / 网页版 / Demo 预览的判断与启动
│   ├── env.ts               # 运行环境允许什么：有后端、管理员、可写，以及只读访问者可用的操作
│   ├── web-runtime.ts / web.ts  # 浏览器里替代 mygo 桥的 HTTP 调用与事件流；管理员登录界面
│   ├── app.ts               # 页面状态、渲染循环与事件委托
│   ├── model.ts             # 纯逻辑：筛选、待补事项、DOI 规范化、导入校验
│   ├── export.ts            # 导出弹窗；浏览器预览用的 TypeScript 版导出格式
│   ├── store.ts             # 持久化：SQLite（经 Go）与 localStorage（Demo）两种实现
│   ├── views.ts / drawer.ts / forms.ts / sources.ts / settings.ts  # 各页面、抽屉与弹窗
│   ├── screen.ts / screen-batch.ts / calibration.ts  # 逐篇筛选、批量 AI 筛选与 AI 对照
│   ├── ui.ts / icons.ts / shell.ts  # HTML 片段构造、图标与侧栏等静态外壳
│   ├── demo-data.ts         # 明确标记的虚构演示数据
│   ├── style.css            # 单一样式表（外壳与设置页 → 基础 → 组件）
│   └── mygo.ts              # mygo generate 生成的类型化客户端
├── tests/                   # node:test 测试（npm test）：纯逻辑测试与 jsdom 界面交互测试
├── public/                  # 图标、设计文档与可下载资料
├── resources/               # 应用图标
├── mygo.config.ts           # mygo 开发、构建和应用元数据（main 指向 cmd/bibliocell）
├── Dockerfile               # 网页版服务器镜像
└── package.json             # Vite / TypeScript 前端工具链
```

---

[← 返回文档目录](README.md)
