# 设置

左下角 **设置** 页面提供：

- **Demo 演示模式**：切换 Demo 与真实数据库，切换时重新加载页面，不复制或合并数据。
- **数据存放位置**：选择一个文件夹，SQLite 数据库（`bibliocell.sqlite`）和文献文件（`papers/`）都保存在里面。通过原生目录对话框选定后立即切换；也可输入绝对路径，点“保存设置”后切换。切换前先试打开新文件夹，打不开就报错，设置和当前数据库都不变；打得开就自动重启过去。应用不会移动或覆盖旧数据，所选文件夹没有数据库时会创建空库。
- **界面语言**：简体中文 / English，也可点顶栏右侧的 EN / 中 一键切换。首次打开按系统语言选择，之后记住你的选择（保存在本机）；切换时重新加载页面。
- **界面字号**：12–20px，拖动时实时预览，保存后跨重启保留。缩放覆盖导航、表格、详情抽屉和笔记等固定字号，不改变面板宽度。
- **外部 AI · MCP**：集中显示连接地址、工具和 Schema 资源，可查看、复制、自定义或重新生成 Bearer Token。Token 修改在重启后生效。
- **AI 模型**：内置助手使用的模型服务，见[内置 AI 助手](ai-assistant.md)。
- 桌面左侧边栏可折叠为图标栏，折叠状态保存在本机；移动端使用抽屉式导航。

设置保存在 mygo 用户数据目录的 `settings.json`。Windows 默认配置文件为 `%APPDATA%\BiblioCell\settings.json`，默认存放文件夹为同一目录。页面分别显示配置的文件夹和本次运行实际使用的文件夹。

存放文件夹可以整体迁移：退出应用后把它复制或移动到其他位置（包括其他电脑），在设置中选择新位置即可继续使用。数据库内不保存文件夹位置，附件的 `resources.local_path` 只存相对存放文件夹的路径，读取时拒绝任何跳出该文件夹的路径。字号、MCP Token 和来源凭证属于本机偏好，保存在 `settings.json`，不随文件夹迁移。

旧版设置中的 `databasePath` 会在启动时自动转换为存放文件夹。如果数据库用了其他文件名，会在原文件夹内改名为 `bibliocell.sqlite`；同名文件已存在时不覆盖，保留旧设置并改用默认文件夹，需要手动处理。

从旧版 PlantCell Library 升级时，应用会在首次启动时自动把 `%APPDATA%\PlantCell Library` 中的 `settings.json`、`mcp-token` 和 `plantcell.sqlite` 一次性迁入 `%APPDATA%\BiblioCell`，默认数据库同时更名为 `bibliocell.sqlite`。自定义位置的数据库留在原文件夹，不会被移走。

## 网络代理

AI 模型和文献源的联网请求默认跟随系统代理：读取 macOS / Windows / Linux 系统设置里的代理（含 PAC）；设置了 `HTTP_PROXY`、`HTTPS_PROXY` 等环境变量时以环境变量为准。设置页“网络代理”可以改为不使用代理（直连），或自定义代理地址（`http://`、`https://`、`socks5://`，可带 `用户名:密码@`）。每个 AI 模型和文献源还有一个“代理”选项：默认跟随全局设置，也可以单独设为直连，例如本机或内网的模型服务。本机地址（localhost、127.0.0.1）始终直连。保存后下一次请求即生效，无需重启。命令行用 `settings set -proxy-mode system|direct|custom -proxy-url URL`、`settings set -ai-proxy inherit|direct` 和 `sources set ID -proxy inherit|direct` 设置。

文献源凭证（NCBI API Key、联系邮箱）见[文献跟踪](literature-tracking.md#文献跟踪)。

---

[← 返回文档目录](README.md)
