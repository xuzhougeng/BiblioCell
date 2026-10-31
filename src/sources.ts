// "更新与来源" page: automatic updates, connector cards, configuration, records and manual sync.
import type { App } from "./app.ts";
import { Library, events, type SourceConfig, type SourceStatus } from "./mygo.ts";
import { arr, dateText, esc, nextRunText, safeURL, shortTime } from "./model.ts";
import { enqueue } from "./store.ts";
import { $, I, badge, empty, errorMessage, option, setFormError, text } from "./ui.ts";

const fallbackSource = (partial: Partial<SourceStatus> & Pick<SourceStatus, "id" | "name" | "kind" | "description" | "homeUrl">): SourceStatus => ({
  supported: true, enabled: true, query: "", maxResults: 20, provider: "", recordCount: 0, lastSuccessAt: "", lastRun: null, recentRecords: [], ...partial,
});

/** FALLBACK_SOURCES is shown in browser preview, where no Go backend lists connectors. */
const FALLBACK_SOURCES: SourceStatus[] = [
  fallbackSource({ id: "pubmed", name: "PubMed", kind: "literature", description: "通过 NCBI E-utilities 检索论文与摘要。", homeUrl: "https://pubmed.ncbi.nlm.nih.gov/" }),
  fallbackSource({ id: "europe-pmc", name: "Europe PMC", kind: "literature", description: "检索文献、摘要与标识符。", homeUrl: "https://europepmc.org/" }),
  fallbackSource({ id: "crossref", name: "Crossref", kind: "literature", description: "检索 DOI 与出版信息。", homeUrl: "https://www.crossref.org/" }),
  fallbackSource({ id: "biorxiv", name: "bioRxiv", kind: "literature", description: "扫描最近 30 天的预印本并匹配关键词。", homeUrl: "https://www.biorxiv.org/" }),
  fallbackSource({ id: "sequence-repositories", name: "GEO / SRA / ENA", kind: "repository", description: "保存公共仓库 accession 线索。", homeUrl: "https://www.ebi.ac.uk/ena/browser/home", provider: "geo" }),
  fallbackSource({ id: "cncb-gsa", name: "CNCB GSA", kind: "repository", description: "站点需要浏览器安全检查，当前仅保留手动入口。", homeUrl: "https://ngdc.cncb.ac.cn/gsa/", supported: false, enabled: false }),
];

const QUERY_HINTS: Record<string, string> = {
  pubmed: '使用 PubMed 检索语法，例如 ("single cell"[Title/Abstract]) AND Arabidopsis[Title/Abstract]。',
  "europe-pmc": '使用 Europe PMC 检索语法，例如 TITLE_ABS:"single cell" AND TITLE_ABS:plant。',
  crossref: "输入书目关键词；Crossref 按相关性返回 DOI 和出版信息。",
  biorxiv: "输入空格分隔的普通关键词，所有词都需匹配题目、摘要或分类；扫描最近 30 天，最多检查 10 页。",
  "sequence-repositories": 'GEO / SRA 使用 NCBI 检索语法；ENA 使用 Portal API 语法，例如 description="*single*" AND tax_tree(33090)。',
};

/** SOURCE_CREDENTIALS are the per-machine settings that raise a source's rate limit. */
const NCBI_CREDENTIAL = { name: "ncbiApiKey", label: "NCBI API Key", type: "password" } as const;
const SOURCE_CREDENTIALS: Record<string, { name: "ncbiApiKey" | "contactEmail"; label: string; type: string; hint: string }> = {
  pubmed: { ...NCBI_CREDENTIAL, hint: "选填。在 NCBI 账户设置中生成，请求上限从每秒 3 次提高到 10 次；与 GEO / SRA 共用。" },
  "sequence-repositories": { ...NCBI_CREDENTIAL, hint: "选填。在 NCBI 账户设置中生成，请求上限从每秒 3 次提高到 10 次；与 PubMed 共用，ENA 不使用。" },
  crossref: { name: "contactEmail", label: "联系邮箱", type: "email", hint: "选填。Crossref 没有免费 API Key，提供邮箱即可进入 polite 池，获得更高限额；Unpaywall 查找开放全文也用它。" },
};

const COMMON_QUERY = '("single cell"[All Fields] OR "single-cell"[All Fields] OR "single nucleus"[All Fields]) AND (plant[All Fields] OR Arabidopsis[Organism])';
const PROVIDER_DEFAULT_QUERIES: Record<string, string> = { geo: `${COMMON_QUERY} AND gse[ETYP]`, sra: COMMON_QUERY, ena: 'description="*single*" AND tax_tree(33090)' };

const INTERVALS: [number, string][] = [[6, "每 6 小时"], [12, "每 12 小时"], [24, "每天"], [72, "每 3 天"], [168, "每周"]];

function renderAutomation(app: App): string {
  const automation = app.env.automation;
  const disabled = app.env.real ? "" : " disabled";
  // Another interval can come from the command line.
  const choices = INTERVALS.some(([hours]) => hours === automation.intervalHours) ? INTERVALS : [...INTERVALS, [automation.intervalHours, `每 ${automation.intervalHours} 小时`] as [number, string]];
  const facts = [
    ["上次更新", automation.lastRunAt ? shortTime(automation.lastRunAt) : "尚未运行"],
    ["下次更新", nextRunText(automation)],
    ["新增", `${automation.added} 条`],
    ["跳过（去重）", `${automation.skipped} 条`],
  ].map(([label, value]) => `<div><dt>${label}</dt><dd>${esc(value)}</dd></div>`).join("");
  const failures = automation.failures.map((failure) => `<p class="source-error" role="alert"><strong>${esc(failure.source)}</strong>：${esc(failure.message || "同步失败，请检查配置和网络。")}</p>`).join("");
  return `<section class="panel automation-panel"><div class="panel-header"><div><h2>自动更新</h2><p>应用运行时按频率更新已启用的来源；关闭期间错过的更新，下次启动时补跑。</p></div><input class="settings-switch" id="automation-enabled" type="checkbox" role="switch" aria-label="自动更新" ${automation.enabled ? "checked" : ""}${disabled}></div>`
    + `<div class="automation-body"><label class="automation-interval"><span>更新频率</span><select class="input" id="automation-interval"${disabled}>${choices.map(([hours, label]) => `<option value="${hours}"${hours === automation.intervalHours ? " selected" : ""}>${esc(label)}</option>`).join("")}</select></label><dl class="automation-facts">${facts}</dl></div>`
    + `<div class="automation-notes"><p class="muted">新增与跳过是各来源最近一次运行的合计，手动同步也计入。</p>${failures}</div></section>`;
}

export function renderSources(app: App): string {
  const realMode = app.env.real;
  const items = app.env.sources.length ? app.env.sources : FALLBACK_SOURCES;
  const notice = realMode
    ? "自动更新开启后才会在后台访问公开 API，否则只在点击“同步”时访问。新文献进入“待判断”；按 DOI、PMID、题目判定已存在的记录会跳过，不改动人工内容；仓库结果只保存为 accession 线索。"
    : "来源同步只写入真实 SQLite 数据库。请先在设置中关闭 Demo 演示模式，再配置和运行同步。";
  const cards = items.map((source) => {
    const run = source.lastRun;
    const running = app.syncing.has(source.id) || run?.status === "running";
    const failed = run?.status === "failed";
    const stateLabel = !source.supported ? "手动入口" : running ? "同步中" : !source.enabled ? "已停用" : failed ? "上次失败" : "已配置";
    const stateColor = failed ? "amber" : source.supported && source.enabled ? "green" : "gray";
    const counts = run?.status === "succeeded" ? ` · 新增 ${source.kind === "repository" ? run.recordsNew : run.papersAdded} · 跳过 ${run.duplicates}` : "";
    const last = source.lastSuccessAt ? `最近成功：${shortTime(source.lastSuccessAt)}${counts}` : "最近同步：尚未成功";
    const query = source.id === "sequence-repositories" ? `${String(source.provider || "geo").toUpperCase()} · ${source.query}` : source.query;
    const actions = source.supported
      ? `<button class="btn btn-secondary" data-action="configure-source" data-id="${esc(source.id)}"${!realMode || running ? " disabled" : ""}>配置</button><button class="btn btn-primary" data-action="sync-source" data-id="${esc(source.id)}"${!realMode || !source.enabled || running || app.env.automation.running ? " disabled" : ""}>${I(running ? "clock" : "refresh", 15)}${running ? " 正在同步" : " 同步"}</button>`
      : "";
    return `<section class="source-card"><div class="source-heading"><span class="source-letter">${esc(source.name.slice(0, 1))}</span><div><h3>${esc(source.name)}</h3><p class="muted">${source.kind === "repository" ? "数据仓库" : "文献元数据"}</p></div>${badge(stateLabel, stateColor)}</div><p class="source-description">${esc(source.description)}</p>`
      + (source.supported ? `<div class="source-query" title="${esc(query)}"><code>${esc(query || "查询待配置")}</code></div>` : "")
      + (failed ? `<p class="source-error" role="alert">${esc(source.lastRun?.errorMessage || "同步失败，请检查配置和网络。")}</p>` : "")
      + `<div class="source-footer"><span class="muted">${esc(last)}</span><a href="${esc(safeURL(source.homeUrl))}" target="_blank" rel="noopener noreferrer" class="text-link">官方入口 ${I("arrow-up-right", 14)}</a></div>`
      + `<div class="source-actions"><button class="text-link" data-action="source-records" data-id="${esc(source.id)}">记录 ${Number(source.recordCount || 0)}</button>${actions}</div></section>`;
  }).join("");
  const activities = app.state.activities;
  const activityList = activities.length
    ? `<div class="activity-list">${activities.slice(0, 20).map((a) => `<div class="activity-item"><span class="activity-dot"></span><div><p>${esc(a.message)}</p><span class="muted">${dateText(a.at)} · ${new Date(a.at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}</span></div></div>`).join("")}</div>`
    : empty("暂无整理活动", "添加文献、保存笔记、导入数据或同步来源后，可以在这里查看记录。");
  return `<div class="notice-box">${I("info", 18)}<span>${notice}</span></div>${renderAutomation(app)}<div class="source-grid">${cards}</div><section class="panel activity-panel"><div class="panel-header"><div><h2>整理活动</h2><p class="muted">记录当前会话中的整理与同步操作</p></div><button class="text-link" data-action="export-json">${I("download", 15)} 导出备份</button></div>${activityList}</section>`;
}

export function sourceConfigModal(app: App, id: string): void {
  const source = app.env.sources.find((item) => item.id === id);
  if (!source || !source.supported) return;
  const provider = source.id === "sequence-repositories"
    ? `<label class="form-field full-width"><span>检索仓库</span><select class="input" name="provider">${option("geo", "GEO（NCBI）", source.provider)}${option("sra", "SRA（NCBI）", source.provider)}${option("ena", "ENA（European Nucleotide Archive）", source.provider)}</select></label>`
    : "";
  const credential = SOURCE_CREDENTIALS[id];
  const credentialField = credential
    ? `<label class="form-field full-width"><span>${esc(credential.label)}</span><input class="input" name="${credential.name}" type="${credential.type}" value="${esc(app.env.settings[credential.name])}" maxlength="256" autocomplete="off" spellcheck="false"><small class="form-hint">${esc(credential.hint)}只保存在本机，不写入数据库。</small></label>`
    : "";
  const body = `<form id="source-config-form" data-initial-query="${esc(source.query)}"><input type="hidden" name="id" value="${esc(id)}"><div class="settings-mode-row source-enable-row"><div><label for="source-enabled">启用此来源</label><p>启用后可手动同步；开启自动更新时也会按频率访问。</p></div><input class="settings-switch" id="source-enabled" name="enabled" type="checkbox" role="switch" ${source.enabled ? "checked" : ""}></div><div class="form-grid">${provider}<label class="form-field full-width"><span>检索式 / 关键词 <b class="required">*</b></span><textarea class="textarea" name="query" rows="5" maxlength="4000" required>${esc(source.query)}</textarea><small class="form-hint">${esc(QUERY_HINTS[id] ?? "")}</small></label><label class="form-field"><span>单次最多保存</span><input class="input" name="maxResults" type="number" min="1" max="100" step="1" value="${esc(source.maxResults || 20)}" required></label>${credentialField}</div><p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(`配置 ${source.name}`, body, `<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="source-config-form">${I("check", 16)} 保存配置</button>`, true);
}

/** applyProviderDefault swaps in the provider's default query while the user has not edited the field. */
export function applyProviderDefault(select: HTMLSelectElement): void {
  const form = select.form;
  const query = form?.elements.namedItem("query");
  if (!form || !(query instanceof HTMLTextAreaElement)) return;
  if (query.value !== form.dataset.initialQuery) return;
  query.value = PROVIDER_DEFAULT_QUERIES[select.value] ?? query.value;
  form.dataset.initialQuery = query.value;
}

export function saveSourceConfig(app: App, form: HTMLFormElement): void {
  if (!app.env.desktop) return;
  const data = new FormData(form);
  const config: SourceConfig = { id: text(data, "id"), enabled: data.get("enabled") === "on", query: text(data, "query"), maxResults: Number(data.get("maxResults")), provider: text(data, "provider") };
  if (!config.query) {
    setFormError("请填写检索式或关键词。");
    return;
  }
  if (!Number.isInteger(config.maxResults) || config.maxResults < 1 || config.maxResults > 100) {
    setFormError("单次结果数量需为 1–100。");
    return;
  }
  const button = $<HTMLButtonElement>('button[form="source-config-form"]');
  if (button) button.disabled = true;
  const credential = SOURCE_CREDENTIALS[config.id];
  const value = credential ? text(data, credential.name) : "";
  const savedCredential = credential && value !== app.env.settings[credential.name]
    ? Library.saveSettings({ ...app.env.settings, [credential.name]: value }).then((result) => { app.env.settings = result.settings; })
    : Promise.resolve();
  savedCredential.then(() => Library.saveSourceConfig(config)).then((status) => {
    app.env.sources = app.env.sources.map((item) => (item.id === status.id ? status : item));
    app.closeModal();
    app.render();
    app.toast(`${status.name} 配置已保存。`);
  }).catch((error: unknown) => {
    setFormError(errorMessage(error, "保存失败，请重试。"));
    if (button) button.disabled = false;
  });
}

export function sourceRecordsModal(app: App, id: string): void {
  const source = app.env.sources.find((item) => item.id === id);
  const close = '<button class="btn btn-secondary" data-action="close-modal">关闭</button>';
  if (!source) {
    app.showModal("外部记录", empty("桌面应用中查看", "浏览器预览不连接正式 SQLite；打开桌面应用并同步后，可在这里查看外部标识和官方链接。"), close);
    return;
  }
  const records = arr(source.recentRecords);
  const body = records.length
    ? `<p class="muted">显示最近见到的 ${records.length} 条；所有来源快照均保留在 SQLite。</p><div class="source-record-list">${records.map((record) => {
      const url = safeURL(record.url);
      return `<div class="source-record-item"><div><strong>${esc(record.title || record.externalId)}</strong><code>${esc(record.externalId)}</code><span>${esc(new Date(record.lastSeenAt).toLocaleString("zh-CN"))}</span></div>${url ? `<a class="btn-icon" href="${esc(url)}" target="_blank" rel="noopener noreferrer" aria-label="打开外部记录">${I("arrow-up-right", 18)}</a>` : ""}</div>`;
    }).join("")}</div>`
    : empty("暂无外部记录", "保存配置并点击同步后，外部标识和官方链接会显示在这里。");
  app.showModal(`${source.name} · 外部记录`, body, close, true);
}

export async function syncSource(app: App, id: string): Promise<void> {
  if (app.syncing.has(id) || !app.env.desktop) return;
  app.syncing.add(id);
  app.render();
  try {
    const result = await enqueue(() => Library.syncSource(id));
    await refreshSources(app);
    await app.reload();
    const source = app.env.sources.find((item) => item.id === id);
    const message = `${source ? source.name : id} 同步完成：${result.recordsSeen} 条外部记录，新增 ${result.papersAdded} 篇文献，${result.duplicates} 条已存在。`;
    app.logActivity(message, "sync");
    app.toast(message);
  } catch (error) {
    app.toast(`同步失败：${errorMessage(error, "请检查网络和来源配置。")}`, true);
    await refreshSources(app);
  } finally {
    app.syncing.delete(id);
    app.render();
  }
}

export async function refreshSources(app: App): Promise<void> {
  try {
    [app.env.sources, app.env.automation] = await Promise.all([Library.listSources(), Library.sourceAutomation()]);
  } catch {
    // Keep the stale list; the next render still shows the failure toast.
  }
}

export function saveAutomation(app: App): void {
  if (!app.env.real) return;
  const enabled = $<HTMLInputElement>("#automation-enabled")?.checked ?? false;
  const hours = Number($<HTMLSelectElement>("#automation-interval")?.value);
  Library.saveSourceAutomation(enabled, hours).then((automation) => {
    app.env.automation = automation;
    app.toast(enabled ? "自动更新已开启。" : "自动更新已关闭。");
  }).catch((error: unknown) => {
    app.toast(`保存失败：${errorMessage(error, "请重试。")}`, true);
  }).finally(() => app.render());
}

/** watchSources shows what background updates do while the window is open. */
export function watchSources(app: App): void {
  events.sourcesChanged.on((automation) => {
    app.env.automation = automation;
    void refreshSources(app).then(() => app.refresh());
  });
}
