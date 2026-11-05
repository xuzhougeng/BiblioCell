# BiblioCell

基于原始网页原型开发的 Go 桌面应用，使用 [mygo](https://github.com/egoist/mygo) 提供原生窗口和类型化 Go ↔ TypeScript 调用，使用纯 Go SQLite 保存正式数据。

## 界面预览

在线体验（Demo 数据，浏览器预览版，修改只保存在你自己的浏览器里）：**https://xuzhougeng.github.io/BiblioCell/**

| 总览 · 收录与整理进度 | 文献库 · 筛选、排序与分页 |
|:---:|:---:|
| ![总览](docs/screenshots/overview.png) | ![文献库](docs/screenshots/papers.png) |
| **文献详情 · 摘要与收录判断** | **数据集 · 仓库编号与关联文献** |
| ![文献详情](docs/screenshots/paper-drawer.png) | ![数据集](docs/screenshots/datasets.png) |
| **数据集详情 · Accession 直达仓库** | **待补事项 · 按整理状态生成** |
| ![数据集详情](docs/screenshots/dataset-drawer.png) | ![待补事项](docs/screenshots/tasks.png) |

## 下载安装

正式版本在 [GitHub Releases](https://github.com/xuzhougeng/BiblioCell/releases) 发布：

- **macOS**：`bibliocell-<版本>.dmg`，Apple Silicon 与 Intel 通用。已用 Developer ID 签名并经 Apple 公证，打开后把 BiblioCell 拖进“应用程序”即可。
- **Windows**：`bibliocell-setup-<版本>-amd64.exe`，带 Authenticode 签名的安装包，安装到当前用户目录，不需要管理员权限。运行需要 Microsoft Edge WebView2（Windows 11 自带）。

装好之后不用再手动下载：应用启动后和每隔 6 小时检查一次新版本，发现后在后台下载并校验签名，下好后顶栏左侧出现“↑ 更新”，点一下确认即重启换上新版本；不点也没关系，下次打开时会自动换上。悬停时出现的 × 可以隐藏这个版本的提示。0.2.0 及更早的版本还没有这个功能，需要手动下载安装一次之后的版本。

发布流程见下文“发布”。

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
- 文献详情显示 agent 记录的证据，可逐条确认或驳回；文献与数据集的关联、数据集的仓库编号都可以逐条核实。文献列表可按来源筛选。
- 通用资料链接和 JSON 导入目前只在 Demo 中开放；正式数据集的文件链接通过下述资源清单登记。

## 数据集资源准备

数据集详情的 **数据集资源与存储** 用于第一阶段整理：查清公开文件、样本归属、参考版本和存储位置。支持 GEO 系列及样本的补充文件发现，其他仓库可手动登记 HTTP(S)/FTP 文件地址。文件名识别只是初步分类，压缩包保持“其他 / 待检查”，矩阵数值默认为“待确认”。重复发现只补充新样本与新文件，不覆盖已有人工整理。

- **创建存储目录**：在设置的存放文件夹内创建 `datasets/<内部数据集 ID>/source/`、`original/`、`standardized/`、`metadata/`，以及共享的 `references/`。创建目录不代表文件已下载。
- **登记文件**：记录表达矩阵、细胞/基因信息、PEP、CDS、基因—转录本—蛋白映射或原始 reads，关联一个或多个样本。现有本地文件填写相对存放文件夹的路径；文件不会被移动或复制。
- **校验本地文件**：实际读取文件并计算 SHA-256。再次校验发现内容改变时显示异常，保留上一次指纹；确认要接纳替换文件时，先移除记录再重新登记。文件读取被限制在存放文件夹内。
- **参考序列与版本**：按物种、组装、注释登记候选版本，确认匹配时必须填写版本与依据。PEP/CDS/映射文件通过版本标识关联，可共享 `references/` 下的同一份文件。只有所有登记的参考都确认匹配，且三类文件均校验过，才显示“参考序列已匹配”。这不是自动的生物学一致性验证，也不等于分析就绪。
- **导出资源清单**：将当前快照写入对应目录的 `manifest.json`。后续修改不会实时更新旧快照，需要再次导出。移除文件记录或删除数据集均保留这些磁盘文件，避免删除共享参考或手动放入的原始数据。
- **整理进度筛选**：按目录、矩阵记录、文件校验和参考匹配分别筛选，独立于原有元数据核查状态。文件状态表示上次校验结果，磁盘文件改动后需重新校验。

GEO 发现覆盖整个 GSE，而内部数据集可能只使用其中一部分。请核对样本范围；没有单独列出的矩阵可能在压缩包或其他仓库中，不能据此判定不存在。样本说明保留 GEO 的来源、实验特征和数据处理记录，供追溯参考版本。

本阶段不自动下载大文件、不解压检查矩阵、不从 FASTQ 重算、不推断 PEP/CDS 匹配，也不生成分析就绪标记。按上述流程逐步准备资源后，再做统一矩阵格式和基因 ID 映射验证。schema v20 新增资源清单与工作目录记录，旧库通过现有迁移机制升级。

命令行也可用于批量整理：

```sh
BiblioCell datasets discover DS-xxx GSE123456
BiblioCell datasets manifest DS-xxx
BiblioCell datasets prepare DS-xxx
BiblioCell datasets save-file DS-xxx -f file.json
BiblioCell datasets verify-file DS-xxx DA-xxx
BiblioCell datasets export-manifest DS-xxx
```

`save-file` 的 JSON 至少包含 `title`、`kind` 和 `url` 或 `localPath`，可选 `matrixKind`（`unknown`、`counts`、`normalized`）、`sampleIds`、`referenceKey`、`notes`；更新已有记录时附上其 `id`。示例：`{"title":"counts.mtx.gz","kind":"matrix","matrixKind":"counts","localPath":"datasets/DS-xxx/original/counts.mtx.gz"}`。

## 设置

左下角 **设置** 页面提供：

- **Demo 演示模式**：切换 Demo 与真实数据库，切换时重新加载页面，不复制或合并数据。
- **数据存放位置**：选择一个文件夹，SQLite 数据库（`bibliocell.sqlite`）和文献文件（`papers/`）都保存在里面。通过原生目录对话框选定后立即切换；也可输入绝对路径，点“保存设置”后切换。切换前先试打开新文件夹，打不开就报错，设置和当前数据库都不变；打得开就自动重启过去。应用不会移动或覆盖旧数据，所选文件夹没有数据库时会创建空库。
- **界面语言**：简体中文 / English，也可点顶栏右侧的 EN / 中 一键切换。首次打开按系统语言选择，之后记住你的选择（保存在本机）；切换时重新加载页面。
- **界面字号**：12–20px，拖动时实时预览，保存后跨重启保留。缩放覆盖导航、表格、详情抽屉和笔记等固定字号，不改变面板宽度。
- **外部 AI · MCP**：集中显示连接地址、工具和 Schema 资源，可查看、复制、自定义或重新生成 Bearer Token。Token 修改在重启后生效。
- **AI 模型**：内置助手使用的模型服务，见下文“内置 AI 助手”。
- 桌面左侧边栏可折叠为图标栏，折叠状态保存在本机；移动端使用抽屉式导航。

设置保存在 mygo 用户数据目录的 `settings.json`。Windows 默认配置文件为 `%APPDATA%\BiblioCell\settings.json`，默认存放文件夹为同一目录。页面分别显示配置的文件夹和本次运行实际使用的文件夹。

存放文件夹可以整体迁移：退出应用后把它复制或移动到其他位置（包括其他电脑），在设置中选择新位置即可继续使用。数据库内不保存文件夹位置，附件的 `resources.local_path` 只存相对存放文件夹的路径，读取时拒绝任何跳出该文件夹的路径。字号、MCP Token 和来源凭证属于本机偏好，保存在 `settings.json`，不随文件夹迁移。

旧版设置中的 `databasePath` 会在启动时自动转换为存放文件夹。如果数据库用了其他文件名，会在原文件夹内改名为 `bibliocell.sqlite`；同名文件已存在时不覆盖，保留旧设置并改用默认文件夹，需要手动处理。

从旧版 PlantCell Library 升级时，应用会在首次启动时自动把 `%APPDATA%\PlantCell Library` 中的 `settings.json`、`mcp-token` 和 `plantcell.sqlite` 一次性迁入 `%APPDATA%\BiblioCell`，默认数据库同时更名为 `bibliocell.sqlite`。自定义位置的数据库留在原文件夹，不会被移走。

## 界面翻译

界面文字以中文为源语言：代码里的中文字符串本身就是翻译键，`t("共 {n} 篇", { n })` 在中文界面原样返回，在英文界面查 `src/locales/en/` 下的词典，缺失时回退中文。新增界面文字时用 `t()` 包裹并在对应词典补一条英文；`npm test` 会检查每个 `t()` 都有英文、占位符一致，且没有绕过 `t()` 的中文。保存进数据库的值（文献类型、来源名称等）保持中文，显示时用 `tv()` 翻译。Go 后端返回的错误信息和 Demo 示例数据暂时只有中文。

## 内置 AI 助手

左侧 **AI 助手** 页面内置一个极简的 agent loop：把你的话发给配置好的模型，模型要调用工具就在本地执行，把结果交回去，直到它用文字作答或达到步数上限。工具就是上面的 MCP 工具集，在进程内直接连接（不经过网络和 Token），所以外部 AI 和内置助手看到的是同一套能力和同一条规则：收录判断与核实仍由人在详情页决定。

模型服务在 **设置 · AI 模型** 配置，按接口协议而不是厂商区分：

- **Anthropic Messages**：`POST {地址}/v1/messages`，`x-api-key` 认证。默认地址 `https://api.anthropic.com`，默认模型 `claude-opus-5-5`。
- **OpenAI Chat Completions（兼容）**：`POST {地址}/chat/completions`，Bearer 认证。默认地址 `https://api.openai.com/v1`；DeepSeek、通义千问、Ollama（`http://localhost:11434/v1`）、vLLM 等填各自地址和模型名即可。

还可以设置**思考强度**（low / medium / high / xhigh / max，默认跟随服务；Anthropic 发送 `output_config.effort`，OpenAI 兼容协议发送 `reasoning_effort`，xhigh 与 max 都按 high 发送，老模型不支持时保持“服务默认”）、单次运行的步数上限（默认 20）和单次回复的 token 上限。Key 和地址只保存在本机 `settings.json`，修改后下一次对话生效，无需重启。

对话在每轮回复结束后自动保存到数据库（`agent_conversations` 表，随存放文件夹迁移）；标题栏的「历史对话」弹窗可以重新打开或删除以前的对话，「新对话」另起一段。助手的回复按 Markdown 渲染（标题、列表、代码、表格、链接），模型返回的思考摘要可展开查看；每次工具调用都显示在对话中，可展开查看参数和返回。运行中可以继续输入，发送的消息进入队列，当前一轮结束后按顺序自动发出；排队中的消息可以在队列里直接修改或移除，发送后光标留在输入框。

命令行同样可用：

```bash
BiblioCell settings set -ai-protocol openai -ai-base-url http://localhost:11434/v1 -ai-model qwen3 -ai-effort medium
```

```bash
BiblioCell agent "库里有多少篇还没判断收录的文献？"
```

`agent` 命令把工具调用打印到 stderr，最终结果（完整对话、回复、步数、token 用量）以 JSON 打印到 stdout。

### AI 筛选建议、对照与重跑

逐篇筛选页的「AI 建议」和文献库的「批量 AI 筛选」会按你写的筛选要求让模型判断每篇文献（优先读全文，没有全文读摘要），给出 included / excluded / uncertain 的建议、理由、一段逐字引用的原文和 0～1 的置信度。建议只是建议：采纳与否由你按 Y / N / U 决定，模型永远不写 `screening_status`。

每条建议都记进 `screening_suggestions` 表：生成时间、所用模型配置（设置里的名称 + 模型 id + 协议）、筛选要求、结论、理由、置信度、引用原文及其 PDF 页码、所属批次。文献库顶部的 **AI 对照** 标签把这些建议和你最终的收录判定放在一起：

- **一致率**、已对照 / 待判定篇数，以及 AI 建议 × 人工判定的 3×3 **混淆矩阵**（纳入 / 不纳入 / 证据不足）。
- 可按**模型配置**、**批次**和**时间范围**筛选；不按批次筛选时，同一篇文献同一模型只取最新一条建议，所以重跑会替换旧答案而不是重复计数。
- **按模型配置**和**按批次**两张表分别给出各自的一致率，换模型或改提示词后的差异一眼可见。
- **不一致样本**列出 AI 与人判断不同的文献，点开直接进入逐篇筛选页，建议、理由和模型信息原样显示，退出后回到对照页。

对照用的人工判定永远是当前记录的那一个（`screening_method = 'human'`）；agent 通过 MCP 写的 `proposed_screening` 和仍待判断的文献不参与一致率。

**重跑对照**（对照页右上，或批量筛选弹窗把范围换成「已人工判定的文献」）选一组已经判定过的文献，用当前或指定的模型配置重新生成一批建议。它复用批量筛选的并发逻辑，不会改动任何人工判定；结果带着自己的批次号进入对照页，批次表里能直接比较新旧两批的一致率。

模型引用的原文和「AI 从全文填写」保存的证据都记录所在 PDF 页码（提取时按页读取 PDF 文本层定位，存在 `field_evidence.source_location` 的 `page N`）。筛选页的 AI 建议框和「全文依据」区里点击页码，右侧内嵌 PDF 跳到对应页并把这段原文高亮出来；没有页码的旧证据点「在 PDF 中查找」，由后端在已缓存的 PDF 文本里找出页码作为提示，查看器再在自己的文本层里定位，找不到时跳到该页并提示。

### 查出处：全文检索与可点击的引用

问助手"哪篇文献提到了 X、原话怎么说"，它用 `search_fulltext` 在所有已保存全文里检索（大小写和空白不敏感，整句命中优先于各词分散命中），再用 `read_fulltext` 按页读取命中处核对上下文，然后在回答里写成带出处的引用：

```markdown
Zhang 等[在方法部分说明了建库平台](cite:P-1699…#page=3 "Libraries were prepared with 10x Genomics Chromium.")。
```

链接文字可以是原句，也可以是对这句话的概括或翻译；引号里是全文中逐字出现的一句话。回答里它显示为一个带页码的黄色按钮，点一下，这篇文献的 PDF 就在 AI 助手页右侧打开，跳到第 3 页并把这句话高亮出来（再点别的引用，同一个 PDF 直接跳转，换文献则换文件；Esc 或 × 关闭）。面板标题栏显示定位结果（"第 3 页 · 已高亮原文"，或"已跳到第 3 页，未找到这段原文"），并可打开文献详情或在系统中打开文件。没有 PDF 全文的文献点引用会打开它的"数据与资料"，方便导入。

定位在 PDF.js 的文本层里完成：两边都归一到只剩字母和数字再比较，所以提取工具之间的空格、断行、连字符和连字差异不影响命中；整句找不到时退而用开头 80 / 40 / 20 个字符定位，先看引用的页和相邻页，再看全文。各文件的逐页文本缓存在 `fulltext_text` 表（schema v17），按文件大小和修改时间失效，文献删除或移除文件时一并删除；第一次检索要把所有 PDF 的文本层提取一遍，之后即时返回。扫描件没有文本层，会记为无法读取而不会每次重试。

## 文献跟踪

真实数据库模式下，“文献跟踪”页面可以保存每个来源的启用状态、检索式和单次结果上限（1–100），手动运行同步，也可以开启定时自动更新：

- **PubMed**：NCBI E-utilities 检索与 XML 元数据，保留 PMID、DOI、作者和摘要。
- **Europe PMC**：核心文献元数据检索。
- **Crossref**：书目关键词检索，保存 DOI 和出版信息。
- **bioRxiv**：扫描最近 30 天的预印本，在本地按题目、摘要和分类匹配关键词。
- **GEO / SRA / ENA**：可选择仓库，首次见到的记录新建为“待整理”数据集，保存 accession、标题、物种和官方页面，不自动关联文献。已有数据集持有同一 accession 时跳过。
- **CNCB GSA**：站点会要求浏览器安全检查，因此仅提供官方手动入口，不绕过验证抓取。

来源配置窗口里可以填写提高限额的凭证，都是选填，修改后下一次同步生效：

- **NCBI API Key**：PubMed 和 GEO / SRA 共用，请求上限从每秒 3 次提高到 10 次。ENA 不使用。
- **联系邮箱**：在 Crossref 配置中填写。作为 `mailto` 发送给 Crossref，进入 polite 池；Unpaywall 查询全文时也要用它。
- Europe PMC、bioRxiv、ENA 未接入凭证。

凭证保存在本机的 `settings.json`，不写入数据库，不随存放文件夹迁移，也不会通过 MCP 或 `sql` 读到。命令行用 `settings set -ncbi-api-key KEY -contact-email EMAIL` 设置。

### 网络代理

AI 模型和文献源的联网请求默认跟随系统代理：读取 macOS / Windows / Linux 系统设置里的代理（含 PAC）；设置了 `HTTP_PROXY`、`HTTPS_PROXY` 等环境变量时以环境变量为准。设置页“网络代理”可以改为不使用代理（直连），或自定义代理地址（`http://`、`https://`、`socks5://`，可带 `用户名:密码@`）。每个 AI 模型和文献源还有一个“代理”选项：默认跟随全局设置，也可以单独设为直连，例如本机或内网的模型服务。本机地址（localhost、127.0.0.1）始终直连。保存后下一次请求即生效，无需重启。命令行用 `settings set -proxy-mode system|direct|custom -proxy-url URL`、`settings set -ai-proxy inherit|direct` 和 `sources set ID -proxy inherit|direct` 设置。

补全期刊、作者、年份等书目信息：桌面端在文献表单里按 DOI 获取；agent 用 MCP 的 `lookup_metadata`（`doi` 或 `pmid` 二选一），命令行用 `papers lookup DOI|PMID`，纯数字按 PMID 处理，其余按 DOI。DOI 先查 Crossref，再用 Europe PMC 补 PMID 和摘要；PMID 先查 Europe PMC，记录里有 DOI 时再用 Crossref 的出版社记录。查询只返回结果，要保存请用 `update_paper` 或 `papers update`；已筛选的文献只会补上还空着的书目字段，已有的值不会被覆盖。

预印本正式发表后可以合并掉：

1. **找重复**：`find_duplicates`（`papers duplicates`）按题目匹配键分组，忽略大小写、标点和标记，只读本地库；每组给出建议保留的一篇，优先已发表，其次已人工筛选，再次最早加入。题目在发表时改过的，用 `find_published_versions`（`papers published`）逐篇问 Crossref 预印本的 `is-preprint-of` 关系，返回期刊版 DOI，以及期刊版是否已在库里。期刊版不在库里时，先 `lookup_metadata` 再 `add_paper`。
2. **合并**：`merge_papers`（`papers merge 保留ID 合并ID -reason 理由`），`keepId` 一般是期刊版，`mergeId` 是预印本，必须写理由。合并在一个事务里完成：
   - 附件、数据集关联、证据、AI 建议和来源记录移到保留的文献；同一数据集关联两边都有时，只要有一边核实过就算核实。
   - 保留文献的书目信息不变，只有空着的摘要和分类字段（文章类型、物种、器官、技术、平台）用预印本的值补上。
   - 笔记和标签合并，星标、阅读进度取两者中更进一步的。
   - 人工筛选结果：只有一篇有时沿用那一篇；两篇都有而且结论不同时拒绝合并，要先在桌面端统一。
3. **不会再回来**：被合并文献的原 ID、题目、DOI、PMID 和理由记在 `merged_papers` 表（schema v18），`list_merged_papers`（`papers merged ID`）可查。之后来源更新或导入再遇到这个 DOI、PMID 或 ID，都会认作保留的文献，不会把预印本重新加回来，也不会把预印本的标识符写到保留的文献上。

合并会删除预印本，不能撤销。

文献详情的“资料”页可以点“获取开放全文”，agent 用 MCP 的 `fetch_fulltext`，命令行用 `papers fetch ID`。按 DOI 或 PMID 依次尝试：

1. **PMC 开放获取子集**：经 Europe PMC 查到 PMCID，再从 NCBI 公开的 S3 存储桶下载。
2. **Unpaywall**：遍历它列出的开放 PDF 地址，需要联系邮箱。

只接受 https 链接和真正的 PDF，单个文件上限 200 MiB，保存时记录来源地址和许可证。已有全文的文献不会重复下载。Europe PMC 和 PMC 网页的 PDF 入口有人机验证，出版社网站也可能拦截，这些情况都按失败处理，不会绕过；订阅内容和 Sci-Hub 一类渠道不接入，需要时手动导入。

自动更新默认关闭，此时只有点击“同步”才会访问远程公开 API。开启后，桌面应用运行期间每分钟检查一次，按设定频率逐个更新已启用的来源：

- 每个来源从自己最近一次运行起算，手动同步也算一次；应用关闭期间错过的更新在下次启动时补跑。失败的来源最多一小时后重试。
- 同一时间只运行一次更新。手动、定时和命令行的更新都在数据库里抢同一个运行名额，抢不到时立即返回“正在运行”，不排队。
- 页面显示上次和下次更新时间、各来源最近一次运行的新增与跳过合计，以及失败原因。设置保存在数据库中，随存放文件夹一起移动。`serve` 模式不运行定时更新。

去重在写入事务里完成，手动同步、自动更新和 agent 导入共用同一套规则，按顺序匹配：

1. **见过的外部记录**：同一来源的同一外部 ID 始终归到首次匹配的文献。所以人工改过题目或 DOI、撤销导入删除过的文献，都不会被重新抓回来。
2. **DOI**：比较前统一转小写，去掉 `https://doi.org/`、`doi:` 等前缀和末尾标点。
3. **PMID**：去掉 `PMID:` 标签和 PubMed 链接。
4. **题目**：只比较小写字母和数字，忽略标点、连字符和标记。题目只在缺标识符时兜底：DOI 或 PMID 不同的、年份相差超过一年的都算两篇，所以预印本和正式发表版各占一条。

规范化后的 DOI、PMID 在 `paper_identifiers` 上有唯一约束。通过题目匹配到已有文献时，只补它缺的 DOI 或 PMID，便于其他来源直接命中。自动流程不会改动已有文献的元数据、笔记、标签、收录判断和阅读状态；人工判断过的文献连标识符也不补。

来源更新新增的文献和数据集都进入“需核对”：文献库和数据集页各有一个“需核对”标签页（文献库的这一页同时列出 agent 建议排除或建议核对的文献）。文献做出收录判断、数据集修改整理状态后自动移出，也可以在详情里直接点“核查完成”。手动添加、agent 导入的记录不进入；agent 修改记录不会把它移出。

升级到 schema v8 时，已有的 DOI 和 PMID 会改写成规范形式，并补上题目匹配键，不合并也不删除任何文献。如果改写后与另一篇文献冲突，这个写法会移到 `other` 命名空间，两篇都保留，是否为同一篇由人判断。

配置、来源记录快照、每次成功/失败的运行记录和最近成功时间均保存在正式 SQLite 数据库中。Demo 数据不会与同步结果混合。

### 让助手优化检索式

可以直接对 AI 助手说“帮我改进 PubMed 的检索式”。助手按这样的流程做：

1. `list_sources` 读取各来源当前的检索式和检索语法；`list_papers` 看你已纳入和判为不纳入的文献，从中提炼关键词、同义词和应排除的方向。
2. `try_source_query` 先用当前检索式跑一次作为基准，再逐个试候选检索式。试检索只访问远程来源，**不写入任何文献、记录或运行**，返回命中总数（PubMed、Europe PMC、Crossref、GEO、SRA 支持）和取回的每条结果：题目、摘要片段，以及它对应库里哪篇文献、你对它的收录判断。
3. 比较各检索式：取回结果里命中已纳入的多、命中不纳入的少、新结果的题目摘要切题、命中总数在可处理的范围内，就是更好的检索式。
4. 只有候选明显优于基准时，助手才用 `update_source_query` 保存，必须写明理由（例如两次试检索的对比）。来源的启用状态不变，下一次同步起生效。

检索式的每次修改都记入 `source_query_history`（schema v11）：版本号、手动还是助手改的、理由、改前的检索式。来源卡片显示检索式版本和最近一次由谁修改；配置窗口列出全部版本，“填入”可以把任一旧版本放回输入框，保存后即恢复，记为手动修改。每次同步运行记录当时的检索式版本。

## 网页版：内网部署

同一个可执行文件可以作为网站运行，让内网同事在浏览器里查文献库。它不开窗口、不需要桌面环境，Linux 服务器上用 `CGO_ENABLED=0` 交叉编译即可运行：

```bash
BIBLIOCELL_ADMIN_PASSWORD=change-me ./BiblioCell web -addr 0.0.0.0:8080
```

页面和桌面应用是同一套前端，角色由服务端决定：

- **所有访问者（不登录）只读**：浏览总览、文献库、数据集、待补事项、详情抽屉和已保存的 PDF，可以筛选、排序、导出。看不到“文献跟踪”和“AI 助手”，设置页只剩界面语言和登录入口。详情里的星标、收录判断、笔记等控件不可操作。
- **管理员**：顶栏“管理员登录”输入密码后，页面以管理员身份重新加载，和桌面端的真实数据库模式一样：运行与配置来源更新、开关定时更新、编辑记录、做收录判断、核实证据与关联、批量 AI 筛选、AI 助手，以及设置里的 **AI 模型 / API 配置**。密码来自 `BIBLIOCELL_ADMIN_PASSWORD` 环境变量或 `-admin-password-file FILE`；两者都不设时所有人只读，没有登录入口。
- 定时自动更新在服务器上照常运行（见“文献跟踪”），所以“数据库更新只有管理员能看到”的同时，更新本身不依赖桌面应用在线。
- 桌面才有的功能在网页版没有：导入本地文件（管理员可用“获取开放全文”，或在桌面应用里导入后把存放文件夹放到服务器）、移除文件、选择存放文件夹、MCP 端点、应用自动更新。导出在浏览器中直接下载。

权限在服务端检查，前端隐藏控件只是为了界面干净：

- 页面通过 `POST /api/call` 调用 Library 方法；`web.go` 里的 `webMethodAccess` 把每个方法标为 **只读** / **管理员** / **仅桌面**，新增方法必须归类，`TestWebMethodsAreClassified` 会检查。匿名调用管理员方法返回 403，仅桌面的方法返回 404。
- 匿名读取 `GetSettings`、`Status`、`MCPInfo` 时，API Key、NCBI Key、邮箱、MCP Token 和服务器路径都被去掉；管理员能看到并修改 AI 模型，但存放文件夹和 MCP Token 在网页上改不了（请在服务器上 `settings set`）。
- 登录后用 HttpOnly、`SameSite=Strict` 的 cookie 保存会话，内存中保存 7 天，重启服务即全部登出；`/api/*` 拒绝浏览器标为跨站的请求；同一地址 15 分钟内密码错 10 次后暂停登录。
- 管理员的事件（后台更新进度、助手的工具调用）经 `/api/events` 以 SSE 推送；PDF 由 `/paperfile/<资源 ID>` 提供，只认库里登记的文件。
- 服务默认监听 `127.0.0.1:8080`，`-addr 0.0.0.0:8080` 才对局域网开放。HTTP 是明文，管理员密码和 API Key 都会经过网络，建议放在 nginx / Caddy 之类的反向代理后面开启 HTTPS（代理传 `X-Forwarded-Proto: https` 时 cookie 自动加 `Secure`）。不要暴露到公网。

构建与部署：

```bash
npm run build:server
```

等于 `npm run build:web`（生成 `dist/`）加 `go build -tags webdist -o build/bibliocell-web .`：`-tags webdist` 把 `dist/` 嵌进二进制，一个文件就能部署。不带这个 tag 时，`web` 读 `./dist`，或用 `-dist DIR` 指定。交叉编译 Linux 服务器版本：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags webdist -o bibliocell-web .
```

也可以用仓库里的 `Dockerfile`：

```bash
docker build -t bibliocell-web .
```

```bash
docker run -d -p 8080:8080 -v bibliocell-data:/data -e BIBLIOCELL_ADMIN_PASSWORD=change-me bibliocell-web
```

容器内数据目录是 `/data`（`BIBLIOCELL_DATA_DIR`），settings.json、`bibliocell.sqlite` 和 `papers/` 都在里面；把桌面应用的存放文件夹复制进去即可带上已有文献和 PDF。开发时 `npm run build:web` 后 `go run . web -addr 127.0.0.1:8090` 就能在浏览器里打开。

## 外部 AI：MCP

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
| `delete_paper` | 按 `id` 永久删除单篇文献（含已审核记录）、笔记、相关证据和保存的附件副本；保留数据集与原始导入文件，需要写权限 |
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

### MCP 权限

- **无认证**：只读，可调用所有读取工具（`describe_library`、`library_status`、`list_*`、`get_*`），并读取 Schema 资源。
- **Bearer Token 认证**：额外允许标注“需要写权限”的工具。
- 无 Token 的写调用会被拒绝；错误 Token 返回 HTTP 401。
- Token 是应用首次启动时生成的 256 位随机密钥。可以在“设置 → 外部 AI · MCP”中查看、复制、自定义或重新生成；自定义 Token 保存在 `settings.json`，修改后重启生效。Token 不会通过 MCP 暴露。
- `describe_library` 返回当前连接的 `accessRole`（`read-only` 或 `read-write`）和 `canWrite`。

### 人工核查

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

### Agent 工作流

1. `describe_library` 读取规则和 `inboxPath`；一次运行用同一个 `batchId`。
2. `import_papers` 导入文献，已有文献被跳过并列在 `skipped`，可以放心重跑。`add_dataset` 同样可重跑。
3. 把 PDF 放进 `inboxPath`，再用 `attach_file` 挂到文献上。只能读取 `inbox/` 内的文件，`..` 和指向外部的符号链接都会被拒绝。文件移入文库后从 inbox 消失。
4. `add_dataset` / `link_dataset` 记录数据集和关联，`add_evidence` 记录每个字段的摘录和来源，`update_paper` 提交 `proposedScreening`。
5. 人在桌面端核查。agent 用 `list_papers` 的 `needsReview` 和 `list_evidence` 查看进度和判断。
6. 这批导入错了就 `undo_import`：删除未经人处理的文献、数据集和它们的文件副本；人已确认、收藏、阅读或核实过的记录保留并列在 `kept`。

`update_paper` 使用 `id` 和 `patch`。例如只修改期刊时传入 `{"id":"P-...","patch":{"journal":"New journal"}}`，未提供的 DOI、作者、笔记等字段保持不变；`clearYear: true` 可以将年份清空。多值字段（`species`、`organs`、`modalities`、`platforms`、`articleTypes`）以数组整体替换，例如 `{"patch":{"platforms":["10x Genomics Chromium","BD Rhapsody"]}}`，空数组表示清空。

### MCP 自描述与 Schema

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

## 导出

筛选完一批文献后，结果可以从文献库页和数据集页右上角的“导出”按钮导出，也可以用命令行 `export`。桌面应用弹出保存对话框，由你选择位置；Demo 模式导出的是浏览器里的 Demo 数据，真实数据库模式导出的是正式库，两者都不会写入数据库。在线浏览器预览里文件直接下载。总览页“整理覆盖”面板有“导出进度报告”的快捷入口。

文献库页的范围可选“当前筛选结果”“全部已收录”或“全部文献”，数据集页可选“当前搜索结果”或“全部数据集”。格式：

| 格式 | 文件 | 内容 |
|---|---|---|
| `bibtex` | `.bib` | 每篇一个 `@article`，题目加双层花括号保留大小写，`\&`、`\%`、`\_` 等已转义，作者 `A; B` 写成 `A and B`，附 DOI、URL、PMID、物种/组织/测序类型/平台作 `keywords`，预印本加 `note = {Preprint}`。引用键为“第一作者姓 + 年份 + 题目首个长词”，重复时加 a、b 后缀 |
| `papers-csv` | `.csv` | 每行一篇：`id, title, authors, journal, year, doi, pmid, publication_status, species, species_label, organs, modalities, platforms, article_types, screening, screening_reason, fulltext, reading, tags, dataset_accessions, dataset_ids, source, added_at`。多值字段用 `; ` 连接 |
| `report` | `.md` | 筛选进度报告：纳入 / 不纳入 / 证据不足 / 待判断数量、待处理的 agent 建议、来源更新新增未核查数、已收录文献的全文覆盖；数据集整理状态与可用性；按物种、测序类型、组织的数据集数（含已核实、公开）和已收录文献数；最后列出已收录文献的题目、年份、DOI 和 accession。`-title` 自定义标题 |
| `datasets-csv` | `.csv` | 每行一个数据集：`accessions`、`repositories`、`accession_kinds`（如 `geo/series; sra/run`）、`verified_accessions`、整理状态、可用性、细胞数、关联文献 DOI / ID / 角色 |
| `datasets-summary` | `.csv` | 按 `-group-by species|organ|modality` 聚合：数据集数、已核实数、公开数、关联文献数、accession 数。多物种的数据集在每个物种下各计一次，没填的记为 `(unspecified)` |
| `download-script` | `.sh` | 批量下载脚本。按 accession 前缀分类：GEO 系列用 wget 取 `suppl/` 和 SOFT family 文件；SRA 的 SRP/SRX/SRS/SRR 用 `prefetch` + `fasterq-dump`；ENA / DDBJ 的 ERR/ERP/PRJEB 等从 ENA 文件报告取 fastq 链接；PRJNA 用 entrez-direct 展开成 SRR 再 prefetch；GSA、BioStudies、Zenodo 等没有批量工具的列为 `manual`，附页面地址。每条命令上方注释数据集题目和关联文献 DOI。应用只生成文件，不运行；`OUT=/data/scrna bash xxx.sh` 指定目标目录 |
| `accessions` | `.txt` | 每行一个 accession，去重，可直接 `xargs prefetch` 或交给其他工具 |

CSV 带 UTF-8 BOM 和 CRLF，Excel 直接打开；以 `=`、`+`、`-`、`@` 开头的单元格前面加 `'`，防止被当公式执行。accession 的识别以前缀为准：记成 GEO 但编号是 `SRR…` 的按 SRA 处理，前缀不认识的沿用记录的仓库名。

命令行的 `-q`、`-screening` 筛选文献（和 `papers list` 一样），`-status` 筛选数据集；数据集格式把 `-q` 用在数据集上。

```bash
./BiblioCell export report -title "Week 44" -o report.md
./BiblioCell export download-script -status verified > download.sh
./BiblioCell export datasets-summary -group-by modality
```

示例：

```bibtex
@article{zhang2024singlecell,
  title = {{Single-cell atlas of Arabidopsis \& rice roots}},
  author = {Zhang, Wei and O'Brien, Ann},
  journal = {Nature Plants},
  year = {2024},
  doi = {10.1000/abc\_1},
  url = {https://doi.org/10.1000/abc\_1},
  keywords = {Arabidopsis thaliana, root, scRNA-seq, 10x Genomics Chromium},
}
```

```bash
# ---- GEO (series) ----
# Root atlas data (doi: 10.1000/abc_1)
geo_series GSE123456 'https://ftp.ncbi.nlm.nih.gov/geo/series/GSE123nnn/GSE123456/'

# ---- SRA (prefetch + fasterq-dump) ----
# Root atlas data (doi: 10.1000/abc_1)
sra SRP111111
```

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
./BiblioCell papers files P-xxxx
./BiblioCell papers list -needs-review -batch run-1 -limit 100 -offset 100
./BiblioCell papers undo run-1

# 预印本正式发表后：找重复、问 Crossref、合并（期刊版在前，预印本在后）
./BiblioCell papers duplicates
./BiblioCell papers published -limit 50
./BiblioCell papers merge P-journal P-preprint -reason "Crossref is-preprint-of 10.1016/j.cell.2019.05.031"
./BiblioCell papers merged P-journal

# 永久删除单篇文献，成功返回 {"id":"P-...","deleted":true}
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
- `export KIND` 把文献或数据集写成文件，见上文“导出”；不带 `-o` 时内容直接打到 stdout，带 `-o` 时输出写入路径和记录数的 JSON。
- `sql` 直接执行任意语句：`SELECT` / `WITH` / `PRAGMA` 打印行，其他语句打印受影响行数。它绕过应用层校验，改数据前先备份。
- `db version` 只读取数据库文件的版本、已应用和待应用的迁移，不打开、不创建、不修改库；`db migrate` 把库升级到当前版本（先备份），其他子命令和桌面应用在打开库时也会自动做同样的事。见「数据库版本与迁移」。
- 数据目录按 `-data-dir`、`$BIBLIOCELL_DATA_DIR`、应用默认用户数据目录的顺序确定；在 macOS 上 `.app` 包内和 `go run` 的默认目录名相同，命令行与桌面应用看到的是同一个库。
- `settings set` 修改的存放文件夹和 MCP Token 在下次启动桌面应用后生效。
- 桌面应用运行中也可以同时使用命令行，SQLite 自带锁，忙等 5 秒后报错。

## 目录

```text
BiblioCell/
├── main.go                  # mygo 应用、窗口、SQLite 与 MCP 生命周期
├── service.go               # 暴露给前端的 Library 服务
├── cli.go                   # 命令行子命令：文献、来源、设置、原始 SQL
├── web.go                   # 网页版：HTTP 调用分发、方法权限表、管理员会话、事件流、静态文件
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
├── mygo.config.ts           # mygo 开发、构建和应用元数据
├── Dockerfile               # 网页版服务器镜像
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

## 数据库版本与迁移

数据库结构的版本写在 SQLite 的 `PRAGMA user_version` 里，每一步升级在 `schema_migrations` 表留一行（版本号、说明、时间）。当前版本是 `library.SchemaVersion`，由 `internal/library/migrate.go` 里的基线版本加迁移条数算出；`./BiblioCell db version` 可以查看某个库处在哪个版本、还差哪些迁移。

打开数据库（桌面应用启动、任何命令行子命令、`db migrate`）时自动升级，流程固定：

1. 读取 `user_version`。比当前构建还新的库直接拒绝打开，提示更新 BiblioCell。
2. 库落后时，先用 `VACUUM INTO` 把整个文件复制为同目录下的 `bibliocell.sqlite.bak-v<旧版本>`，再动原库。出问题时把备份复制回原文件名即可恢复。
3. 低于基线（v13）的库，比如 v0.1.0 发布版的 v11，先经过 `store.go` 里按实际表结构修复的旧路径（补列、回填 JSON 多值字段、重建仓库名称约束、重建视图），再盖上基线版本号。
4. 基线之后的迁移按编号顺序逐条执行。每条迁移在自己的事务里完成：执行 SQL、执行 Go 函数、写入 `schema_migrations`、更新 `user_version`，然后提交。失败即回滚这一条，库停在上一条完成的版本，错误信息写明失败的迁移、库停在的版本和备份路径；修好后再次打开会从停下的地方继续。
5. 最后执行幂等的 `schema.sql`（新库在这一步一次建成；升级过的库在这里重建迁移中 DROP 掉的视图），并由 Go 盖上当前版本号。

### 新增一条迁移

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

## 发布

版本号只写在 `package.json` 的 `version` 里，`mygo.config.ts` 从那里读取。发布一个版本：

```bash
npm version 0.1.0 --no-git-tag-version
```

```bash
git commit -am "release: v0.1.0" && git tag v0.1.0 && git push origin main v0.1.0
```

推送 `v*` 标签会触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)：先核对标签与 `package.json` 版本一致、签名 secrets 齐全，然后在 macOS runner 上 `mygo build -platform darwin/universal`（Developer ID 签名 + Apple 公证），在 Windows runner 上 `mygo build -platform windows/amd64`（SSL.com eSigner 签 exe、卸载程序与安装包），两边都通过后创建 GitHub Release，上传 DMG、安装包、自动更新文件和 `SHA256SUMS`。自动更新文件由 mygo 写出：每个平台一个签名的更新包 `bibliocell-<版本>-<平台>.tar.gz`、从已发布版本出发的增量包 `.delta`，以及清单 `update-<平台>.json`。已安装的应用读最新 Release 里的清单，只接受用 `mygo.config.ts` 里 `updates.publicKey` 对应私钥签名的更新。`.github/release-notes/<tag>.md` 存在时作为发布说明，后面附上自动生成的变更列表。

签名只在设置了环境变量时启用（见 `mygo.config.ts` 顶部注释），本机 `npm run build` 仍是 ad hoc 签名、不公证，可以直接运行但 Gatekeeper 会拦截其他 Mac。仓库需要这些 secrets，与 citebox / wisp-science 同名，可复用同一套证书：

| Secret | 用途 |
|---|---|
| `MAC_CERT_P12_BASE64`、`MAC_CERT_PASSWORD` | Developer ID Application 证书（含私钥）的 base64 与密码 |
| `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD` | 公证用的 Apple ID、Team ID 与 app 专用密码 |
| `ESIGNER_CREDENTIAL_ID`、`ESIGNER_USERNAME`、`ESIGNER_PASSWORD`、`ESIGNER_TOTP_SECRET`、`ESIGNER_PIN` | SSL.com eSigner |
| `MYGO_UPDATER_PRIVATE_KEY` | 自动更新的签名私钥，即 `mygo keygen` 生成的 `mygo-update.key`，BiblioCell 专用。丢失后已安装的应用无法再自动更新，泄露则任何人都能向用户推送代码，请另存一份到密码管理器 |

手动运行该 workflow（workflow_dispatch）可以对已有标签重新构建；不勾选 publish 时只把签好名的安装包留作 artifact，不碰 Release。
