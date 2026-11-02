// Settings page: demo switch, storage folder, MCP token and font size.
import type { App } from "./app.ts";
import { Library, type AgentConfig, type Config, type ModelProfile, type TaskModels } from "./mygo.ts";
import { AI_DEFAULTS, PROTOCOL_LABELS, TASKS, endpointOf, modelName, models, newProfileId, profileStatus } from "./models.ts";
import { LOCALES, locale, setLocale, t } from "./i18n.ts";
import { esc } from "./model.ts";
import { applyFontSize } from "./shell.ts";
import { enqueue } from "./store.ts";
import { $, I, badge, errorMessage, option, setFormError, text } from "./ui.ts";

export function renderSettings(app: App): string {
  const { env } = app;
  const preferences = env.settings;
  const mcp = env.mcp;
  const canSave = env.desktop;
  const currentPath = env.status?.storagePath || preferences.storagePath || t("桌面应用启动后显示");
  const disabled = canSave ? "" : "disabled";
  const pendingPath = preferences.storagePath && preferences.storagePath !== currentPath;
  const pendingToken = preferences.mcpToken && preferences.mcpToken !== mcp.token;
  const saveStatus = canSave
    ? pendingToken ? t("MCP Token 已更改，将在下次重启后使用。") : pendingPath ? t("存放文件夹已更改，将在下次重启后使用。") : t("设置保存在本机用户配置目录。")
    : t("浏览器预览仅用于查看；请在 mygo 桌面应用中修改设置。");
  return '<div class="settings-layout"><form class="panel settings-panel" id="settings-form">'
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("leaf", 21)}</span><div><h2>${t("数据模式")}</h2><p>${t("演示与正式研究记录分别保存。")}</p></div></div>`
    + `<div class="settings-mode-row"><div><label for="demo-mode">${t("Demo 演示模式")}</label><p>${t("浏览虚构示例；关闭后使用真实 SQLite 数据库。")}</p></div><input class="settings-switch" id="demo-mode" type="checkbox" role="switch" ${env.real ? "" : "checked"}${env.desktop ? "" : " disabled"} aria-label="${t("Demo 演示模式")}"></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>${t("切换后立即重新加载，不会复制、合并或清空任何一边的数据。MCP 始终连接正式数据库。")}</span></div></section>`
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("folder", 21)}</span><div><h2>${t("数据存放位置")}</h2><p>${t("SQLite 数据库和文献文件都保存在这个文件夹。")}</p></div></div>`
    + `<div class="settings-field"><label for="settings-storage-path">${t("存放文件夹")}</label><div class="settings-path-row"><input class="settings-path-input" id="settings-storage-path" name="storagePath" type="text" value="${esc(preferences.storagePath)}" placeholder="${t("在桌面应用中选择存放文件夹")}" autocomplete="off" spellcheck="false" required ${disabled}><button class="btn btn-secondary" type="button" data-action="choose-storage" ${disabled}>${I("folder", 16)} ${t("选择文件夹")}</button></div></div>`
    + `<div class="settings-active-path"><span>${t("本次运行正在使用")}</span><code>${esc(currentPath)}</code></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>${t("文件夹内是 bibliocell.sqlite 和 papers/。选择文件夹后，应用会自动重启并切换过去；手动输入路径时，点“保存设置”后切换。退出应用后也可以把整个文件夹复制到别处，再在这里选中副本。应用不会移动或覆盖旧数据；所选文件夹没有数据库时会新建空库。")}</span></div></section>`
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("code", 21)}</span><div><h2>${t("外部 AI · MCP")}</h2><p>${t("允许本机 AI 查询正式库；认证后可导入和编辑。")}</p></div>${badge(mcp.enabled ? t("已启用") : t("未启动"), mcp.enabled ? "green" : "gray")}</div>`
    + `<div class="settings-mcp-meta"><div><span>${t("Streamable HTTP 地址")}</span><code>${esc(mcp.enabled ? mcp.endpoint : t("启动桌面应用后监听 127.0.0.1:37841"))}</code></div><p><strong>${t("工具")}</strong>${esc(mcp.tools.join(" · "))}</p><p><strong>${t("Schema 资源")}</strong>${esc(mcp.resources.join(" · "))}</p></div>`
    + `<div class="settings-field"><label for="settings-mcp-token">Bearer Token</label><div class="settings-token-row"><input class="settings-path-input" id="settings-mcp-token" name="mcpToken" type="password" value="${esc(preferences.mcpToken)}" minlength="32" maxlength="256" autocomplete="off" spellcheck="false" required ${disabled}><button class="btn btn-secondary" type="button" data-action="toggle-mcp-token" ${disabled}>${t("显示")}</button><button class="btn btn-secondary" type="button" data-action="copy-mcp-token" ${disabled}>${t("复制")}</button><button class="btn btn-secondary" type="button" data-action="regenerate-mcp-token" ${disabled}>${t("重新生成")}</button></div><p class="form-hint">${t("32–256 个可见 ASCII 字符，不能包含空格。重新生成后，旧 Token 将在重启时失效。")}</p></div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>${t("无认证连接只读；写入使用 Authorization: Bearer &lt;TOKEN&gt;。Token 修改在重启后生效，且不会通过 MCP 返回。服务仅监听本机回环地址，不要转发到局域网或公网。")}</span></div></section>`
    + `<div id="settings-ai-section">${aiSection(preferences, disabled)}</div>`
    + `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("book", 21)}</span><div><h2>${t("阅读样式")}</h2><p>${t("只调整文字大小，不改变面板宽度与整体布局。")}</p></div></div>`
    + `<div class="settings-field"><div class="settings-label-row"><label for="settings-font-size">${t("界面字号")}</label><output id="settings-font-size-value" for="settings-font-size">${esc(preferences.fontSize)} px</output></div><div class="settings-font-control"><span>${t("小")}</span><input id="settings-font-size" name="fontSize" type="range" min="12" max="20" step="1" value="${esc(preferences.fontSize)}" ${disabled}><span>${t("大")}</span><button class="text-link" type="button" data-action="reset-font-size" ${disabled}>${t("恢复 15 px")}</button></div></div>`
    + `<div class="settings-font-preview"><span>${t("阅读预览")}</span><p>${t("从一片叶、一个细胞，到一篇可追溯的研究。")}</p><small>${t("Plant single-cell research · 文献、数据与笔记")}</small></div></section>`
    + localeSection()
    + `<footer class="settings-footer"><p id="settings-save-status" role="status">${saveStatus}</p><button class="btn btn-primary" type="submit" ${disabled}>${I("check", 16)} ${t("保存设置")}</button></footer></form>`
    + `<aside class="settings-aside"><section class="panel settings-aside-card"><span class="eyebrow">YOUR WORKSPACE</span><h2>${t("安静、清晰，<br>适合自己的阅读节奏。")}</h2><p>${t("字号会同时应用到文献表格、详情抽屉、导航与笔记，并在下次启动时保留。")}</p><div class="settings-aside-rule"></div><dl><div><dt>${t("当前模式")}</dt><dd>${env.real ? t("真实数据库") : "Demo"}</dd></div><div><dt>${t("正式数据")}</dt><dd>${t("本地 SQLite")}</dd></div><div><dt>${t("配置方式")}</dt><dd>${t("自动持久化")}</dd></div></dl></section><div class="settings-restart-note">${I("clock", 17)}<p>${t("<strong>切换存放文件夹会自动重启</strong><br>MCP Token 在下次启动时生效；字体大小无需重启，保存前即可预览。")}</p></div></aside></div>`;
}

/** localeSection picks the interface language; it works in browser preview too, so it is never disabled. */
const localeSection = (): string =>
  `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("settings", 21)}</span><div><h2>${t("界面语言")}</h2><p>${t("切换后立即重新加载。文献、笔记等研究记录保持原样，不会被翻译。")}</p></div></div>`
  // Bilingual on purpose, so someone who cannot read the current language still finds the switch.
  + '<div class="settings-field"><label for="settings-locale">界面语言 / Interface language</label>' // i18n-ignore
  + `<select class="input" id="settings-locale">${LOCALES.map(([code, name]) => option(code, name, locale)).join("")}</select></div></section>`;

const PROTOCOL_OPTIONS = (selected: string): string => option("anthropic", "Anthropic Messages", selected) + option("openai", t("OpenAI Chat Completions（兼容）"), selected);

/** modelCard shows one configured model service with what it is and how to change it. */
function modelCard(profile: ModelProfile, disabled: string): string {
  const [label, color] = profileStatus(profile);
  const config = profile.config;
  return `<div class="model-card"><div class="model-card-head"><strong>${esc(profile.name)}</strong>${badge(label, color)}<span class="section-actions"><button class="text-link" type="button" data-action="model-edit" data-id="${esc(profile.id)}" ${disabled}>${t("编辑")}</button><button class="text-link" type="button" data-action="model-delete" data-id="${esc(profile.id)}" ${disabled}>${t("删除")}</button></span></div>`
    + `<div class="model-card-meta"><span>${esc(modelName(config) || t("模型名待填"))}</span><span class="meta-separator">·</span><span>${esc(PROTOCOL_LABELS[config.protocol || "anthropic"] ?? config.protocol)}</span><span class="meta-separator">·</span><code>${esc(endpointOf(config))}</code>${config.effort ? `<span class="meta-separator">·</span><span>${t("思考 {effort}", { effort: esc(config.effort) })}</span>` : ""}</div></div>`;
}

/** aiSection lists the model services and binds each AI feature to one of them. */
function aiSection(settings: Config, disabled: string): string {
  const list = models(settings);
  const tasks: TaskModels = settings.tasks ?? { chat: "", screen: "", translate: "" };
  const cards = list.length
    ? list.map((m) => modelCard(m, disabled)).join("")
    : `<p class="muted model-empty">${t("还没有添加模型。添加后，AI 对话、AI 判断和 AI 翻译都可以各自选择用哪个。")}</p>`;
  const first = list[0];
  const bindings = TASKS.map(([task, label, hint]) =>
    `<div class="settings-field"><label for="settings-task-${task}">${esc(label)}</label><select class="input" id="settings-task-${task}" name="task-${task}" ${disabled}>${option("", first ? t("默认 · {name}", { name: first.name }) : t("默认（第一个模型）"), tasks[task])}${list.map((m) => option(m.id, m.name, tasks[task])).join("")}</select><p class="form-hint">${esc(hint)}</p></div>`,
  ).join("");
  return `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("sparkle", 21)}</span><div><h2>${t("AI 模型")}</h2><p>${t("添加一个或多个模型服务，再为每项 AI 功能选择用哪个。")}</p></div>${badge(list.length ? t("{n} 个模型", { n: list.length }) : t("未配置"), list.length ? "green" : "gray")}</div>`
    + `<div class="model-list">${cards}</div><button class="btn btn-secondary" type="button" data-action="model-add" ${disabled}>${I("plus", 16)} ${t("添加模型")}</button>`
    + `<h3 class="settings-subheading">${t("各功能使用的模型")}</h3><div class="settings-grid settings-grid-3">${bindings}</div>`
    + `<div class="settings-inline-note">${I("info", 16)}<span>${t("模型的添加、编辑和删除会立即保存；各功能的选择点“保存设置”后生效。Key 和地址只保存在本机 settings.json，不随存放文件夹迁移，也不会通过 MCP 返回。")}</span></div></section>`;
}

/** modelModal edits one profile, or a blank one when id is empty. */
export function modelModal(app: App, id = ""): void {
  const existing = models(app.env.settings).find((m) => m.id === id);
  const config: AgentConfig = existing?.config ?? { protocol: "anthropic", baseUrl: "", apiKey: "", model: "", maxTokens: 0, maxSteps: 0, effort: "" };
  const protocol = config.protocol || "anthropic";
  const defaults = AI_DEFAULTS[protocol] ?? AI_DEFAULTS.anthropic!;
  const body = `<form id="model-profile-form"><input type="hidden" name="id" value="${esc(existing?.id ?? "")}">`
    + `<div class="settings-grid"><label class="form-field"><span>${t("名称")} <b class="required">*</b></span><input class="input" name="name" value="${esc(existing?.name ?? "")}" placeholder="${t("例如 Claude、DeepSeek、本机 Ollama")}" maxlength="100" required></label>`
    + `<label class="form-field"><span>${t("接口协议")}</span><select class="input" id="model-protocol" name="protocol">${PROTOCOL_OPTIONS(protocol)}</select></label></div>`
    + `<label class="form-field"><span>${t("模型名称")}</span><input class="input" id="model-name" name="model" value="${esc(config.model)}" placeholder="${esc(defaults.model)}" autocomplete="off" spellcheck="false" maxlength="200"></label>`
    + `<label class="form-field"><span>${t("服务地址")}</span><input class="input" id="model-base-url" name="baseUrl" type="url" value="${esc(config.baseUrl)}" placeholder="${esc(defaults.baseUrl)}" autocomplete="off" spellcheck="false" maxlength="500"><small class="form-hint" id="model-hint">${esc(defaults.hint)}</small></label>`
    + `<label class="form-field"><span>API Key</span><span class="settings-path-row"><input class="input" id="model-api-key" name="apiKey" type="password" value="${esc(config.apiKey)}" placeholder="${t("本地模型服务可留空")}" autocomplete="off" spellcheck="false" maxlength="1000"><button class="btn btn-secondary" type="button" data-action="toggle-model-key">${t("显示")}</button></span></label>`
    + `<div class="settings-grid"><label class="form-field"><span>${t("思考强度")}</span><select class="input" name="effort">${option("", t("服务默认"), config.effort)}${option("low", t("低 · 快速简答"), config.effort)}${option("medium", t("中"), config.effort)}${option("high", t("高"), config.effort)}${option("xhigh", t("很高 · 适合多步整理"), config.effort)}${option("max", t("最高"), config.effort)}</select></label>`
    + `<label class="form-field"><span>${t("单次运行步数上限")}</span><input class="input" name="maxSteps" type="number" min="1" max="200" value="${config.maxSteps || ""}" placeholder="20"></label></div>`
    + `<label class="form-field"><span>${t("单次回复 token 上限")}</span><input class="input" name="maxTokens" type="number" min="0" max="1000000" value="${config.maxTokens || ""}" placeholder="${t("默认")}"></label>`
    + `<p class="form-hint">${t("Anthropic 发送 output_config.effort；OpenAI 兼容协议发送 reasoning_effort，很高与最高都按 high 发送。老模型不支持时请选“服务默认”。")}</p>`
    + `<p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(existing ? t("编辑模型") : t("添加模型"), body, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-primary" type="submit" form="model-profile-form">${t("保存模型")}</button>`);
}

/** saveModelProfile stores the modal's profile at once, since the key is useless until it is saved. */
export function saveModelProfile(app: App, form: HTMLFormElement): void {
  if (!app.env.desktop) return;
  const data = new FormData(form);
  const config: AgentConfig = {
    protocol: text(data, "protocol"), baseUrl: text(data, "baseUrl"), apiKey: text(data, "apiKey"), model: text(data, "model"),
    maxSteps: Number(data.get("maxSteps")) || 0, maxTokens: Number(data.get("maxTokens")) || 0, effort: text(data, "effort"),
  };
  const name = text(data, "name");
  if (!name) return setFormError(t("请填写模型名称。"));
  if (config.protocol === "openai" && !config.model) return setFormError(t("OpenAI 兼容协议需要填写模型名称。"));
  const list = models(app.env.settings).slice();
  const id = text(data, "id");
  const index = list.findIndex((m) => m.id === id);
  const profile: ModelProfile = { id: index >= 0 ? id : newProfileId(list), name, config };
  if (index >= 0) list[index] = profile; else list.push(profile);
  persistModels(app, list, app.env.settings.tasks, t("模型已保存。"));
}

/** deleteModelProfile removes a profile and clears any binding that pointed at it. */
export function deleteModelProfile(app: App, id: string): void {
  const list = models(app.env.settings).filter((m) => m.id !== id);
  const tasks = { ...(app.env.settings.tasks ?? { chat: "", screen: "", translate: "" }) };
  for (const task of ["chat", "screen", "translate"] as const) if (tasks[task] === id) tasks[task] = "";
  persistModels(app, list, tasks, t("模型已删除。"));
}

function persistModels(app: App, list: ModelProfile[], tasks: TaskModels, done: string): void {
  const next: Config = { ...app.env.settings, models: list, tasks };
  Library.saveSettings(next).then((result) => {
    app.env.settings = result.settings;
    app.closeModal();
    const section = $("#settings-ai-section");
    if (section) section.innerHTML = aiSection(result.settings, "");
    app.toast(done);
  }).catch((error: unknown) => {
    const message = t("保存失败：{error}", { error: errorMessage(error, t("请检查填写内容后重试。")) });
    if ($("#model-profile-form")) setFormError(message); else app.toast(message, true);
  });
}

/** updateAIPlaceholders follows the protocol switch so the blanks read right. */
export function updateAIPlaceholders(select: HTMLSelectElement): void {
  const defaults = AI_DEFAULTS[select.value] ?? AI_DEFAULTS.anthropic!;
  const url = $<HTMLInputElement>("#model-base-url");
  const model = $<HTMLInputElement>("#model-name");
  const hint = $("#model-hint");
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
  const tasks: TaskModels = { chat: text(data, "task-chat"), screen: text(data, "task-screen"), translate: text(data, "task-translate") };
  const next: Config = { ...app.env.settings, storagePath: text(data, "storagePath"), fontSize: Number(data.get("fontSize")), mcpToken: text(data, "mcpToken"), tasks };
  if (!next.storagePath) {
    setStatus(t("请填写存放文件夹的绝对路径。"));
    return;
  }
  if (!/^[\x21-\x7e]{32,256}$/.test(next.mcpToken)) {
    setStatus(t("MCP Token 需为 32–256 个可见 ASCII 字符，且不能包含空格。"));
    return;
  }
  if (button) button.disabled = true;
  setStatus(t("正在保存设置…"));
  Library.saveSettings(next).then(async (result) => {
    if (result.storageChanged) {
      setStatus(t("已保存，应用正在重启并切换到新的文件夹…"));
      await enqueue(() => Library.restart()); // Behind any edit still being written.
      return;
    }
    app.env.settings = result.settings;
    applyFontSize(result.settings.fontSize);
    const path = $<HTMLInputElement>("#settings-storage-path");
    if (path) path.value = result.settings.storagePath;
    const token = $<HTMLInputElement>("#settings-mcp-token");
    if (token) token.value = result.settings.mcpToken;
    setStatus(result.mcpTokenChanged ? t("设置已保存。MCP Token 将在重启后生效。") : t("设置已保存，字号已应用。"));
    app.toast(result.mcpTokenChanged ? t("设置已保存，MCP Token 将在重启后生效。") : t("设置已保存。"));
  }).catch((error: unknown) => {
    setStatus(t("保存失败：{error}", { error: errorMessage(error, t("请检查路径和权限后重试。")) }));
  }).finally(() => {
    if (button) button.disabled = false;
  });
}

let localeListening = false;

/** listenLocale switches language as soon as #settings-locale changes; registered once, it survives re-renders. */
function listenLocale(): void {
  if (localeListening || typeof document === "undefined") return;
  localeListening = true;
  document.addEventListener("change", (e) => {
    const target = e.target;
    if (target instanceof HTMLSelectElement && target.id === "settings-locale") setLocale(target.value);
  });
}

/** settingsActions builds the handlers of the settings page's data-action buttons. */
export const settingsActions = (app: App): Record<string, (el: HTMLElement) => void> => (listenLocale(), {
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
      app.toast(t("无法选择文件夹：{error}", { error: errorMessage(error, t("请重试")) }), true);
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
  "model-add": () => modelModal(app),
  "model-edit": (el) => modelModal(app, el.dataset.id ?? ""),
  "model-delete": (el) => {
    const profile = models(app.env.settings).find((m) => m.id === el.dataset.id);
    if (!profile) return;
    app.showModal(t("删除模型"), `<p>${t("确定删除模型“{name}”？用它的功能会改用第一个模型。", { name: esc(profile.name) })}</p>`, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-danger" data-action="confirm-model-delete" data-id="${esc(profile.id)}">${t("确认删除")}</button>`);
  },
  "confirm-model-delete": (el) => deleteModelProfile(app, el.dataset.id ?? ""),
  "toggle-model-key": (el) => {
    const input = $<HTMLInputElement>("#model-api-key");
    if (!input) return;
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    el.textContent = show ? t("隐藏") : t("显示");
  },
  "toggle-mcp-token": (el) => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input) return;
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    el.textContent = show ? t("隐藏") : t("显示");
  },
  "copy-mcp-token": () => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input || !input.value) return;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(input.value).then(() => app.toast(t("MCP Token 已复制。"))).catch(() => app.toast(t("复制失败，请显示 Token 后手动复制。"), true));
      return;
    }
    input.type = "text";
    input.select();
    const copied = document.execCommand("copy");
    input.type = "password";
    app.toast(copied ? t("MCP Token 已复制。") : t("复制失败，请手动复制。"), !copied);
  },
  "regenerate-mcp-token": () => {
    const input = $<HTMLInputElement>("#settings-mcp-token");
    if (!input) return;
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    input.value = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
    input.type = "text";
    const toggle = $('[data-action="toggle-mcp-token"]');
    if (toggle) toggle.textContent = t("隐藏");
    setStatus(t("已生成新 Token，保存并重启后生效。"));
  },
});

/** previewFontSize applies the slider value immediately; saving persists it. */
export function previewFontSize(input: HTMLInputElement): void {
  const size = Number(input.value);
  const output = $("#settings-font-size-value");
  if (output) output.textContent = `${size} px`;
  applyFontSize(size);
  setStatus(t("正在预览 {size} px，点击“保存设置”后保留。", { size }));
}
