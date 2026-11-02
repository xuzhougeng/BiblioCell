import { PAPER_COLUMNS, COLUMN_STORAGE_KEY, defaultPaperColumns, loadPaperColumns } from "./columns.ts";
// Application state, rendering and event delegation.
import type { Config, MCPInfo, Paper, SourceAutomation, SourceStatus, Status } from "./mygo.ts";
import { demoState } from "./demo-data.ts";
import { renderDrawer } from "./drawer.ts";
import type { Evidence } from "./mygo.ts";
import { doiActions } from "./doi.ts";
import { fileActions } from "./files.ts";
import {
  datasetModal, exclusionReasonModal, importSummaryModal, linkDatasetModal, paperModal, resetDemoModal, resourceModal,
  saveDataset, saveDatasetLink, saveExclusionReason, savePaper, saveResource,
} from "./forms.ts";
import {
  PAGES, PAGE_SIZES, READING, SCREEN, SORT_KEYS, clone, csvText, decideScreening, emptyFilters, esc, filterDatasets, filterPapers, isPage, nowISO, paperDatasets, prepareImport,
  speciesNameFor, tasksFor, toggleSort, uid, type Filters, type ImportStats, type LibraryState, type Page, type Resource, type SortKey, type TaskRow, type TaskType,
} from "./model.ts";
import { applyProviderDefault, refreshSources, renderSources, saveAutomation, saveSourceConfig, sourceConfigModal, sourceRecordsModal, syncSource, useSourceQuery, watchSources } from "./sources.ts";
import { reviewActions } from "./review.ts";
import { DECISIONS, applyDecision, renderScreening, screenActions, screenPapers, type ScreenState } from "./screen.ts";
import { previewFontSize, renderSettings, saveSettings, settingsActions, updateAIPlaceholders } from "./settings.ts";
import { agentActions, emptyAgentState, renderAgent, sendAgentMessage, watchAgent, type AgentState } from "./agent.ts";
import { Library } from "./mygo.ts";
import { enqueue, SaveRejected, type LibraryStore } from "./store.ts";
import { $, $$, I, downloadFile, errorMessage, speciesRow } from "./ui.ts";
import { renderDatasets, renderOverview, renderPapers, renderTasks } from "./views.ts";
import { t } from "./i18n.ts";

/** Env describes the runtime: desktop vs browser preview, real vs demo data, and backend-provided info. */
export interface Env {
  desktop: boolean;
  real: boolean;
  settings: Config;
  mcp: MCPInfo;
  status: Status | null;
  sources: SourceStatus[];
  automation: SourceAutomation;
}

export interface AppOptions {
  store: LibraryStore;
  env: Env;
  initial: LibraryState;
  warning?: string;
}

type Drawer = { type: "paper" | "dataset"; id: string };
type Action = (el: HTMLElement, id: string) => void;

const DEMO_ONLY_ACTIONS = new Set(["import-json", "confirm-import", "reset-demo", "confirm-reset", "add-resource"]);

export class App {
  state: LibraryState;
  env: Env;
  filters: Filters = emptyFilters();
  taskFilter: TaskType | "all" = "all";
  datasetSearch = "";
  /** Datasets page: only those a source update added that no person has checked. */
  datasetReview = false;
  /** Papers-table pagination; the stored page is clamped against the filtered total at render time. */
  paperPage = 1;
  paperColumns = loadPaperColumns();
  paperPageSize = PAGE_SIZES.includes(Number(localStorage.getItem("bibliocell-page-size"))) ? Number(localStorage.getItem("bibliocell-page-size")) : PAGE_SIZES[0]!;
  /** Row the arrow keys last landed on, per table; null until the first ↑/↓ press. */
  paperCursor: number | null = null;
  datasetCursor: number | null = null;
  /** Datasets page: table vs card view plus the same pagination pattern. */
  datasetView: "cards" | "table" = localStorage.getItem("bibliocell-datasets-view") === "cards" ? "cards" : "table";
  datasetPage = 1;
  datasetPageSize = PAGE_SIZES.includes(Number(localStorage.getItem("bibliocell-datasets-page-size"))) ? Number(localStorage.getItem("bibliocell-datasets-page-size")) : PAGE_SIZES[0]!;
  drawer: Drawer | null = null;
  /** Papers page: screening mode shows one paper at a time instead of the table. */
  screen: ScreenState | null = null;
  agent: AgentState = emptyAgentState();
  drawerTab = "overview";
  syncing = new Set<string>();
  pendingImport: { next: LibraryState; stats: ImportStats } | null = null;

  private readonly store: LibraryStore;
  /** The state as last synced with the store. It moves with this.state, never after an await. */
  private saved: LibraryState;
  private edits = 0;
  private page: Page;
  private lastFocus: Element | null = null;
  private modalLastFocus: Element | null = null;
  private searchTimer: ReturnType<typeof setTimeout> | undefined;
  private warning: string;
  private deleting = false;
  private contextRow: HTMLElement | null = null;

  constructor(options: AppOptions) {
    this.store = options.store;
    this.env = options.env;
    this.state = options.initial;
    this.saved = clone(options.initial);
    this.warning = options.warning ?? "";
    const hash = location.hash.slice(1);
    this.page = isPage(hash) ? hash : "overview";
    if (this.env.real) document.body.classList.add("real-mode");
  }

  start(): void {
    document.addEventListener("contextmenu", (e) => {
      const row = e.target instanceof Element ? e.target.closest<HTMLElement>('tr[data-action="open-paper"]') : null;
      if (!row || this.drawer || $("#modal-root .modal")) return;
      e.preventDefault();
      this.closeContextMenu();
      row.focus();
      this.contextRow = row;
      const menu = document.createElement("div");
      menu.id = "paper-context-menu";
      menu.className = "paper-context-menu";
      menu.setAttribute("role", "menu");
      const button = document.createElement("button");
      button.className = "btn btn-danger";
      button.textContent = t("删除文献");
      button.setAttribute("role", "menuitem");
      button.dataset.action = "delete-paper";
      button.dataset.id = row.dataset.id;
      menu.appendChild(button);
      document.body.appendChild(menu);
      menu.style.left = Math.max(0, Math.min(e.clientX, innerWidth - menu.offsetWidth)) + "px";
      menu.style.top = Math.max(0, Math.min(e.clientY, innerHeight - menu.offsetHeight)) + "px";
      button.focus();
    });
    window.addEventListener("resize", () => this.closeContextMenu());
    document.addEventListener("scroll", () => this.closeContextMenu(), true);
    document.addEventListener("click", (e) => this.onClick(e));
    document.addEventListener("submit", (e) => this.onSubmit(e));
    document.addEventListener("change", (e) => this.onChange(e));
    document.addEventListener("input", (e) => this.onInput(e));
    document.addEventListener("keydown", (e) => this.onKeydown(e));
    window.addEventListener("hashchange", () => {
      const next = location.hash.slice(1);
      if (isPage(next)) {
        this.page = next;
        this.render();
      }
    });
    // Agents write while the window sits in the background; show their work on return.
    if (this.env.real) window.addEventListener("focus", () => void refreshSources(this).then(() => this.refresh()));
    if (this.env.desktop) {
      watchSources(this);
      watchAgent(this);
    }
    this.render();
    if (this.warning) this.toast(this.warning, true);
  }

  private closeContextMenu(): void {
    const menu = $("#paper-context-menu");
    if (menu?.contains(document.activeElement)) this.contextRow?.focus();
    menu?.remove();
    this.contextRow = null;
  }

  private requestDeletePaper(id: string): void {
    const paper = this.paperById(id);
    if (!paper) return;
    this.showModal(t("删除文献"), `<p>${t("确定删除《{title}》？", { title: esc(paper.title) })}</p><p>${t("将删除文献、笔记、相关证据和保存的附件副本，并解除数据集关联。数据集及原始导入文件会保留。此操作无法撤销。")}</p>`, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-danger" data-action="confirm-delete-paper" data-id="${esc(id)}">${t("确认删除")}</button>`);
  }

  private async deletePaper(id: string, button: HTMLElement): Promise<void> {
    if (this.deleting || !this.paperById(id)) return;
    this.deleting = true;
    button.setAttribute("disabled", "");
    try {
      if (this.env.real) await enqueue(() => Library.deletePaper(id));
      const next = clone(this.state);
      const fileIds = new Set(next.resources.filter(r => r.paperId === id).map(r => r.id));
      next.papers = next.papers.filter(p => p.id !== id);
      next.resources = next.resources.filter(r => r.paperId !== id);
      next.evidence = (next.evidence ?? []).filter(e => e.paperId !== id && !fileIds.has(e.fileId));
      next.datasets.forEach(d => { d.paperLinks = d.paperLinks.filter(l => l.paperId !== id); });
      if (!this.env.real) await this.store.save(next, this.saved);
      this.state = next;
      this.saved = clone(next);
      this.edits++;
      this.closeModal();
      if (this.drawer?.type === "paper" && this.drawer.id === id) this.closeDrawer();
      this.render();
      if (this.env.real) await this.reload();
      this.toast(t("文献已删除。"));
    } catch (error) {
      this.toast(errorMessage(error, t("删除失败，请重试。")), true);
    } finally {
      this.deleting = false;
      button.removeAttribute("disabled");
    }
  }

  private showPaperColumns(): void {
    this.showModal(t("显示列"), `<p class="muted">${t("勾选要显示的列，至少保留一列。设置自动保存到当前设备。")}</p><div class="column-options">` + PAPER_COLUMNS.map(([id, label]) => '<label class="checkbox-choice"><input type="checkbox" data-paper-column="' + id + '"' + (this.paperColumns.includes(id) ? ' checked' : '') + '><span>' + label + '</span></label>').join("") + '</div>', `<button class="btn btn-secondary" data-action="reset-paper-columns">${t("恢复默认")}</button><button class="btn btn-primary" data-action="close-modal">${t("完成")}</button>`);
  }
  private savePaperColumns(): void {
    try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(this.paperColumns)); }
    catch { this.toast(t("列设置已应用，但无法保存到当前设备。"), true); }
    this.render();
  }

  // ---- queries ----
  paperById(id: string): Paper | undefined {
    return this.state.papers.find((p) => p.id === id);
  }
  paperDatasets(id: string) {
    return paperDatasets(this.state.datasets, id);
  }
  paperResources(id: string): Resource[] {
    return this.state.resources.filter((r) => r.paperId === id);
  }
  evidenceFor(target: "paperId" | "datasetId", id: string): Evidence[] {
    return (this.state.evidence ?? []).filter((e) => e[target] === id);
  }
  tasks(): TaskRow[] {
    return tasksFor(this.state.papers, this.state.datasets, this.state.evidence);
  }

  // ---- persistence ----
  persist(): void {
    const next = clone(this.state);
    const saved = this.saved;
    this.saved = next;
    this.edits++;
    this.store.save(next, saved).catch((error: unknown) => {
      this.toast(errorMessage(error, t("保存失败，请重试。")), true);
      if (error instanceof SaveRejected) void this.reload();
    });
  }
  /** reload shows what the database holds now, including what agents wrote. */
  async reload(): Promise<void> {
    const edits = this.edits;
    try {
      const fresh = await this.store.load();
      if (edits !== this.edits) return await this.reload(); // A newer edit is queued behind this read.
      this.state = { ...fresh, activities: this.state.activities };
      this.saved = clone(this.state);
      this.render();
      this.renderDrawer();
    } catch (error) {
      this.toast(t("刷新失败：{reason}", { reason: errorMessage(error, t("请重新打开应用。")) }), true);
    }
  }
  /** refresh shows background writes, unless that would pull the page from under the person. */
  refresh(): void {
    if (this.drawer || $("#modal-root .modal") || document.activeElement?.matches("#app-content input, #app-content textarea, #app-content select")) return;
    if (this.env.real) void this.reload();
    else this.render();
  }
  logActivity(message: string, kind = "edit"): void {
    this.state.activities.unshift({ id: uid("act"), message, kind, at: nowISO() });
    this.state.activities = this.state.activities.slice(0, 100);
  }

  // ---- rendering ----
  render(): void {
    this.closeContextMenu();
    const content = $("#app-content");
    if (!content) return;
    const renderers: Record<Page, () => string> = {
      overview: () => renderOverview(this),
      papers: () => (this.screen ? renderScreening(this) : renderPapers(this)),
      datasets: () => renderDatasets(this),
      tasks: () => renderTasks(this),
      sources: () => renderSources(this),
      agent: () => renderAgent(this),
      settings: () => renderSettings(this),
    };
    if (this.page !== "papers") this.screen = null; // Screening belongs to the papers list it started from.
    content.innerHTML = renderers[this.page]();
    document.body.classList.toggle("page-agent", this.page === "agent"); // The assistant page is the chat window itself.
    if (this.page === "agent") {
      const transcript = $("#agent-transcript");
      if (transcript) transcript.scrollTop = transcript.scrollHeight; // Open on the newest message, like a chat window.
    }
    const actions = $(".heading-actions");
    if (actions) actions.hidden = this.page === "settings" || this.page === "agent";
    const [title, subtitle] = PAGES[this.page];
    for (const [selector, value] of [["#page-title", title], ["#page-subtitle", subtitle]] as const) {
      const el = $(selector);
      if (el) el.textContent = value;
    }
    $$("[data-page]").forEach((el) => {
      const active = el.dataset.page === this.page;
      el.classList.toggle("active", active);
      if (active) el.setAttribute("aria-current", "page");
      else el.removeAttribute("aria-current");
    });
    const counts: [string, number][] = [["#count-papers", this.state.papers.length], ["#count-datasets", this.state.datasets.length], ["#count-tasks", this.tasks().length]];
    for (const [selector, value] of counts) {
      const el = $(selector);
      if (el) el.textContent = String(value);
    }
  }

  navigate(next: string): void {
    if (!isPage(next)) return;
    this.page = next;
    if (location.hash !== `#${next}`) history.replaceState(null, "", `#${next}`);
    this.render();
    document.body.classList.remove("sidebar-open", "nav-open");
  }

  toast(message: string, error = false): void {
    const root = $("#toast-root");
    if (!root) return;
    const el = document.createElement("div");
    el.className = `toast${error ? " error" : ""}`;
    el.setAttribute("role", error ? "alert" : "status");
    el.innerHTML = `${I(error ? "alert" : "check-circle", 18)}<span>${esc(message)}</span>`;
    root.appendChild(el);
    setTimeout(() => el.remove(), error ? 6500 : 3600);
  }

  // ---- drawer ----
  openPaper(id: string): void {
    if (!this.paperById(id)) return;
    this.lastFocus = document.activeElement;
    this.drawer = { type: "paper", id };
    this.drawerTab = "overview";
    this.renderDrawer(true);
  }
  openPaperResources(id: string): void {
    if (!this.paperById(id)) return;
    this.lastFocus = document.activeElement;
    this.drawer = { type: "paper", id };
    this.drawerTab = "resources";
    this.renderDrawer(true);
  }
  openDataset(id: string): void {
    if (!this.state.datasets.some((d) => d.id === id)) return;
    this.lastFocus = document.activeElement;
    this.drawer = { type: "dataset", id };
    this.drawerTab = "overview";
    this.renderDrawer(true);
  }
  closeDrawer(): void {
    this.drawer = null;
    const root = $("#drawer-root");
    if (root) root.innerHTML = "";
    document.body.classList.remove("drawer-open");
    if (this.lastFocus instanceof HTMLElement && document.contains(this.lastFocus)) this.lastFocus.focus();
  }
  /** renderDrawer keeps the reading position unless fresh, so reviewing item after item stays in place. */
  renderDrawer(fresh = false): void {
    if (!this.drawer) return;
    const root = $("#drawer-root");
    if (!root) return;
    const html = renderDrawer(this);
    if (html === null) {
      this.closeDrawer();
      return;
    }
    const scrollTop = fresh ? 0 : $(".drawer-body", root)?.scrollTop ?? 0;
    root.innerHTML = html;
    const body = $(".drawer-body", root);
    if (body) body.scrollTop = scrollTop;
    document.body.classList.add("drawer-open");
    requestAnimationFrame(() => $('#drawer-root button[data-action="close-drawer"]')?.focus());
  }

  /** switchDrawerItem moves the open drawer to the previous or next record of the list it came from. */
  private switchDrawerItem(delta: 1 | -1): void {
    if (!this.drawer) return;
    let id = "";
    if (this.drawer.type === "paper") {
      const list = this.page === "papers" ? filterPapers(this.state.papers, this.filters) : this.state.papers;
      id = list[list.findIndex((p) => p.id === this.drawer!.id) + delta]?.id ?? "";
    } else {
      const list = this.page === "datasets" && this.datasetView === "table"
        ? filterDatasets(this.state.datasets, this.datasetSearch, (pid) => this.paperById(pid)?.title ?? "", this.datasetReview)
        : this.state.datasets;
      id = list[list.findIndex((d) => d.id === this.drawer!.id) + delta]?.id ?? "";
    }
    if (!id) return;
    this.drawer = { type: this.drawer.type, id };
    this.renderDrawer(true);
  }

  // ---- screening mode ----
  /** startScreening snapshots the filtered list and opens it one paper at a time, from the row cursor if any. */
  startScreening(): void {
    const ids = filterPapers(this.state.papers, this.filters).map((p) => p.id);
    const index = this.paperCursor == null ? 0 : (this.paperPage - 1) * this.paperPageSize + this.paperCursor;
    this.screen = { ids, index: Math.min(Math.max(index, 0), Math.max(ids.length - 1, 0)) };
    this.closeDrawer();
    this.render();
    window.scrollTo({ top: 0 });
    $("#app-content")?.focus({ preventScroll: true });
  }
  exitScreening(): void {
    if (!this.screen) return;
    const id = screenPapers(this)[this.screen.index]?.id;
    this.screen = null;
    this.closeDrawer();
    if (id) {
      // Land the table on the paper that was open, so the list picks up where screening left off.
      const index = filterPapers(this.state.papers, this.filters).findIndex((p) => p.id === id);
      if (index >= 0) {
        this.paperPage = Math.floor(index / this.paperPageSize) + 1;
        this.paperCursor = index % this.paperPageSize;
      }
    }
    this.render();
    if (this.paperCursor != null) this.focusTableRow(false, this.paperCursor);
  }
  moveScreening(delta: number): void {
    if (!this.screen) return;
    const papers = screenPapers(this);
    const next = this.screen.index + delta;
    if (next < 0 || next >= papers.length) return;
    this.screen.index = next;
    this.render();
    window.scrollTo({ top: 0 });
  }
  /** decideInScreening records the decision and moves on; an exclusion without a reason asks for one first. */
  decideInScreening(id: string, status: string): void {
    const p = this.paperById(id);
    if (!p || !this.screen) return;
    if (!applyDecision(this, p, status, (paper) => exclusionReasonModal(this, paper))) return;
    this.toast(t("已记为：{label}", { label: SCREEN[status] ?? status }));
    this.screeningDecided();
  }
  /** screeningDecided moves to the next paper after a decision, or re-renders on the last one. */
  screeningDecided(): void {
    if (!this.screen) return;
    if (this.screen.index < screenPapers(this).length - 1) this.moveScreening(1);
    else this.render();
  }

  // ---- modal ----
  showModal(title: string, body: string, footer: string, wide = false): void {
    this.modalLastFocus = document.activeElement;
    const root = $("#modal-root");
    if (!root) return;
    root.innerHTML = `<div class="modal-backdrop" data-action="close-modal"></div><section class="modal${wide ? " modal-wide" : ""}" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-header"><h2 id="modal-title">${esc(title)}</h2><button class="btn-icon" data-action="close-modal" aria-label="${t("关闭弹窗")}">${I("x", 20)}</button></div><div class="modal-body">${body}</div><div class="modal-footer">${footer}</div></section>`;
    document.body.classList.add("modal-open");
    requestAnimationFrame(() => {
      const target = $("#modal-root .modal input:not([type=hidden])") ?? $("#modal-root .modal textarea") ?? $("#modal-root .modal select") ?? $("#modal-root .modal button");
      target?.focus();
    });
  }
  closeModal(): void {
    const root = $("#modal-root");
    if (root) root.innerHTML = "";
    this.pendingImport = null;
    document.body.classList.remove("modal-open");
    if (this.modalLastFocus instanceof HTMLElement && document.contains(this.modalLastFocus)) this.modalLastFocus.focus();
  }

  // ---- import / export ----
  exportCSV(): void {
    const list = filterPapers(this.state.papers, this.filters);
    downloadFile(`plantcell-papers-${nowISO().slice(0, 10)}.csv`, csvText(list, this.state.datasets), "text/csv;charset=utf-8");
    this.toast(t("已导出当前筛选的 {n} 篇文献。", { n: list.length }));
  }
  exportJSON(): void {
    downloadFile(`plantcell-library-${nowISO().slice(0, 10)}.json`, JSON.stringify({ ...this.state, exportedAt: nowISO() }, null, 2), "application/json;charset=utf-8");
    this.toast(t("已导出文献、数据集、链接与笔记。"));
  }
  importJSON(): void {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json,application/json";
    input.hidden = true;
    document.body.appendChild(input);
    input.addEventListener("change", async () => {
      const file = input.files?.[0];
      if (!file) {
        input.remove();
        return;
      }
      try {
        if (file.size > 10 * 1024 * 1024) throw new Error(t("请导入 10 MB 以内的 JSON 文件。"));
        const data: unknown = JSON.parse(await file.text());
        this.pendingImport = prepareImport(data, this.state);
        importSummaryModal(this, this.pendingImport.stats);
      } catch (error) {
        this.toast(error instanceof SyntaxError ? t("JSON 格式无效，请使用本工具导出的备份文件。") : errorMessage(error, t("导入失败，原有数据未改变。")), true);
      }
      input.remove();
    });
    input.click();
  }
  private confirmImport(): void {
    if (!this.pendingImport) return;
    const { next, stats } = this.pendingImport;
    this.state = next;
    this.logActivity(t("导入 {papers} 篇文献、{datasets} 组数据集和 {resources} 个链接", { papers: stats.papers, datasets: stats.datasets, resources: stats.resources }), "import");
    this.persist();
    this.closeModal();
    this.closeDrawer();
    this.render();
    this.toast(t("导入完成，已有记录保持原值。"));
  }
  private confirmReset(): void {
    this.state = demoState();
    this.logActivity(t("已恢复演示数据"), "reset");
    this.persist();
    this.closeModal();
    this.closeDrawer();
    this.filters = emptyFilters();
    this.navigate("overview");
    this.toast(t("已恢复示例内容。"));
  }

  // ---- paper edits from drawer / table ----
  private touchPaper(id: string, update: (p: Paper) => void): Paper | undefined {
    const p = this.paperById(id);
    if (!p) return undefined;
    update(p);
    p.updatedAt = nowISO();
    return p;
  }

  private readonly actions: Record<string, Action> = {
    "new-paper": () => paperModal(this),
    "edit-paper": (_el, id) => paperModal(this, id),
    "configure-source": (_el, id) => sourceConfigModal(this, id),
    "sync-source": (_el, id) => void syncSource(this, id),
    "source-records": (_el, id) => sourceRecordsModal(this, id),
    "use-source-query": (el) => useSourceQuery(el),
    "add-species-row": () => {
      const rows = $("#species-rows");
      if (!rows) return;
      rows.insertAdjacentHTML("beforeend", speciesRow());
      $$<HTMLElement>(".species-row", rows).at(-1)?.querySelector<HTMLInputElement>("[name=speciesName]")?.focus();
    },
    "remove-species-row": (el) => el.closest(".species-row")?.remove(),
    "delete-paper": (_el, id) => this.requestDeletePaper(id),
    "confirm-delete-paper": (el, id) => { void this.deletePaper(id, el); },
    "open-paper": (_el, id) => this.openPaper(id),
    "supplement-fulltext": (_el, id) => this.openPaperResources(id),
    "open-dataset": (_el, id) => this.openDataset(id),
    "close-drawer": () => this.closeDrawer(),
    "close-modal": () => this.closeModal(),
    "drawer-tab": (el) => {
      this.drawerTab = el.dataset.tab ?? "overview";
      this.renderDrawer(true);
    },
    "paper-columns": () => this.showPaperColumns(),
    "reset-paper-columns": () => { this.paperColumns = defaultPaperColumns(); this.savePaperColumns(); $$("[data-paper-column]").forEach(el => { if (el instanceof HTMLInputElement) el.checked = true; }); },
    "export-csv": () => this.exportCSV(),
    "export-json": () => this.exportJSON(),
    "import-json": () => this.importJSON(),
    "reset-demo": () => resetDemoModal(this),
    "confirm-reset": () => this.confirmReset(),
    "confirm-import": () => this.confirmImport(),
    "toggle-star": (_el, id) => {
      if (!this.touchPaper(id, (p) => { p.starred = !p.starred; })) return;
      this.persist();
      this.render();
      if (this.drawer) this.renderDrawer();
    },
    "save-notes": (_el, id) => {
      const notes = $<HTMLTextAreaElement>("#paper-notes")?.value ?? "";
      const p = this.touchPaper(id, (paper) => { paper.notes = notes; });
      if (!p) return;
      this.logActivity(t("保存笔记：{title}", { title: p.title }));
      this.persist();
      this.toast(t("笔记已保存。"));
    },
    "save-tags": (_el, id) => {
      const raw = $<HTMLInputElement>("#paper-tags")?.value ?? "";
      const tags = [...new Set(raw.split(/[,，;；\n]/).map((s) => s.trim()).filter(Boolean))].slice(0, 30);
      if (!this.touchPaper(id, (p) => { p.tags = tags; })) return;
      this.persist();
      this.toast(t("标签已保存。"));
      this.render();
    },
    "clear-filters": () => {
      this.filters = emptyFilters();
      this.paperPage = 1;
      const search = $<HTMLInputElement>("#global-search");
      if (search) search.value = "";
      this.render();
    },
    "table-page": (el) => {
      const page = Number(el.dataset.id);
      if (!page || page < 1) return;
      const datasets = el.dataset.scope === "datasets";
      if (datasets) this.datasetPage = page;
      else this.paperPage = page;
      this.render();
      // Keep the new page's header in view below the sticky topbar.
      const table = $(datasets ? ".datasets-panel .table-wrap" : ".papers-panel .table-wrap");
      if (table) window.scrollTo({ top: window.scrollY + table.getBoundingClientRect().top - 66 });
    },
    "datasets-view": (el) => {
      const view = el.dataset.view === "cards" ? "cards" : "table";
      if (view === this.datasetView) return;
      this.datasetView = view;
      localStorage.setItem("bibliocell-datasets-view", view);
      this.render();
    },
    "sort-column": (el) => {
      const key = el.dataset.key as SortKey;
      if (!SORT_KEYS.includes(key)) return;
      this.filters.sort = toggleSort(this.filters.sort, key);
      this.paperPage = 1;
      this.render();
    },
    "all-papers": () => {
      this.filters.starred = false;
      this.filters.review = "";
      this.paperPage = 1;
      this.render();
    },
    "starred-papers": () => {
      this.filters.starred = true;
      this.filters.review = "";
      this.paperPage = 1;
      this.render();
    },
    "review-papers": () => {
      clearTimeout(this.searchTimer);
      this.filters = { ...emptyFilters(), review: "concern", sort: this.filters.sort };
      this.paperPage = 1;
      const search = $<HTMLInputElement>("#global-search");
      if (search) search.value = "";
      this.render();
    },
    "all-datasets": () => {
      this.datasetReview = false;
      this.datasetPage = 1;
      this.render();
    },
    "review-datasets": () => {
      this.datasetReview = true;
      this.datasetPage = 1;
      this.render();
    },
    "task-filter": (el) => {
      const value = el.dataset.filterValue;
      this.taskFilter = value === "screen" || value === "fulltext" || value === "data" ? value : "all";
      this.render();
    },
    "stat-nav": (el) => {
      const target = el.dataset.target ?? "";
      if (target === "screen" || target === "fulltext") {
        this.taskFilter = target;
        this.navigate("tasks");
      } else {
        this.navigate(target);
      }
    },
    "species-filter": (el) => {
      const name = speciesNameFor(this.state.papers, el.dataset.label ?? "");
      if (!name) return;
      this.filters.species = name;
      this.paperPage = 1;
      this.navigate("papers");
    },
    "new-dataset": () => datasetModal(this),
    "edit-dataset": (_el, id) => datasetModal(this, id),
    "new-dataset-for-paper": (_el, id) => {
      this.closeModal();
      datasetModal(this, "", id);
    },
    "link-dataset": (_el, id) => linkDatasetModal(this, id),
    "add-resource": (el, id) => resourceModal(this, el.dataset.kind ?? "paper", id),
    "accept-proposal": (_el, id) => {
      const p = this.paperById(id);
      if (!p) return;
      decideScreening(p, p.proposedScreening, p.proposedReason || p.screeningReason);
      this.logActivity(t("采纳 Agent 建议：{title}", { title: p.title }));
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast(t("已采纳，判断记为人工确认。"));
    },
    "dismiss-proposal": (_el, id) => {
      if (!this.touchPaper(id, (p) => {
        p.proposedScreening = "";
        p.proposedReason = "";
      })) return;
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast(t("已忽略这条建议。"));
    },
    ...settingsActions(this),
    ...agentActions(this),
    ...fileActions(this),
    ...doiActions(this),
    ...reviewActions(this),
    ...screenActions(this),
  };

  private onClick(e: MouseEvent): void {
    const target = e.target;
    this.closeContextMenu();
    if (!(target instanceof Element)) return;
    // Anchors navigate by themselves; the row or card around them must not also open.
    if (target.closest("a[href]")) return;
    const el = target.closest<HTMLElement>("[data-action]");
    if (el) {
      const action = el.dataset.action ?? "";
      if (this.env.real && DEMO_ONLY_ACTIONS.has(action)) {
        this.toast(t("真实模式暂不支持资料链接和 JSON 导入；文献文件可在详情的“数据与资料”中导入。"), true);
        return;
      }
      const handler = this.actions[action];
      if (handler) {
        handler(el, el.dataset.id ?? "");
        return;
      }
    }
    const nav = target.closest<HTMLElement>("[data-page]");
    if (nav) this.navigate(nav.dataset.page ?? "");
  }

  private onSubmit(e: SubmitEvent): void {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    const handlers: Record<string, (app: App, form: HTMLFormElement) => void> = {
      "paper-form": savePaper, "source-config-form": saveSourceConfig, "settings-form": saveSettings, "agent-form": sendAgentMessage,
      "exclusion-reason-form": saveExclusionReason, "dataset-form": saveDataset, "resource-form": saveResource, "link-dataset-form": saveDatasetLink,
    };
    // getAttribute: an <input name="id"> inside the form shadows form.id.
    const handler = handlers[form.getAttribute("id") ?? ""];
    if (!handler) return;
    e.preventDefault();
    handler(this, form);
  }

  private onChange(e: Event): void {
    const target = e.target;
    if (!(target instanceof HTMLElement)) return;
    if (target instanceof HTMLInputElement && target.dataset.paperColumn) {
      const column = PAPER_COLUMNS.find(([id]) => id === target.dataset.paperColumn)?.[0];
      if (!column) return;
      if (!target.checked && this.paperColumns.length === 1) {
        target.checked = true;
        this.toast(t("至少保留一列。"), true);
        return;
      }
      const chosen = new Set(this.paperColumns);
      if (target.checked) chosen.add(column); else chosen.delete(column);
      this.paperColumns = PAPER_COLUMNS.filter(([id]) => chosen.has(id)).map(([id]) => id);
      this.savePaperColumns();
      return;
    }
    if (target instanceof HTMLSelectElement && target.matches('#source-config-form [name="provider"]')) {
      applyProviderDefault(target);
      return;
    }
    if (target.id === "settings-ai-protocol" && target instanceof HTMLSelectElement) {
      updateAIPlaceholders(target);
      return;
    }
    if (target.id === "automation-enabled" || target.id === "automation-interval") {
      saveAutomation(this);
      return;
    }
    if (target.id === "demo-mode" && target instanceof HTMLInputElement) {
      localStorage.setItem("bibliocell-demo-mode", String(target.checked));
      location.reload();
      return;
    }
    if (target instanceof HTMLSelectElement && "pageSize" in target.dataset) {
      const size = Number(target.value);
      if (PAGE_SIZES.includes(size)) {
        if (target.dataset.pageSize === "datasets") {
          this.datasetPageSize = size;
          localStorage.setItem("bibliocell-datasets-page-size", String(size));
        } else {
          this.paperPageSize = size;
          localStorage.setItem("bibliocell-page-size", String(size));
        }
      }
      this.paperPage = 1;
      this.datasetPage = 1;
      this.render();
    }
    if (target instanceof HTMLSelectElement && target.dataset.filter) {
      const key = target.dataset.filter as keyof Filters;
      if (key !== "starred" && key in this.filters) {
        this.filters[key] = target.value;
        this.paperPage = 1;
        this.render();
      }
    }
    if (target instanceof HTMLSelectElement && target.dataset.paperSetting) {
      const p = this.paperById(target.dataset.id ?? "");
      if (!p) return;
      const key = target.dataset.paperSetting;
      const value = target.value;
      if (key === "screening" && Object.hasOwn(SCREEN, value)) {
        if (value === "excluded" && !String(p.screeningReason || "").trim()) {
          target.value = p.screening;
          exclusionReasonModal(this, p);
          return;
        }
        decideScreening(p, value);
      } else if (key === "reading" && Object.hasOwn(READING, value)) {
        p.reading = value;
      } else {
        return;
      }
      p.updatedAt = nowISO();
      this.logActivity(key === "reading" ? t("更新阅读进度：{title}", { title: p.title }) : t("更新收录判断：{title}", { title: p.title }));
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast(t("整理状态已保存。"));
    }
  }

  private onInput(e: Event): void {
    const target = e.target;
    if (target instanceof HTMLTextAreaElement && target.dataset.queueIndex !== undefined) {
      this.agent.queue[Number(target.dataset.queueIndex)] = target.value; // Queued messages stay editable until sent.
      return;
    }
    if (!(target instanceof HTMLInputElement)) return;
    if (target.id === "settings-font-size") {
      previewFontSize(target);
      return;
    }
    if (target.id === "paper-search" || target.id === "dataset-search") {
      const { id, value, selectionStart } = target;
      clearTimeout(this.searchTimer);
      this.searchTimer = setTimeout(() => {
        if (id === "paper-search") {
          this.filters.search = value;
          this.paperPage = 1;
        } else {
          this.datasetSearch = value;
          this.datasetPage = 1;
        }
        this.render();
        const input = document.getElementById(id);
        if (input instanceof HTMLInputElement) {
          input.focus();
          try {
            input.setSelectionRange(selectionStart, selectionStart);
          } catch {
            // Search inputs do not support selection ranges in some browsers.
          }
        }
      }, 140);
    }
  }

  private onKeydown(e: KeyboardEvent): void {
    const target = e.target instanceof HTMLElement ? e.target : null;
    if (target?.id === "agent-input" && e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      target.closest("form")?.requestSubmit();
      return;
    }
    if ($("#paper-context-menu")) {
      if (e.key === "Escape" || e.key === "Tab") { e.preventDefault(); this.closeContextMenu(); }
      if (e.key !== "Enter" && e.key !== " ") e.preventDefault();
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault(); // Otherwise the web view passes the key on and macOS beeps.
      if ($("#modal-root .modal")) this.closeModal();
      else if (this.drawer) this.closeDrawer();
      else if (this.screen) this.exitScreening();
      else document.body.classList.remove("nav-open");
      return;
    }
    if (this.screen && this.page === "papers" && !this.drawer && !$("#modal-root .modal") && !target?.matches("input,textarea,select") && !e.altKey && !e.ctrlKey && !e.metaKey && this.screenKey(e)) {
      e.preventDefault();
      return;
    }
    if (e.key === "Enter" && target instanceof HTMLInputElement && target.id === "global-search") {
      e.preventDefault();
      this.filters = { ...emptyFilters(), search: target.value, sort: this.filters.sort };
      this.paperPage = 1;
      this.navigate("papers");
      return;
    }
    if (e.key === "Enter" && target?.matches("tr[data-action]")) {
      e.preventDefault();
      if (target.dataset.action === "open-dataset") this.openDataset(target.dataset.id ?? "");
      else this.openPaper(target.dataset.id ?? "");
      return;
    }
    // Arrow keys: ←/→ turn the table page; ↑/↓ move between rows, or between records while a drawer is open.
    const arrows = ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"];
    if (arrows.includes(e.key) && !e.altKey && !e.ctrlKey && !e.metaKey && !target?.matches("input,textarea,select") && !$("#modal-root .modal")) {
      if (this.drawer) {
        if (e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          this.switchDrawerItem(e.key === "ArrowDown" ? 1 : -1);
        }
        return;
      }
      if (this.page === "papers" || (this.page === "datasets" && this.datasetView === "table")) this.arrowTable(e);
      return;
    }
    if ((e.key === "/" || ((e.ctrlKey || e.metaKey) && e.key === "k")) && !target?.matches("input,textarea,select")) {
      e.preventDefault();
      $("#global-search")?.focus();
      return;
    }
    if (e.key === "Tab") {
      const container = $("#modal-root .modal") ?? $("#drawer-root .drawer");
      if (!container) return;
      const focusable = $$<HTMLElement>('button:not([disabled]), a[href], input:not([type=hidden]), select, textarea, [tabindex="0"]', container).filter((el) => el.getClientRects().length);
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!first || !last) return;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }

  /** screenKey handles the single-key shortcuts of screening mode; true when the key was one of them. */
  private screenKey(e: KeyboardEvent): boolean {
    const current = screenPapers(this)[this.screen!.index];
    if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      this.moveScreening(e.key === "ArrowRight" ? 1 : -1);
      return true;
    }
    if (!current) return false;
    const key = e.key.toLowerCase();
    const decision = DECISIONS.find(([k]) => k === key);
    if (decision) {
      this.decideInScreening(current.id, decision[1]);
      return true;
    }
    if (key === "s") {
      this.actions["toggle-star"]!(document.body, current.id);
      return true;
    }
    if (e.key === "Enter" && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLAnchorElement)) {
      this.openPaper(current.id);
      return true;
    }
    return false;
  }

  // ---- arrow-key table navigation ----

  private tableRows(datasets: boolean): HTMLElement[] {
    return $$(`${datasets ? ".datasets-panel" : ".papers-panel"} tbody tr[data-action]`);
  }

  /** arrowTable turns the page on ←/→ and moves the row cursor on ↑/↓ for the active table. */
  private arrowTable(e: KeyboardEvent): void {
    const datasets = this.page === "datasets";
    const rows = this.tableRows(datasets);
    if (!rows.length) return;
    const cursor = datasets ? this.datasetCursor : this.paperCursor;
    if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      const items = datasets
        ? filterDatasets(this.state.datasets, this.datasetSearch, (id) => this.paperById(id)?.title ?? "", this.datasetReview)
        : filterPapers(this.state.papers, this.filters);
      const size = datasets ? this.datasetPageSize : this.paperPageSize;
      const count = Math.max(1, Math.ceil(items.length / size));
      const next = (datasets ? this.datasetPage : this.paperPage) + (e.key === "ArrowRight" ? 1 : -1);
      if (next < 1 || next > count) return;
      e.preventDefault();
      if (datasets) this.datasetPage = next;
      else this.paperPage = next;
      this.render();
      // Keep the new page's header in view below the sticky topbar, then land on the row cursor.
      const table = $(datasets ? ".datasets-panel .table-wrap" : ".papers-panel .table-wrap");
      if (table) window.scrollTo({ top: window.scrollY + table.getBoundingClientRect().top - 66 });
      this.focusTableRow(datasets, cursor ?? 0);
      return;
    }
    e.preventDefault();
    const step = e.key === "ArrowDown" ? 1 : -1;
    const index = cursor == null ? (step > 0 ? 0 : rows.length - 1) : cursor + step;
    const clamped = Math.min(Math.max(index, 0), rows.length - 1);
    if (datasets) this.datasetCursor = clamped;
    else this.paperCursor = clamped;
    this.focusTableRow(datasets, clamped);
  }

  /** focusTableRow moves keyboard focus onto a visible row, so Enter and the focus ring land together. */
  private focusTableRow(datasets: boolean, index: number): void {
    const rows = this.tableRows(datasets);
    const row = rows[Math.min(Math.max(index, 0), rows.length - 1)];
    if (!row) return;
    rows.forEach((r) => r.classList.toggle("cursor-row", r === row));
    row.focus();
    row.scrollIntoView({ block: "nearest" });
  }
}

