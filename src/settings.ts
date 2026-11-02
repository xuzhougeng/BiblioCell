// Settings page: demo switch, storage folder, MCP token and font size.
import type { App } from "./app.ts";
import { Library, type AgentConfig, type Config } from "./mygo.ts";
import { esc } from "./model.ts";
import { applyFontSize } from "./shell.ts";
import { enqueue } from "./store.ts";
import { $, I, badge, errorMessage, option, text } from "./ui.ts";

export function renderSettings(app: App): string {
  const { env } = app;
  const preferences = env.settings;
  const mcp = env.mcp;
  const canSave = env.desktop;
  const currentPath = env.status?.storagePath || preferences.storagePath || "桌面应用启动后显示";
  const disabled = canSave ? "" : "disabled";
  const pendingPath = preferences.storagePath && preferences.storagePath !== currentPath;
  const pendingToken = preferences.mcpToken && preferences.mcpToken !== mcp.token;
  const saveStatus = canSave
    ? pendingToken ? "MCP Token 已更改，将在下次重启后使用。" : pendingPath ? "存放文件夹已更改，将在下次重启后使用。" : "设置保存在本机用户配置目录。"
    : "浏览器预览仅用于查看；请在 mygo 桌面应用中修改设置。";
  return '<div class="settings-layout"><form class="panel settings-panel" id="settings-form">'
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("leaf", 21)}</span><div><h2>数据模式</h2><p>演示与正式研究记录分别保存。</p></div></div>`
    + `<div class="settings-mode-row"><div><label for="demo-mode">Demo 演示模式</label><p>浏览虚构示例；关闭后使用真实 SQLite 数据库。</p></div><input class="settings-switch" id="demo-mode" type="checkbox" role="switch" ${env.real ? "" : "checked"}${env.desktop ? "" : " disabled"} aria-label="Demo 演示模式"></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>切换后立即重新加载，不会复制、合并或清空任何一边的数据。MCP 始终连接正式数据库。</span></div></section>`
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("folder", 21)}</span><div><h2>数据存放位置</h2><p>SQLite 数据库和文献文件都保存在这个文件夹。</p></div></div>`
    + `<div class="settings-field"><label for="settings-storage-path">存放文件夹</label><div class="settings-path-row"><input class="settings-path-input" id="settings-storage-path" name="storagePath" type="text" value="${esc(preferences.storagePath)}" placeholder="在桌面应用中选择存放文件夹" autocomplete="off" spellcheck="false" required ${disabled}><button class="btn btn-secondary" type="button" data-action="choose-storage" ${disabled}>${I("folder", 16)} 选择文件夹</button></div></div>`
    + `<div class="settings-active-path"><span>本次运行正在使用</span><code>${esc(currentPath)}</code></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>文件夹内是 bibliocell.sqlite 和 papers/。选择文件夹后，应用会自动重启并切换过去；手动输入路径时，点“保存设置”后切换。退出应用后也可以把整个文件夹复制到别处，再在这里选中副本。应用不会移动或覆盖旧数据；所选文件夹没有数据库时会新建空库。</span></div></section>`
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("code", 21)}</span><div><h2>外部 AI · MCP</h2><p>允许本机 AI 查询正式库；认证后可导入和编辑。</p></div>${badge(mcp.enabled ? "已启用" : "未启动", mcp.enabled ? "green" : "gray")}</div>`
    + `<div class="settings-mcp-meta"><div><span>Streamable HTTP 地址</span><code>${esc(mcp.enabled ? mcp.endpoint : "启动桌面应用后监听 127.0.0.1:37841")}</code></div><p><strong>工具</strong>${esc(mcp.tools.join(" · "))}</p><p><strong>Schema 资源</strong>${esc(mcp.resources.join(" · "))}</p></div>`
    + `<div class="settings-field"><label for="settings-mcp-token">Bearer Token</label><div class="settings-token-row"><input class="settings-path-input" id="settings-mcp-token" name="mcpToken" type="password" value="${esc(preferences.mcpToken)}" minlength="32" maxlength="256" autocomplete="off" spellcheck="false" required ${disabled}><button class="btn btn-secondary" type="button" data-action="toggle-mcp-token" ${disabled}>显示</button><button class="btn btn-secondary" type="button" data-action="copy-mcp-token" ${disabled}>复制</button><button class="btn btn-secondary" type="button" data-action="regenerate-mcp-token" ${disabled}>重新生成</button></div><p class="form-hint">32–256 个可见 ASCII 字符，不能包含空格。重新生成后，旧 Token 将在重启时失效。</p></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>无认证连接只读；写入使用 Authorization: Bearer &lt;TOKEN&gt;。Token 修改在重启后生效，且不会通过 MCP 返回。服务仅监听本机回环地址，不要转发到局域网或公网。</span></div></section>`
    + aiSection(preferences.ai, disabled)
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("book", 21)}</span><div><h2>阅读样式</h2><p>只调整文字大小，不改变面板宽度与整体布局。</p></div></div>`
    + `<div class="settings-field"><div class="settings-label-row"><label for="settings-font-size">界面字号</label><output id="settings-font-size-value" for="settings-font-size">${esc(preferences.fontSize)} px</output></div><div class="settings-font-control"><span>小</span><input id="settings-font-size" name="fontSize" type="range" min="12" max="20" step="1" value="${esc(preferences.fontSize)}" ${disabled}><span>大</span><button class="text-link" type="button" data-action="reset-font-size" ${disabled}>恢复 15 px</button></div></div>`
    + '<div class="settings-font-preview"><span>阅读预览</span><p>从一片叶、一个细胞，到一篇可追溯的研究。</p><small>Plant single-cell research · 文献、数据与笔记</small></div></section>'
    + `<footer class="settings-footer"><p id="settings-save-status" role="status">${saveStatus}</p><button class="btn btn-primary" type="submit" ${disabled}>${I("check", 16)} 保存设置</button></footer></form>`
    + `<aside class="settings-aside"><section class="panel settings-aside-card"><span class="eyebrow">YOUR WORKSPACE</span><h2>安静、清晰，<br>适合自己的阅读节奏。</h2><p>字号会同时应用到文献表格、详情抽屉、导航与笔记，并在下次启动时保留。</p><div class="settings-aside-rule"></div><dl><div><dt>当前模式</dt><dd>${env.real ? "真实数据库" : "Demo"}</dd></div><div><dt>正式数据</dt><dd>本地 SQLite</dd></div><div><dt>配置方式</dt><dd>自动持久化</dd></div></dl></section><div class="settings-restart-note">${I("clock", 17)}<p><strong>切换存放文件夹会自动重启</strong><br>MCP Token 在下次启动时生效；字体大小无需重启，保存前即可预览。</p></div></aside></div>`;
}

const AI_DEFAULTS: Record<string, { baseUrl: string; model: string; hint: string }> = {
  anthropic: { baseUrl: "https://api.anthropic.com", model: "claude-opus-5-5", hint: "Anthropic Messages 协议：POST {地址}/v1/messages，x-api-key 认证。兼容网关填它的根地址。" },
  openai: { baseUrl: "https://api.openai.com/v1", model: "例如 gpt-5、deepseek-chat、qwen3", hint: "OpenAI Chat Completions 协议：POST {地址}/chat/completions，Bearer 认证。DeepSeek、通义、Ollama、vLLM 等都走这个。" },
};

const aiBadge = (ai: AgentConfig, protocol: string): [string, string] => {
  const hasModel = Boolean(ai.model) || protocol === "anthropic";
  return !hasModel ? ["未配置", "gray"] : ai.apiKey ? ["已配置", "green"] : ["未填 Key", "amber"];
};

function aiSection(ai: AgentConfig, disabled: string): string {
  const protocol = ai.protocol || "anthropic";
  const defaults = AI_DEFAULTS[protocol] ?? AI_DEFAULTS.anthropic!;
  return `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("sparkle", 21)}</span><div><h2>AI 模型</h2><p>内置助手用这个模型服务，通过 MCP 工具操作正式数据库。</p></div>${badge(...aiBadge(ai, protocol))}</div>`
    + `<div class="settings-grid"><div class="settings-field"><label for="settings-ai-protocol">接口协议</label><select class="input" id="settings-ai-protocol" name="aiProtocol" ${disabled}>${option("anthropic", "Anthropic Messages", protocol)}${option("openai", "OpenAI Chat Completions（兼容）", protocol)}</select></div>`
    + `<div class="settings-field"><label for="settings-ai-model">模型名称</label><input class="input" id="settings-ai-model" name="aiModel" type="text" value="${esc(ai.model)}" placeholder="${esc(defaults.model)}" autocomplete="off" spellcheck="false" maxlength="200" ${disabled}></div></div>`
    + `<div class="settings-field"><label for="settings-ai-base-url">服务地址</label><input class="settings-path-input" id="settings-ai-base-url" name="aiBaseUrl" type="url" value="${esc(ai.baseUrl)}" placeholder="${esc(defaults.baseUrl)}" autocomplete="off" spellcheck="false" maxlength="500" ${disabled}><p class="form-hint" id="settings-ai-hint">${esc(defaults.hint)}</p></div>`
    + `<div class="settings-field"><label for="settings-ai-api-key">API Key</label><div class="settings-path-row"><input class="settings-path-input" id="settings-ai-api-key" name="aiApiKey" type="password" value="${esc(ai.apiKey)}" placeholder="本地模型服务可留空" autocomplete="off" spellcheck="false" maxlength="1000" ${disabled}><button class="btn btn-secondary" type="button" data-action="toggle-ai-key" ${disabled}>显示</button></div></div>`
    + `<div class="settings-grid"><div class="settings-field"><label for="settings-ai-effort">思考强度</label><select class="input" id="settings-ai-effort" name="aiEffort" ${disabled}>${option("", "服务默认", ai.effort)}${option("low", "低 · 快速简答", ai.effort)}${option("medium", "中", ai.effort)}${option("high", "高", ai.effort)}${option("xhigh", "很高 · 适合多步整理", ai.effort)}${option("max", "最高", ai.effort)}</select><p class="form-hint">Anthropic 发送 output_config.effort；OpenAI 兼容协议发送 reasoning_effort，很高与最高都按 high 发送。老模型不支持时请选“服务默认”。</p></div>`
    + `<div class="settings-field"><label for="settings-ai-max-steps">单次运行步数上限</label><input class="input" id="settings-ai-max-steps" name="aiMaxSteps" type="number" min="1" max="200" value="${ai.maxSteps || ""}" placeholder="20" ${disabled}></div></div>`
    + `<div class="settings-grid"><div class="settings-field"><label for="settings-ai-max-tokens">单次回复 token 上限</label><input class="input" id="settings-ai-max-tokens" name="aiMaxTokens" type="number" min="0" max="1000000" value="${ai.maxTokens || ""}" placeholder="默认" ${disabled}></div></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>Key 和地址只保存在本机 settings.json，不随存放文件夹迁移，也不会通过 MCP 返回。修改后下一次对话生效，无需重启。</span></div></section>`;
}

/** updateAIPlaceholders follows the protocol switch so the blanks read right. */
export function updateAIPlaceholders(select: HTMLSelectElement): void {
  const defaults = AI_DEFAULTS[select.value] ?? AI_DEFAULTS.anthropic!;
  const url = $<HTMLInputElement>("#settings-ai-base-url");
  const model = $<HTMLInputElement>("#settings-ai-model");
  const hint = $("#settings-ai-hint");
  if (url) url.placeholder = defaults.baseUrl;
  if (model) model.placeholder = defaults.model;
  if (hint) hint.textContent = defaults.hint;
}

const setStatus = (message: string): void => {
  const el = $("#settings-save-status");
  if (el) el.textContent = message;
};

export function saveSettings(app: App, form: HTMLFormElement): void {
  if (!app.env.desktop) return;
  const data = new FormData(form);
  const button = $<HTMLButtonElement>("button[type=submit]", form);
  const ai: AgentConfig = {
    protocol: text(data, "aiProtocol"), baseUrl: text(data, "aiBaseUrl"), apiKey: text(data, "aiApiKey"), model: text(data, "aiModel"),
    maxSteps: Number(data.get("aiMaxSteps")) || 0, maxTokens: Number(data.get("aiMaxTokens")) || 0, effort: text(data, "aiEffort"),
  };
  const next: Config = { ...app.env.settings, storagePath: text(data, "storagePath"), fontSize: Number(data.get("fontSize")), mcpToken: text(data, "mcpToken"), ai };
  if (ai.protocol === "openai" && !ai.model) {
    setStatus("OpenAI 兼容协议需要填写模型名称。");
    return;
  }
  if (!next.storagePath) {
    setStatus("请填写存放文件夹的绝对路径。");
    return;
  }
  if (!/^[\x21-\x7e]{32,256}$/.test(next.mcpToken)) {
    setStatus("MCP Token 需为 32–256 个可见 ASCII 字符，且不能包含空格。");
    return;
  }
  if (button) button.disabled = true;
  setStatus("正在保存设置…");
  Library.saveSettings(next).then(async (result) => {
    if (result.storageChanged) {
      setStatus("已保存，应用正在重启并切换到新的文件夹…");
      await enqueue(() => Library.restart()); // Behind any edit still being written.
      return;
    }
    app.env.settings = result.settings;
    applyFontSize(result.settings.fontSize);
    const path = $<HTMLInputElement>("#settings-storage-path");
    if (path) path.value = result.settings.storagePath;
    const token = $<HTMLInputElement>("#settings-mcp-token");
    if (token) token.value = result.settings.mcpToken;
    setStatus(result.mcpTokenChanged ? "设置已保存。MCP Token 将在重启后生效。" : "设置已保存，字号已应用。");
    app.toast(result.mcpTokenChanged ? "设置已保存，MCP Token 将在重启后生效。" : "设置已保存。");
  }).catch((error: unknown) => {
    setStatus(`保存失败：${errorMessage(error, "请检查路径和权限后重试。")}`);
  }).finally(() => {
    if (button) button.disabled = false;
  });
}

/** settingsActions builds the handlers of the settings page's data-action buttons. */
export const settingsActions = (app: App): Record<string, (el: HTMLElement) => void> => ({
  "choose-storage": (el) => {
    if (!app.env.desktop) return;
    const button = el as HTMLButtonElement;
    button.disabled = true;
    Library.chooseStoragePath().then((path) => {
      const input = $<HTMLInputElement>("#settings-storage-path");
      const form = $<HTMLFormElement>("#settings-form");
      if (!path || !input || !form || path === app.env.settings.storagePath) return;
      input.value = path;
      saveSettings(app, form); // Picking a folder is the decision to switch to it.
    }).catch((error: unknown) => {
      app.toast(`无法选择文件夹：${errorMessage(error, "请重试")}`, true);
    }).finally(() => {
      button.disabled = false;
    });
  },
  "reset-font-size": () => {
    const input = $<HTMLInputElement>("#settings-font-size");
    if (!input) return;
    input.value = "15";
    input.dispatchEvent(new Event("input", { bubbles: true }));
  },
  "toggle-ai-key": (el) => {
    const input = $<HTMLInputElement>("#settings-ai-api-key");
    if (!input) return;
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    el.textContent = show ? "隐藏" : "显示";
  },
  "toggle-mcp-token": (el) => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input) return;
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    el.textContent = show ? "隐藏" : "显示";
  },
  "copy-mcp-token": () => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input || !input.value) return;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(input.value).then(() => app.toast("MCP Token 已复制。")).catch(() => app.toast("复制失败，请显示 Token 后手动复制。", true));
      return;
    }
    input.type = "text";
    input.select();
    const copied = document.execCommand("copy");
    input.type = "password";
    app.toast(copied ? "MCP Token 已复制。" : "复制失败，请手动复制。", !copied);
  },
  "regenerate-mcp-token": () => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input) return;
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    input.value = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
    input.type = "text";
    const toggle = $('[data-action="toggle-mcp-token"]');
    if (toggle) toggle.textContent = "隐藏";
    setStatus("已生成新 Token，保存并重启后生效。");
  },
});

/** previewFontSize applies the slider value immediately; saving persists it. */
export function previewFontSize(input: HTMLInputElement): void {
  const size = Number(input.value);
  const output = $("#settings-font-size-value");
  if (output) output.textContent = `${size} px`;
  applyFontSize(size);
  setStatus(`正在预览 ${size} px，点击“保存设置”后保留。`);
}
