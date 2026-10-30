import "./style.css";
import "./legacy/styles.css";
import "./legacy/components.css";
import "./legacy/demo-data.js";

import { isMyGo } from "mygo-runtime";
import { Library, type Config, type Dataset, type Paper, type SourceConfig } from "./mygo";

const badge = document.querySelector<HTMLSpanElement>("#runtime-badge");
const message = document.querySelector<HTMLSpanElement>("#runtime-message");
const demoPreferred = localStorage.getItem("bibliocell-demo-mode") !== "false";

function applyFontSize(value: number): void {
  const fontSize = Math.min(20, Math.max(12, Math.round(value) || 15));
  document.documentElement.style.setProperty("--font-scale", String(fontSize / 15));
}

Object.assign(window, { PLANT_APPLY_FONT_SIZE: applyFontSize });

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : "无法读取 Go 后端状态。";
}

async function boot() {
  if (!isMyGo()) {
    applyFontSize(15);
    Object.assign(window, {
      PLANT_DESKTOP: false,
      PLANT_REAL_MODE: false,
      PLANT_SETTINGS: { databasePath: "", fontSize: 15, mcpToken: "" },
    });
    if (badge) badge.textContent = "Demo · 浏览器预览";
    if (message) message.textContent = "当前为虚构演示数据；打开桌面应用后可切换到真实 SQLite 数据库。";
    await import("./legacy/app.js");
    return;
  }

  const realMode = !demoPreferred;
  let saveQueue: Promise<unknown> = Promise.resolve();
  const enqueue = <T>(operation: () => Promise<T>): Promise<T> => {
    const next = saveQueue.catch(() => {}).then(operation);
    saveQueue = next;
    return next;
  };
  Object.assign(window, {
    PLANT_DESKTOP: true,
    PLANT_SAVE_SETTINGS: (next: Config) => Library.saveSettings(next),
    PLANT_CHOOSE_DATABASE_PATH: () => Library.chooseDatabasePath(),
    PLANT_LIST_SOURCES: () => Library.listSources(),
    PLANT_SAVE_SOURCE_CONFIG: (next: SourceConfig) => Library.saveSourceConfig(next),
    PLANT_SYNC_SOURCE: (id: string) => enqueue(() => Library.syncSource(id)),
  });
  try {
    const [status, mcp, settings, sources] = await Promise.all([
      Library.status(),
      Library.mcpInfo(),
      Library.getSettings(),
      Library.listSources(),
    ]);
    const mcpText = mcp.enabled ? `MCP ${mcp.endpoint}` : "MCP 未启动";
    applyFontSize(settings.fontSize);
    Object.assign(window, {
      PLANT_MCP_INFO: mcp,
      PLANT_SETTINGS: settings,
      PLANT_STATUS: status,
      PLANT_SOURCES: sources,
    });

    if (realMode) {
      const [papers, datasets] = await Promise.all([Library.listPapers(), Library.listDatasets()]);
      Object.assign(window, {
        PLANT_REAL_MODE: true,
        PLANT_INITIAL_STATE: { schemaVersion: 1, papers, datasets, resources: [], activities: [] },
        PLANT_SAVE_PAPERS: (next: Paper[]): Promise<void> => enqueue(() => Library.savePapers(next)),
        PLANT_SAVE_DATASETS: (next: Dataset[]): Promise<void> => enqueue(() => Library.saveDatasets(next)),
      });
      if (badge) badge.textContent = "真实数据库";
      if (message) {
        message.textContent = `SQLite 中的 ${papers.length} 篇文献、${datasets.length} 组数据集 · ${mcpText} · 附件暂仅在 Demo 中开放。`;
      }
    } else {
      Object.assign(window, { PLANT_REAL_MODE: false });
      if (badge) badge.textContent = "Demo 模式";
      if (message) {
        message.textContent = `虚构示例仅保存在浏览器，不会写入正式库（现有 ${status.paperCount} 篇）· ${mcpText}`;
      }
    }
    await import("./legacy/app.js");
  } catch (error) {
    if (badge) badge.textContent = "后端连接失败";
    if (message) message.textContent = errorText(error);
    if (!realMode) {
      Object.assign(window, { PLANT_REAL_MODE: false });
      await import("./legacy/app.js");
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
