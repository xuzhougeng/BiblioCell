import "./style.css";

import { isMyGo } from "mygo-runtime";
import { Library } from "./mygo.ts";
import { App, type Env } from "./app.ts";
import { demoState } from "./demo-data.ts";
import { applyFontSize, initShell } from "./shell.ts";
import { localStore, sqliteStore } from "./store.ts";

const badge = document.querySelector<HTMLSpanElement>("#runtime-badge");
const message = document.querySelector<HTMLSpanElement>("#runtime-message");
const setRuntime = (badgeText: string, messageText: string) => {
  if (badge) badge.textContent = badgeText;
  if (message) message.textContent = messageText;
};

const previewEnv = (): Env => ({
  desktop: false, real: false,
  settings: { storagePath: "", fontSize: 15, mcpToken: "", ncbiApiKey: "", contactEmail: "", ai: { protocol: "", baseUrl: "", apiKey: "", model: "", maxTokens: 0, maxSteps: 0 } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

/** startDemo loads the browser-only library; a broken localStorage still shows the seeds plus a warning. */
async function startDemo(env: Env): Promise<void> {
  const store = localStore(localStorage);
  let warning = "";
  const initial = await store.load().catch(() => {
    warning = "本地存储暂不可用，操作后请导出 JSON 备份。";
    return demoState();
  });
  new App({ store, env, initial, warning }).start();
}

async function boot(): Promise<void> {
  initShell();
  if (!isMyGo()) {
    applyFontSize(15);
    // 浏览器预览（在线演示）顶栏显示仓库入口；桌面应用本身就在仓库里运行。
    document.getElementById("repo-link")?.removeAttribute("hidden");
    setRuntime("Demo · 浏览器预览", "当前为虚构演示数据；打开桌面应用后可切换到真实 SQLite 数据库。");
    await startDemo(previewEnv());
    return;
  }

  const realMode = localStorage.getItem("bibliocell-demo-mode") === "false";
  try {
    const [status, mcp, settings, sources, automation] = await Promise.all([Library.status(), Library.mcpInfo(), Library.getSettings(), Library.listSources(), Library.sourceAutomation()]);
    applyFontSize(settings.fontSize);
    const env: Env = { desktop: true, real: realMode, settings, mcp, status, sources, automation };
    if (realMode) {
      const initial = await sqliteStore.load();
      setRuntime("真实数据库", "文献与数据集来自本机 SQLite；MCP 连接信息见设置页。");
      new App({ store: sqliteStore, env, initial }).start();
    } else {
      setRuntime("Demo 模式", `虚构示例仅保存在浏览器，不会写入正式库（现有 ${status.paperCount} 篇）。`);
      await startDemo(env);
    }
  } catch (error) {
    setRuntime("后端连接失败", error instanceof Error ? error.message : "无法读取 Go 后端状态。");
    if (!realMode) {
      await startDemo(previewEnv());
      return;
    }
    const content = document.querySelector<HTMLDivElement>("#app-content");
    if (content) {
      content.innerHTML = '<div class="empty-state"><h2>无法打开真实数据库</h2><p>请重新启动应用，或返回 Demo 模式。</p><button class="btn btn-primary" id="fallback-demo-mode">返回 Demo 模式</button></div>';
      document.querySelector<HTMLButtonElement>("#fallback-demo-mode")?.addEventListener("click", () => {
        localStorage.setItem("bibliocell-demo-mode", "true");
        location.reload();
      });
    }
  }
}

await boot();
