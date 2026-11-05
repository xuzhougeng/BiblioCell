# 网页版：内网部署

同一个可执行文件可以作为网站运行，让内网同事在浏览器里查文献库。它不开窗口、不需要桌面环境，Linux 服务器上用 `CGO_ENABLED=0` 交叉编译即可运行：

```bash
BIBLIOCELL_ADMIN_PASSWORD=change-me ./BiblioCell web -addr 0.0.0.0:8080
```

页面和桌面应用是同一套前端，角色由服务端决定：

- **所有访问者（不登录）只读**：浏览总览、文献库、数据集、待补事项、详情抽屉和已保存的 PDF，可以筛选、排序、导出。看不到“文献跟踪”和“AI 助手”，设置页只剩界面语言和登录入口。详情里的星标、收录判断、笔记等控件不可操作。
- **管理员**：顶栏“管理员登录”输入密码后，页面以管理员身份重新加载，和桌面端的真实数据库模式一样：运行与配置来源更新、开关定时更新、编辑记录、做收录判断、核实证据与关联、批量 AI 筛选、AI 助手，以及设置里的 **AI 模型 / API 配置**。密码来自 `BIBLIOCELL_ADMIN_PASSWORD` 环境变量或 `-admin-password-file FILE`；两者都不设时所有人只读，没有登录入口。
- 定时自动更新在服务器上照常运行（见[文献跟踪](literature-tracking.md#自动更新)），所以“数据库更新只有管理员能看到”的同时，更新本身不依赖桌面应用在线。
- 桌面才有的功能在网页版没有：导入本地文件（管理员可用“获取开放全文”，或在桌面应用里导入后把存放文件夹放到服务器）、移除文件、选择存放文件夹、MCP 端点、应用自动更新。导出在浏览器中直接下载。

权限在服务端检查，前端隐藏控件只是为了界面干净：

- 页面通过 `POST /api/call` 调用 Library 方法；`cmd/bibliocell/web.go` 里的 `webMethodAccess` 把每个方法标为 **只读** / **管理员** / **仅桌面**，新增方法必须归类，`TestWebMethodsAreClassified` 会检查。匿名调用管理员方法返回 403，仅桌面的方法返回 404。
- 匿名读取 `GetSettings`、`Status`、`MCPInfo` 时，API Key、NCBI Key、邮箱、MCP Token 和服务器路径都被去掉；管理员能看到并修改 AI 模型，但存放文件夹和 MCP Token 在网页上改不了（请在服务器上 `settings set`）。
- 登录后用 HttpOnly、`SameSite=Strict` 的 cookie 保存会话，内存中保存 7 天，重启服务即全部登出；`/api/*` 拒绝浏览器标为跨站的请求；同一地址 15 分钟内密码错 10 次后暂停登录。
- 管理员的事件（后台更新进度、助手的工具调用）经 `/api/events` 以 SSE 推送；PDF 由 `/paperfile/<资源 ID>` 提供，只认库里登记的文件。
- 服务默认监听 `127.0.0.1:8080`，`-addr 0.0.0.0:8080` 才对局域网开放。HTTP 是明文，管理员密码和 API Key 都会经过网络，建议放在 nginx / Caddy 之类的反向代理后面开启 HTTPS（代理传 `X-Forwarded-Proto: https` 时 cookie 自动加 `Secure`）。不要暴露到公网。

构建与部署：

```bash
npm run build:server
```

等于 `npm run build:web`（生成 `dist/`）加 `go build -tags webdist -o build/bibliocell-web ./cmd/bibliocell`：`-tags webdist` 让根目录的 `assets_webdist.go` 把 `dist/` 嵌进二进制，一个文件就能部署。不带这个 tag 时，`web` 读 `./dist`，或用 `-dist DIR` 指定。交叉编译 Linux 服务器版本：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags webdist -o bibliocell-web ./cmd/bibliocell
```

也可以用仓库里的 `Dockerfile`：

```bash
docker build -t bibliocell-web .
```

```bash
docker run -d -p 8080:8080 -v bibliocell-data:/data -e BIBLIOCELL_ADMIN_PASSWORD=change-me bibliocell-web
```

容器内数据目录是 `/data`（`BIBLIOCELL_DATA_DIR`），settings.json、`bibliocell.sqlite` 和 `papers/` 都在里面；把桌面应用的存放文件夹复制进去即可带上已有文献和 PDF。开发时 `npm run build:web` 后 `go run ./cmd/bibliocell web -addr 127.0.0.1:8090` 就能在浏览器里打开。

---

[← 返回文档目录](README.md)
