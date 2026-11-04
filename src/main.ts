import "./style.css";

import { isMyGo } from "mygo-runtime";
import { Library } from "./mygo.ts";
import { App, type Env } from "./app.ts";
import { demoState } from "./demo-data.ts";
import { applyFontSize, initShell } from "./shell.ts";
import { localStore, sqliteStore } from "./store.ts";
import { applyStaticText, t } from "./i18n.ts";
import { detectWeb, installWebRuntime, type WebSession } from "./web-runtime.ts";

const badge = document.querySelector<HTMLSpanElement>("#runtime-badge");
const message = document.querySelector<HTMLSpanElement>("#runtime-message");
const setRuntime = (badgeText: string, messageText: string) => {
  if (badge) badge.textContent = badgeText;
  if (message) message.textContent = messageText;
};

const previewEnv = (): Env => ({
  desktop: false, backend: false, readOnly: false, web: null, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" }, quitOnWindowClose: false, sourceCheck: { enabled: false, prompt: "" }, proxy: { mode: "", url: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

/** startDemo loads the browser-only library; a broken localStorage still shows the seeds plus a warning. */
async function startDemo(env: Env): Promise<void> {
  const store = localStore(localStorage);
  let warning = "";
  const initial = await store.load().catch(() => {
    warning = t("本地存储暂不可用，操作后请导出 JSON 备份。");
    return demoState();
  });
  new App({ store, env, initial, warning }).start();
}

/** startWeb opens the real database the web server fronts: read-only for a viewer, editable for an admin. */
async function startWeb(session: WebSession): Promise<void> {
  installWebRuntime();
  const admin = session.admin;
  const [status, settings] = await Promise.all([Library.status(), Library.getSettings()]);
  // Sources and the MCP token are the admin's; a viewer's page never asks.
  const [sources, automation, mcp] = admin
    ? await Promise.all([Library.listSources(), Library.sourceAutomation(), Library.mcpInfo()])
    : [[], previewEnv().automation, previewEnv().mcp];
  applyFontSize(settings.fontSize);
  const env: Env = { desktop: false, backend: true, readOnly: !admin, web: session, real: true, settings, mcp, status, sources, automation };
  const initial = await sqliteStore.load();
  setRuntime(admin ? t("在线 · 管理员") : t("在线 · 只读"), admin ? t("正在修改服务器上的正式数据库；来源更新和 AI 模型配置都在这里。") : t("浏览服务器上的正式数据库；登录管理员后可以更新和配置。"));
  new App({ store: sqliteStore, env, initial }).start();
}

async function boot(): Promise<void> {
  applyStaticText();
  initShell();
  if (!isMyGo()) {
    applyFontSize(15);
    const session = await detectWeb();
    if (session) {
      try {
        await startWeb(session);
      } catch (error) {
        setRuntime(t("无法连接服务器"), error instanceof Error ? error.message : t("请刷新页面重试。"));
        const content = document.querySelector<HTMLDivElement>("#app-content");
        if (content) content.innerHTML = `<div class="empty-state"><h2>${t("无法连接服务器")}</h2><p>${t("请刷新页面重试。")}</p></div>`;
      }
      return;
    }
    // 浏览器预览（在线演示）顶栏显示仓库入口；桌面应用本身就在仓库里运行。
    document.getElementById("repo-link")?.removeAttribute("hidden");
    setRuntime(t("Demo · 浏览器预览"), t("当前为虚构演示数据；打开桌面应用后可切换到真实 SQLite 数据库。"));
    await startDemo(previewEnv());
    return;
  }

  const realMode = localStorage.getItem("bibliocell-demo-mode") === "false";
  try {
    const [status, mcp, settings, sources, automation] = await Promise.all([Library.status(), Library.mcpInfo(), Library.getSettings(), Library.listSources(), Library.sourceAutomation()]);
    applyFontSize(settings.fontSize);
    const env: Env = { desktop: true, backend: true, readOnly: false, web: null, real: realMode, settings, mcp, status, sources, automation };
    if (realMode) {
      const initial = await sqliteStore.load();
      setRuntime(t("真实数据库"), t("文献与数据集来自本机 SQLite；MCP 连接信息见设置页。"));
      new App({ store: sqliteStore, env, initial }).start();
    } else {
      setRuntime(t("Demo 模式"), t("虚构示例仅保存在浏览器，不会写入正式库（现有 {n} 篇）。", { n: status.paperCount }));
      await startDemo(env);
    }
  } catch (error) {
    setRuntime(t("后端连接失败"), error instanceof Error ? error.message : t("无法读取 Go 后端状态。"));
    if (!realMode) {
      await startDemo(previewEnv());
      return;
    }
    const content = document.querySelector<HTMLDivElement>("#app-content");
    if (content) {
      content.innerHTML = `<div class="empty-state"><h2>${t("无法打开真实数据库")}</h2><p>${t("请重新启动应用，或返回 Demo 模式。")}</p><button class="btn btn-primary" id="fallback-demo-mode">${t("返回 Demo 模式")}</button></div>`;
      document.querySelector<HTMLButtonElement>("#fallback-demo-mode")?.addEventListener("click", () => {
        localStorage.setItem("bibliocell-demo-mode", "true");
        location.reload();
      });
    }
  }
}

await boot();
