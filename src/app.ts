import { matchingDatasets, datasetAssetActions, saveDatasetAssetForm, saveDatasetPlanForm } from "./dataset-assets.ts";
import { extractPaperMetadata, saveMetadata } from "./metadata.ts";
import { batchActions, batchChange, batchSearch, startBatch, type BatchDraft } from "./screen-batch.ts";
import { calibrationActions, calibrationChange, emptyCalibration, type CalibrationState } from "./calibration.ts";
import { pruneSelection, selectionActions, selectionChange } from "./selection.ts";
import { shortcutsModal } from "./shortcuts.ts";
import { icon } from "./icons.ts";
import { initUpdate, updateActions } from "./update.ts";
import type { ScreeningBatch } from "./screen-batch-queue.ts";
import { PAPER_COLUMNS, COLUMN_STORAGE_KEY, defaultPaperColumns, loadPaperColumns } from "./columns.ts";
// Application state, rendering and event delegation.
import type { Config, MCPInfo, Paper, SourceAutomation, SourceStatus, Status } from "./mygo.ts";
import { demoState } from "./demo-data.ts";
import { renderDrawer } from "./drawer.ts";
import type { Evidence } from "./mygo.ts";
import { doiActions } from "./doi.ts";
import { fileActions } from "./files.ts";
import {
  datasetModal, exclusionReasonModal, importSummaryModal, paperModal, resetDemoModal, resourceModal,
  saveDataset, saveDatasetLink, saveExclusionReason, savePaper, saveResource,
} from "./forms.ts";
import {
  PAGES, PAGE_SIZES, READING, SCREEN, SORT_KEYS, activePapers, clone, decideScreening, emptyFilters, esc, filterPapers, isPage, nowISO, paperDatasets, prepareImport,
  tasksFor, toggleSort, uid, type Filters, type ImportStats, type LibraryState, type Page, type Resource, type SortKey, type TaskRow, type TaskType,
} from "./model.ts";
import { applyProviderDefault, refreshSources, renderSources, saveAutomation, saveSourceCheck, saveSourceConfig, sourceConfigModal, sourceRecordsModal, syncSource, toggleSource, useSourceQuery, watchSources } from "./sources.ts";
import { reviewActions } from "./review.ts";
import { DECISIONS, applyDecision, askAIDisabled, renderScreening, saveScreenAI, screenAIModal, screenActions, screenModelReady, screenPapers, translateModelReady, translationPop, currentAbstractTranslation, updateScreenAbstract, type AbstractTranslation, updateScreenAI, updateScreeningContent, type ScreenState, type Translation } from "./screen.ts";
import { previewFontSize, renderSettings, saveModelProfile, saveSettings, settingsActions, showProxyURL, updateAIPlaceholders } from "./settings.ts";
import { afterAgentRender, agentActions, closeAgentPdf, emptyAgentState, renderAgent, sendAgentMessage, updateAgentContent, watchAgent, type AgentPdfState, type AgentState } from "./agent.ts";
import { watchFrames } from "./pdf-frame.ts";
import { Library } from "./mygo.ts";
import { enqueue, SaveRejected, type LibraryStore } from "./store.ts";
import { plantChoices } from "./research-options.ts";
import { $, $$, I, addMetadataChoice, selectPlant, downloadFile, errorMessage, speciesRow } from "./ui.ts";
import { exportModal, runExport } from "./export.ts";
import { renderDatasets, renderOverview, renderPapers, renderTasks } from "./views.ts";
import { DATASET_REVIEW_KEYS, datasetReviewActions, renderDatasetReview, reviewDatasets, type DatasetReviewState } from "./dataset-review.ts";
import { t } from "./i18n.ts";
import { datasetPickerMode, paperFieldKey, paperFieldModal, savePaperFields, searchDatasetPicker, selectPaperField, stepPaperField } from "./paper-fields.ts";
import { VIEWER_ACTIONS, VIEWER_FORMS, hasBackend, isAdmin, pageAllowed } from "./env.ts";
import type { WebSession } from "./web-runtime.ts";
import { initWebAuth, submitWebLogin, webActions } from "./web.ts";

/** Env describes the runtime: desktop window, web server or static preview; real vs demo data; and backend-provided info. See env.ts for what each allows. */
export interface Env {
  /** desktop: inside the mygo window, where native dialogs, the system shell and the app's updater exist. */
  desktop: boolean;
  /** backend: a Go backend answers calls, in the desktop app or from the web server. */
  backend: boolean;
  /** readOnly: a viewer of the web server; every change is blocked and the admin-only pages are hidden. */
  readOnly: boolean;
  /** web: what the web server said about this session; null in the desktop app and the static preview. */
  web: WebSession | null;
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
/** wideWindow: room for the screening queue rail beside the paper (the stylesheet folds it at 1250px). */
const wideWindow = (): boolean => typeof window !== "undefined" && window.innerWidth > 1250;
/** Pages whose heading shows no "import / add paper" buttons, because they are not about the library's records. */
export const HEADING_ACTIONS_HIDDEN: ReadonlySet<Page> = new Set<Page>(["settings", "agent", "sources"]);

export class App {
  state: LibraryState;
  env: Env;
  filters: Filters = emptyFilters();
  taskFilter: TaskType | "all" = "all";
  datasetSearch = "";
  datasetPreparation = "";
  /** Datasets page: only those a source update added that no person has checked. */
  datasetReview = false;
  /** Papers-table pagination; the stored page is clamped against the filtered total at render time. */
  paperPage = 1;
  paperColumns = loadPaperColumns();
  paperPageSize = PAGE_SIZES.includes(Number(localStorage.getItem("bibliocell-page-size"))) ? Number(localStorage.getItem("bibliocell-page-size")) : PAGE_SIZES[0]!;
  /** Row the arrow keys last landed on, per table; null until the first ↑/↓ press. */
  paperCursor: number | null = null;
  /** Papers ticked in the table, for the bulk actions bar; kept across filters and pages until cleared. */
  selectedPapers = new Set<string>();
  datasetCursor: number | null = null;
  /** Datasets page: table vs card view plus the same pagination pattern. */
  datasetView: "cards" | "table" = localStorage.getItem("bibliocell-datasets-view") === "cards" ? "cards" : "table";
  datasetPage = 1;
  datasetPageSize = PAGE_SIZES.includes(Number(localStorage.getItem("bibliocell-datasets-page-size"))) ? Number(localStorage.getItem("bibliocell-datasets-page-size")) : PAGE_SIZES[0]!;
  drawer: Drawer | null = null;
  /** Datasets page: review mode shows one dataset at a time instead of the list. */
  datasetScreen: DatasetReviewState | null = null;
  /** Papers page: screening mode shows one paper at a time instead of the table. */
  screen: ScreenState | null = null;
  batch: ScreeningBatch | null = null;
  batchDraft: BatchDraft | null = null;
  /** Papers page: the AI comparison tab, with its filter and the last report read. */
  calibrationView = false;
  calibration: CalibrationState = emptyCalibration();
  /** The translation card floating by the selection in screening mode, if any. */
  translation: Translation | null = null;
  abstractTranslations = new Map<string, AbstractTranslation>();
  /** Screening mode shows the stored PDF beside the paper unless hidden. */
  screenPdf = localStorage.getItem("bibliocell-screen-pdf") !== "off";
  overviewHidePreprint = localStorage.getItem("bibliocell-overview-hide-preprint") === "on";
  /** Screening statuses the overview statistics count; empty counts every status. */
  overviewScreening: string[] = (localStorage.getItem("bibliocell-overview-screening") ?? "included").split(",").filter((s) => s in SCREEN && s !== "irrelevant");
  /** Screening mode: the queue rail is open by default on wide windows; narrow ones show it as an overlay on request. */
  screenQueueOpen = wideWindow();
  private translationAnchor: DOMRect | null = null;
  private selectionTimer: ReturnType<typeof setTimeout> | undefined;
  agent: AgentState = emptyAgentState();
  /** The assistant page: the PDF open beside the conversation for a citation, if any. */
  agentPdf: AgentPdfState | null = null;
  drawerTab = "overview";
  syncing = new Set<string>();
  togglingSources = new Set<string>();
  /** Papers a source update added whose AI second check is still running. */
  sourceCheckPending = 0;
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
  private dismissing = false;
  private contextRow: HTMLElement | null = null;

  constructor(options: AppOptions) {
    this.store = options.store;
    this.env = options.env;
    this.state = options.initial;
    this.saved = clone(options.initial);
    this.warning = options.warning ?? "";
    const hash = location.hash.slice(1);
    this.page = isPage(hash) && pageAllowed(this.env, hash) ? hash : "overview";
    if (this.env.real) document.body.classList.add("real-mode");
    if (this.env.desktop) document.body.classList.add("desktop");
    if (this.env.readOnly) document.body.classList.add("read-only");
  }

  start(): void {
    document.addEventListener("contextmenu", (e) => {
      const row = e.target instanceof Element ? e.target.closest<HTMLElement>('tr[data-action="open-paper"], tr[data-action="open-dataset"]') : null;
      if (!row || this.drawer || $("#modal-root .modal") || this.env.readOnly) return;
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
      const dataset = row.dataset.action === "open-dataset";
      button.textContent = dataset ? t("删除数据集") : t("删除文献");
      button.setAttribute("role", "menuitem");
      button.dataset.action = dataset ? "delete-dataset" : "delete-paper";
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
    // A keyboard button in the top bar opens the same shortcut sheet as the key, for people who never press it.
    const topbar = $(".topbar-right");
    if (topbar && !$("#shortcuts-help")) {
      const help = document.createElement("button");
      help.type = "button";
      help.id = "shortcuts-help";
      help.className = "topbar-link";
      help.dataset.action = "shortcuts-help";
      help.innerHTML = icon("keyboard", 20);
      help.title = t("快捷键");
      help.setAttribute("aria-label", t("快捷键"));
      topbar.prepend(help);
    }
    initUpdate(this);
    initWebAuth(this);
    // Selecting text while screening translates it: the card appears once the selection settles.
    document.addEventListener("selectionchange", () => {
      clearTimeout(this.selectionTimer);
      this.selectionTimer = setTimeout(() => this.onSelectionSettled(), 450);
    });
    window.addEventListener("resize", () => this.placeTranslationPop());
    window.addEventListener("hashchange", () => {
      const next = location.hash.slice(1);
      if (isPage(next) && pageAllowed(this.env, next)) {
        this.page = next;
        this.render();
      }
    });
    // Agents write while the window sits in the background; show their work on return. Sources are the admin's to reread.
    if (this.env.real) window.addEventListener("focus", () => void (isAdmin(this.env) ? refreshSources(this) : Promise.resolve()).then(() => this.refresh()));
    // Background updates and agent progress are the admin's to follow; the web server streams them only to admins.
    if (isAdmin(this.env)) {
      watchSources(this);
      watchAgent(this);
      watchFrames();
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

  /** dismissPaper marks a paper irrelevant and drops its stored attachments. The record
   * stays, which is what keeps imports and source updates from adding the paper back;
   * no default view or count shows it afterwards. In screening the run moves on. */
  async dismissPaper(id: string): Promise<void> {
    const p = this.paperById(id);
    if (!p || this.env.readOnly || this.dismissing) return;
    this.dismissing = true;
    try {
      if (this.env.real) await enqueue(() => Library.dismissPaper(id));
      const next = clone(this.state);
      const fileIds = new Set(next.resources.filter((r) => r.paperId === id).map((r) => r.id));
      next.resources = next.resources.filter((r) => r.paperId !== id);
      next.evidence = (next.evidence ?? []).filter((e) => !fileIds.has(e.fileId));
      const paper = next.papers.find((x) => x.id === id)!;
      decideScreening(paper, "irrelevant");
      paper.fulltext = "missing";
      paper.fulltextUrl = "";
      if (!this.env.real) await this.store.save(next, this.saved);
      this.state = next;
      this.saved = clone(next);
      this.edits++;
      this.selectedPapers.delete(id);
      if (this.screen?.ids.includes(id)) {
        // Taken out of the run before rendering, so the paper that followed it is on screen.
        this.screen.dismissed = (this.screen.dismissed ?? 0) + 1;
        this.screen.ids = this.screen.ids.filter((x) => x !== id);
      }
      if (this.drawer?.type === "paper" && this.drawer.id === id) this.closeDrawer();
      this.logActivity(t("标为无关：{title}", { title: p.title }));
      this.render();
      if (this.screen) window.scrollTo({ top: 0 });
      if (this.env.real) await this.reload();
      this.toast(t("已标为无关，保存的附件已清理。"));
    } catch (error) {
      this.toast(errorMessage(error, t("操作失败，请重试。")), true);
    } finally {
      this.dismissing = false;
    }
  }

  private requestDeletePaper(id: string): void {
    const paper = this.paperById(id);
    if (!paper) return;
    this.showModal(t("删除文献"), `<p>${t("确定删除《{title}》？", { title: esc(paper.title) })}</p><p>${t("将删除文献、笔记、相关证据和保存的附件副本，并解除数据集关联。数据集及原始导入文件会保留。此操作无法撤销。")}</p>`, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-danger" data-action="confirm-delete-paper" data-id="${esc(id)}">${t("确认删除")}</button>`);
  }

  private deletePaper(id: string, button: HTMLElement): Promise<void> {
    return this.deletePapers([id], button);
  }

  /** deletePapers removes papers with their notes, evidence and attachment copies; datasets stay, unlinked. */
  async deletePapers(ids: string[], button: HTMLElement): Promise<void> {
    ids = ids.filter((id) => this.paperById(id));
    if (this.deleting || !ids.length) return;
    this.deleting = true;
    button.setAttribute("disabled", "");
    try {
      if (this.env.real) for (const id of ids) await enqueue(() => Library.deletePaper(id));
      const gone = new Set(ids);
      const next = clone(this.state);
      const fileIds = new Set(next.resources.filter(r => gone.has(r.paperId ?? "")).map(r => r.id));
      next.papers = next.papers.filter(p => !gone.has(p.id));
      next.resources = next.resources.filter(r => !gone.has(r.paperId ?? ""));
      next.evidence = (next.evidence ?? []).filter(e => !gone.has(e.paperId ?? "") && !fileIds.has(e.fileId));
      next.datasets.forEach(d => { d.paperLinks = d.paperLinks.filter(l => !gone.has(l.paperId)); });
      if (!this.env.real) await this.store.save(next, this.saved);
      this.state = next;
      this.saved = clone(next);
      this.edits++;
      // Counted before rendering, which drops the deleted papers from the run.
      if (this.screen) this.screen.deleted = (this.screen.deleted ?? 0) + ids.filter((id) => this.screen!.ids.includes(id)).length;
      for (const id of ids) this.selectedPapers.delete(id);
      this.closeModal();
      if (this.drawer?.type === "paper" && gone.has(this.drawer.id)) this.closeDrawer();
      this.render();
      if (this.screen) window.scrollTo({ top: 0 }); // The next paper of the run, from its top.
      if (this.env.real) await this.reload();
      this.toast(ids.length === 1 ? t("文献已删除。") : t("已删除 {n} 篇文献。", { n: ids.length }));
    } catch (error) {
      this.toast(errorMessage(error, t("删除失败，请重试。")), true);
    } finally {
      this.deleting = false;
      button.removeAttribute("disabled");
    }
  }

  private requestDeleteDataset(id: string): void {
    const dataset = this.state.datasets.find((d) => d.id === id);
    if (!dataset) return;
    this.showModal(t("删除数据集"), `<p>${t("确定删除数据集《{title}》？", { title: esc(dataset.title) })}</p><p>${t("将删除数据集、它与文献的关联、相关证据和只属于它的资料入口。文献会保留。来源更新不会再把同一编号加回来。此操作无法撤销。")}</p>`, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-danger" data-action="confirm-delete-dataset" data-id="${esc(id)}">${t("确认删除")}</button>`);
  }

  /** deleteDatasets removes datasets with their paper links, evidence and resource entries; papers stay. */
  async deleteDatasets(ids: string[], button: HTMLElement): Promise<void> {
    ids = ids.filter((id) => this.state.datasets.some((d) => d.id === id));
    if (this.deleting || !ids.length) return;
    this.deleting = true;
    button.setAttribute("disabled", "");
    try {
      if (this.env.real) for (const id of ids) await enqueue(() => Library.deleteDataset(id));
      const gone = new Set(ids);
      const next = clone(this.state);
      next.datasets = next.datasets.filter((d) => !gone.has(d.id));
      next.resources = next.resources.filter((r) => !gone.has(r.datasetId ?? ""));
      next.evidence = (next.evidence ?? []).filter((e) => !gone.has(e.datasetId ?? ""));
      if (!this.env.real) await this.store.save(next, this.saved);
      this.state = next;
      this.saved = clone(next);
      this.edits++;
      if (this.datasetScreen) this.datasetScreen.deleted += ids.length;
      this.closeModal();
      if (this.drawer?.type === "dataset" && gone.has(this.drawer.id)) this.closeDrawer();
      this.render();
      if (this.env.real) await this.reload();
      this.toast(ids.length === 1 ? t("数据集已删除。") : t("已删除 {n} 组数据集。", { n: ids.length }));
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
    if (this.env.readOnly) {
      // Every path here is gated before the state changes; should one slip through, the database wins.
      this.toast(t("只读访问：登录管理员后才能修改。"), true);
      void this.reload();
      return;
    }
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
      datasets: () => (this.datasetScreen ? renderDatasetReview(this) : renderDatasets(this)),
      tasks: () => renderTasks(this),
      sources: () => renderSources(this),
      agent: () => renderAgent(this),
      settings: () => renderSettings(this),
    };
    if (this.page !== "papers") this.screen = null; // Screening belongs to the papers list it started from.
    if (this.page !== "datasets") this.datasetScreen = null;
    if (!this.screen) this.clearTranslation();
    pruneSelection(this);
    const html = renderers[this.page]();
    if (this.page === "agent") updateAgentContent(content, html); // Keeps the cited PDF loaded across repaints.
    else updateScreeningContent(content, html);
    if (this.screen) {
      const current = screenPapers(this)[this.screen.index];
      if (current) {
        this.loadCachedAbstract(current);
        if (this.screen.ai) this.requestJudgement(current.id);
      }
      $(".screen-queue-item[aria-current]")?.scrollIntoView({ block: "nearest" });
    }
    if (this.datasetScreen) $(".screen-queue-item[aria-current]")?.scrollIntoView({ block: "nearest" });
    document.body.classList.toggle("page-agent", this.page === "agent"); // The assistant page is the chat window itself.
    if (this.page === "agent") afterAgentRender(this);
    // "Import records / Add paper" belong to the library pages; settings, sources and the assistant have their own actions.
    const actions = $(".heading-actions");
    if (actions) actions.hidden = HEADING_ACTIONS_HIDDEN.has(this.page);
    $$<HTMLElement>("[data-overview-only]").forEach((el) => { el.hidden = this.page !== "overview"; }); // The sidebar already reaches the assistant elsewhere.
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
    const counts: [string, number][] = [["#count-papers", activePapers(this.state.papers).length], ["#count-datasets", this.state.datasets.length], ["#count-tasks", this.tasks().length]];
    for (const [selector, value] of counts) {
      const el = $(selector);
      if (el) el.textContent = String(value);
    }
  }

  navigate(next: string): void {
    if (!isPage(next) || !pageAllowed(this.env, next)) return;
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
        ? matchingDatasets(this)
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
    this.calibrationView = false;
    const ids = filterPapers(this.state.papers, this.filters).map((p) => p.id);
    const index = this.paperCursor == null ? 0 : (this.paperPage - 1) * this.paperPageSize + this.paperCursor;
    this.screen = { ids, index: Math.min(Math.max(index, 0), Math.max(ids.length - 1, 0)), ai: null };
    this.screenQueueOpen = wideWindow(); // Measured now: the window may have been resized since the app started.
    this.closeDrawer();
    this.render();
    window.scrollTo({ top: 0 });
    $("#app-content")?.focus({ preventScroll: true });
    // Offer suggestions once per entry, only where a model service can be reached; a standing choice turns the question off.
    if (ids.length && this.canScreenWithAI() && !askAIDisabled()) screenAIModal(this);
  }
  /** resumeScreening opens screening on the whole library at the first paper still undecided. */
  resumeScreening(): void {
    const list = filterPapers(this.state.papers, emptyFilters());
    const index = list.findIndex((p) => p.screening === "pending");
    if (index < 0) {
      this.toast(t("没有待判断的文献。"));
      return;
    }
    this.filters = emptyFilters();
    this.calibrationView = false;
    this.paperPage = Math.floor(index / this.paperPageSize) + 1;
    this.paperCursor = index % this.paperPageSize;
    this.navigate("papers");
    this.startScreening();
  }
  /** canScreenWithAI: a model is bound to screening and the backend can reach it. */
  canScreenWithAI(): boolean {
    return screenModelReady(this);
  }
  canTranslate(): boolean {
    return translateModelReady(this);
  }
  /** screeningSelection returns the text selected inside the screening page, with where it sits. */
  private screeningSelection(): { text: string; rect: DOMRect } | null {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || !selection.rangeCount) return null;
    const range = selection.getRangeAt(0);
    const body = $(".screen-body");
    if (!body || !body.contains(range.commonAncestorContainer)) return null;
    const text = selection.toString().trim();
    return text ? { text, rect: range.getBoundingClientRect() } : null;
  }
  /** onSelectionSettled translates what was just selected, once per selection. */
  private onSelectionSettled(): void {
    if (!this.screen || this.page !== "papers" || !this.canTranslate() || $("#modal-root .modal") || this.drawer) return;
    const current = screenPapers(this)[this.screen.index];
    const selected = this.screeningSelection();
    if (!current || !selected || selected.text.length < 2) return;
    if (this.translation?.paperId === current.id && this.translation.source === selected.text) return;
    this.translate(current.id, selected.text, selected.rect);
  }
  /** Restore a saved translation on entry, keeping the original visible until toggled. */
  private loadCachedAbstract(paper: Paper): void {
    if (!hasBackend(this.env) || !paper.abstract.trim() || currentAbstractTranslation(this, paper)) return;
    const result: AbstractTranslation = { paperId: paper.id, source: paper.abstract.trim(), prompt: this.env.settings.translationPrompt ?? "", showOriginal: true, state: "checking" };
    this.abstractTranslations.set(paper.id, result);
    Library.cachedTranslation(result.source).then((text) => {
      if (this.abstractTranslations.get(paper.id) !== result) return;
      this.abstractTranslations.set(paper.id, text ? { ...result, state: "done", text } : { ...result, state: "uncached" });
      updateScreenAbstract(this, paper.id);
    }, () => {
      if (this.abstractTranslations.get(paper.id) === result) this.abstractTranslations.set(paper.id, { ...result, state: "uncached" });
    });
  }
  /** Translate the original abstract in place; cache it for switching back and forth. */
  translateInScreening(id: string): void {
    const p = this.paperById(id);
    if (!p || !this.canTranslate()) return;
    if (!p.abstract.trim()) {
      this.toast(t("先划选要翻译的文字，或为文献补充摘要。"), true);
      return;
    }
    this.clearTranslation();
    const cached = currentAbstractTranslation(this, p);
    if (cached?.state === "loading") return;
    if (cached?.state === "done") {
      cached.showOriginal = false;
      updateScreenAbstract(this, id);
      return;
    }
    const result: AbstractTranslation = { paperId: id, source: p.abstract.trim(), prompt: this.env.settings.translationPrompt ?? "", showOriginal: false, state: "loading" };
    this.abstractTranslations.set(id, result);
    updateScreenAbstract(this, id);
    Library.translateText(result.source).then((text) => {
      if (this.abstractTranslations.get(id) !== result) return;
      this.abstractTranslations.set(id, { ...result, state: "done", text });
      updateScreenAbstract(this, id);
    }, (error: unknown) => {
      if (this.abstractTranslations.get(id) !== result) return;
      this.abstractTranslations.set(id, { ...result, state: "error", message: errorMessage(error, t("翻译失败，请重试。")) });
      updateScreenAbstract(this, id);
    });
  }
  toggleAbstractTranslation(id: string): void {
    const paper = this.paperById(id);
    const result = paper && currentAbstractTranslation(this, paper);
    if (result?.state !== "done") return;
    result.showOriginal = !result.showOriginal;
    updateScreenAbstract(this, id);
  }
  retrySelectionTranslation(): void {
    if (this.translation && this.translationAnchor) this.translate(this.translation.paperId, this.translation.source, this.translationAnchor);
  }
  private translate(id: string, source: string, anchor: DOMRect): void {
    const translation: Translation = { paperId: id, source, state: "loading" };
    this.translation = translation;
    this.translationAnchor = anchor;
    this.renderTranslationPop();
    Library.translateText(source).then((text) => {
      if (this.translation !== translation) return; // A newer selection or a closed card replaced it.
      this.translation = { paperId: id, source, state: "done", text };
      this.renderTranslationPop();
    }, (error: unknown) => {
      if (this.translation !== translation) return;
      this.translation = { paperId: id, source, state: "error", message: errorMessage(error, t("翻译失败，请重试。")) };
      this.renderTranslationPop();
    });
  }
  clearTranslation(): void {
    this.translation = null;
    this.translationAnchor = null;
    $("#translate-pop")?.remove();
  }
  /** renderTranslationPop draws the card outside the page content, so page re-renders leave it alone. */
  private renderTranslationPop(): void {
    let pop = $("#translate-pop");
    if (!this.translation) {
      pop?.remove();
      return;
    }
    if (!pop) {
      pop = document.createElement("div");
      pop.id = "translate-pop";
      pop.className = "translate-pop";
      pop.setAttribute("role", "dialog");
      pop.setAttribute("aria-label", t("译文"));
      document.body.appendChild(pop);
    }
    pop.innerHTML = translationPop(this.translation);
    this.placeTranslationPop();
  }
  /** placeTranslationPop puts the card under the selection, or above it near the bottom of the window. */
  private placeTranslationPop(): void {
    const pop = $("#translate-pop");
    const rect = this.translationAnchor;
    if (!pop || !rect) return;
    const margin = 12;
    const width = Math.min(460, innerWidth - margin * 2);
    pop.style.width = `${width}px`;
    const left = Math.min(Math.max(rect.left, margin), innerWidth - width - margin);
    const below = rect.bottom + 8;
    const height = pop.offsetHeight;
    const top = below + height <= innerHeight - margin ? below : Math.max(margin, rect.top - height - 8);
    pop.style.left = `${left}px`;
    pop.style.top = `${top}px`;
  }
  toggleScreenPdf(): void {
    this.screenPdf = !this.screenPdf;
    localStorage.setItem("bibliocell-screen-pdf", this.screenPdf ? "on" : "off");
    this.render();
  }
  enableScreeningAI(requirement: string): void {
    if (!this.screen) return;
    this.screen.ai = { requirement, results: new Map() };
    this.render();
  }
  disableScreeningAI(): void {
    if (!this.screen) return;
    this.screen.ai = null;
    this.render();
  }
  /** requestJudgement asks the model about one paper, once; only the paper on screen is ever asked for. */
  requestJudgement(id: string, retry = false): void {
    const ai = this.screen?.ai;
    const p = this.paperById(id);
    if (!ai || !p) return;
    if (ai.results.has(id) && !retry) return;
    ai.results.set(id, { state: "loading" });
    if (retry) updateScreenAI(this, id);
    Library.screenPaper(id, ai.requirement).then((judgement) => {
      ai.results.set(id, { state: "done", judgement });
    }, (error: unknown) => {
      ai.results.set(id, { state: "error", message: errorMessage(error, t("AI 判断失败，请重试。")) });
    }).then(() => {
      // Show the answer only if this run is still on and that paper is still on screen.
      if (this.screen?.ai === ai && screenPapers(this)[this.screen.index]?.id === id) updateScreenAI(this, id);
    });
  }
  /** adoptJudgement records the model's verdict as the person's decision, with the model's reason as the basis. */
  adoptJudgement(id: string): void {
    const result = this.screen?.ai?.results.get(id);
    const p = this.paperById(id);
    if (!p || !result || result.state !== "done" || result.judgement.skipped) return;
    const { verdict, reason } = result.judgement;
    if (!applyDecision(this, p, verdict, (paper) => exclusionReasonModal(this, paper), reason || p.screeningReason)) return;
    this.toast(t("已采纳 AI 建议：{label}", { label: SCREEN[verdict] ?? verdict }));
    this.screeningDecided();
  }
  exitScreening(): void {
    if (!this.screen) return;
    this.clearTranslation();
    const id = screenPapers(this)[this.screen.index]?.id;
    const returnTo = this.screen.returnTo;
    this.screen = null;
    this.closeDrawer();
    if (returnTo === "calibration") {
      // Back to the comparison, which rereads so a decision made here shows at once.
      this.calibrationView = true;
      this.render();
      void this.reload().then(() => { this.actions["calibration-refresh"]!(document.body, ""); });
      return;
    }
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
    this.goToScreening(this.screen.index + delta);
  }
  /** goToScreening opens the paper at a position of the run, from the arrows or the queue rail. */
  goToScreening(index: number): void {
    if (!this.screen) return;
    const papers = screenPapers(this);
    if (!Number.isInteger(index) || index < 0 || index >= papers.length || index === this.screen.index) return;
    this.screen.index = index;
    this.clearTranslation();
    // On a narrow window the rail is an overlay: picking a paper there closes it.
    if (this.screenQueueOpen && !wideWindow()) this.screenQueueOpen = false;
    this.render();
    window.scrollTo({ top: 0 });
  }
  toggleScreenQueue(): void {
    this.screenQueueOpen = !this.screenQueueOpen;
    this.render();
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

  // ---- dataset review mode ----
  /** startDatasetReview snapshots the filtered datasets and opens them one at a time, from the row cursor if any. */
  startDatasetReview(): void {
    const ids = matchingDatasets(this).map((d) => d.id);
    const index = this.datasetView === "table" && this.datasetCursor != null ? (this.datasetPage - 1) * this.datasetPageSize + this.datasetCursor : 0;
    this.datasetScreen = { ids, index: Math.min(Math.max(index, 0), Math.max(ids.length - 1, 0)), deleted: 0 };
    this.screenQueueOpen = wideWindow();
    this.closeDrawer();
    this.render();
    window.scrollTo({ top: 0 });
    $("#app-content")?.focus({ preventScroll: true });
  }
  exitDatasetReview(): void {
    if (!this.datasetScreen) return;
    const id = reviewDatasets(this)[this.datasetScreen.index]?.id;
    this.datasetScreen = null;
    this.closeDrawer();
    if (id && this.datasetView === "table") {
      const index = matchingDatasets(this).findIndex((d) => d.id === id);
      if (index >= 0) {
        this.datasetPage = Math.floor(index / this.datasetPageSize) + 1;
        this.datasetCursor = index % this.datasetPageSize;
      }
    }
    this.render();
    if (this.datasetCursor != null && this.datasetView === "table") this.focusTableRow(true, this.datasetCursor);
  }
  moveDatasetReview(delta: number): void {
    if (this.datasetScreen) this.goToDatasetReview(this.datasetScreen.index + delta);
  }
  goToDatasetReview(index: number): void {
    if (!this.datasetScreen) return;
    const datasets = reviewDatasets(this);
    if (!Number.isInteger(index) || index < 0 || index >= datasets.length || index === this.datasetScreen.index) return;
    this.datasetScreen.index = index;
    if (this.screenQueueOpen && !wideWindow()) this.screenQueueOpen = false;
    this.render();
    window.scrollTo({ top: 0 });
  }
  /** keepInDatasetReview marks the dataset reviewed when a source update added it, then moves on. */
  async keepInDatasetReview(id: string): Promise<void> {
    const dataset = this.state.datasets.find((d) => d.id === id);
    if (!dataset || !this.datasetScreen) return;
    const last = this.datasetScreen.index >= reviewDatasets(this).length - 1;
    if (!dataset.awaitingReview) {
      if (!last) this.moveDatasetReview(1);
      return;
    }
    try {
      if (this.env.real) await enqueue(() => Library.markDatasetReviewed(id));
      dataset.awaitingReview = false;
      const saved = this.saved.datasets.find((d) => d.id === id);
      if (this.env.real && saved) saved.awaitingReview = false; // Already in SQLite; not an edit for save().
      else this.persist();
      this.toast(t("已保留，核查完成。"));
      if (!last) this.moveDatasetReview(1);
      else this.render();
    } catch (error) {
      this.toast(t("保存失败：{reason}", { reason: errorMessage(error, t("请重试。")) }), true);
    }
  }
  /** datasetReviewKey handles the single-key shortcuts of dataset review; true when the key was one of them. */
  private datasetReviewKey(e: KeyboardEvent): boolean {
    if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      this.moveDatasetReview(e.key === "ArrowRight" ? 1 : -1);
      return true;
    }
    const current = reviewDatasets(this)[this.datasetScreen!.index];
    if (!current) return false;
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
    const has = (keys: readonly string[]): boolean => keys.includes(key);
    if (has(DATASET_REVIEW_KEYS.keep)) {
      void this.keepInDatasetReview(current.id);
      return true;
    }
    if (has(DATASET_REVIEW_KEYS.remove)) {
      void this.deleteDatasets([current.id], document.createElement("button"));
      return true;
    }
    if (has(DATASET_REVIEW_KEYS.edit)) {
      datasetModal(this, current.id);
      return true;
    }
    if (e.key === "Enter" && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLAnchorElement)) {
      this.openDataset(current.id);
      return true;
    }
    return false;
  }

  // ---- modal ----
  showModal(title: string, body: string, footer: string, wide = false): void {
    this.modalLastFocus = document.activeElement;
    const root = $("#modal-root");
    if (!root) return;
    root.innerHTML = `<div class="modal-backdrop" data-action="close-modal"></div><section class="modal${wide ? " modal-wide" : ""}" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-header"><h2 id="modal-title">${esc(title)}</h2><button class="btn-icon" data-action="close-modal" aria-label="${t("关闭弹窗")}">${I("x", 20)}</button></div><div class="modal-body">${body}</div><div class="modal-footer">${footer}</div></section>`;
    document.body.classList.add("modal-open");
    requestAnimationFrame(() => {
      if (root.contains(document.activeElement)) return;
      const visible = (selector: string) => $$<HTMLElement>(selector, root).find(el => !el.closest("[hidden]") && !el.matches(":disabled"));
      const target = visible("input:not([type=hidden])") ?? visible("textarea") ?? visible("select") ?? visible("button");
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
    "edit-paper-field": (el, id) => paperFieldModal(this, id, el.dataset.field),
    "select-paper-field": (el) => selectPaperField(el.dataset.field ?? "species"),
    "step-paper-field": (el) => stepPaperField(Number(el.dataset.step) || 1),
    "field-dataset-mode": (el) => datasetPickerMode(el.dataset.mode ?? "existing"),
    "configure-source": (_el, id) => sourceConfigModal(this, id),
    "sync-source": (_el, id) => void syncSource(this, id),
    "source-records": (_el, id) => sourceRecordsModal(this, id),
    "use-source-query": (el) => useSourceQuery(el),
    "extract-paper-metadata": (_el, id) => { void extractPaperMetadata(this, id); },
    "add-metadata-choice": (el) => addMetadataChoice(el),
    "add-species-row": () => {
      const rows = $("#species-rows");
      if (!rows) return;
      rows.insertAdjacentHTML("beforeend", speciesRow({}, plantChoices(this.state)));
      $$<HTMLElement>(".species-row", rows).at(-1)?.querySelector<HTMLSelectElement>("[data-species-picker]")?.focus();
    },
    "remove-species-row": (el) => el.closest(".species-row")?.remove(),
    "delete-paper": (_el, id) => this.requestDeletePaper(id),
    "delete-dataset": (_el, id) => this.requestDeleteDataset(id),
    "confirm-delete-dataset": (el, id) => { void this.deleteDatasets([id], el); },
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
    "export-papers": () => exportModal(this, "papers"),
    "export-datasets": () => exportModal(this, "datasets"),
    "export-report": () => exportModal(this, "papers", "report"),
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
    "clear-filter": (el) => {
      const key = el.dataset.filterKey as keyof Filters | undefined;
      if (!key || key === "starred" || !(key in this.filters)) return;
      this.filters[key] = "";
      this.paperPage = 1;
      this.render();
    },
    "clear-filters": () => {
      this.datasetSearch = "";
      this.datasetPreparation = "";
      this.datasetPage = 1;
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
      this.calibrationView = false;
      this.paperPage = 1;
      this.render();
    },
    "starred-papers": () => {
      this.filters.starred = true;
      this.filters.review = "";
      this.calibrationView = false;
      this.paperPage = 1;
      this.render();
    },
    "review-papers": () => {
      clearTimeout(this.searchTimer);
      this.calibrationView = false;
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
    "shortcuts-help": () => shortcutsModal(this),
    "resume-screening": () => this.resumeScreening(),
    "review-papers-nav": () => { this.navigate("papers"); this.actions["review-papers"]!(document.body, ""); },
    "stat-nav": (el) => {
      const target = el.dataset.target ?? "";
      if (target === "screen" || target === "fulltext") {
        this.taskFilter = target;
        this.navigate("tasks");
      } else {
        this.navigate(target);
      }
    },
    "overview-filter": (el) => {
      const field = el.dataset.field as "species" | "modality" | "organ" | "year" | "articleType" | undefined;
      const value = el.dataset.value ?? "";
      if (!field || !value) return;
      // With preprints left out of the charts, the library shows the same papers the bar counted.
      // The library filters one screening status at a time, so a single chosen status carries over.
      const screening = this.overviewScreening.length === 1 ? this.overviewScreening[0]! : "";
      this.filters = { ...emptyFilters(), [field]: value, screening, preprint: this.overviewHidePreprint ? "no" : "", sort: this.filters.sort };
      this.paperPage = 1;
      this.navigate("papers");
    },
    "new-dataset": () => datasetModal(this),
    "edit-dataset": (_el, id) => datasetModal(this, id),
    "new-dataset-for-paper": (_el, id) => {
      this.closeModal();
      datasetModal(this, "", id);
    },
    "link-dataset": (_el, id) => paperFieldModal(this, id, "datasets"),
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
    ...datasetAssetActions(this),
    ...doiActions(this),
    ...reviewActions(this),
    ...screenActions(this),
    ...datasetReviewActions(this),
    ...batchActions(this),
    ...calibrationActions(this),
    ...selectionActions(this),
    ...updateActions(this),
    ...webActions(this),
  };

  private onClick(e: MouseEvent): void {
    const target = e.target;
    this.closeContextMenu();
    if (!(target instanceof Element)) return;
    // A click away from the translation card closes it, unless it is making a new selection.
    if (this.translation && !target.closest("#translate-pop") && !target.closest("[data-action=screen-translate]") && window.getSelection()?.isCollapsed) this.clearTranslation();
    // Anchors navigate by themselves; the row or card around them must not also open.
    if (target.closest("a[href]")) return;
    // The checkbox column ticks a row; the row around it must not open.
    if (target.closest(".select-cell, .col-select")) return;
    const el = target.closest<HTMLElement>("[data-action]");
    if (el) {
      const action = el.dataset.action ?? "";
      if (this.env.real && DEMO_ONLY_ACTIONS.has(action)) {
        this.toast(t("真实模式暂不支持资料链接和 JSON 导入；文献文件可在详情的“数据与资料”中导入。"), true);
        return;
      }
      if (this.env.readOnly && !VIEWER_ACTIONS.has(action)) {
        this.toast(t("只读访问：登录管理员后才能修改。"), true);
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
      "dataset-asset-form": saveDatasetAssetForm, "dataset-sample-form": saveDatasetPlanForm, "dataset-reference-form": saveDatasetPlanForm,
      "batch-screen-form": startBatch, "export-form": runExport,
      "metadata-apply-form": saveMetadata, "paper-form": savePaper, "paper-fields-form": savePaperFields, "source-config-form": saveSourceConfig, "settings-form": saveSettings, "agent-form": sendAgentMessage,
      "exclusion-reason-form": saveExclusionReason, "screen-ai-form": saveScreenAI, "model-profile-form": saveModelProfile, "dataset-form": saveDataset, "resource-form": saveResource, "link-dataset-form": saveDatasetLink,
      "web-login-form": submitWebLogin, "source-check-form": (app) => saveSourceCheck(app),
    };
    // getAttribute: an <input name="id"> inside the form shadows form.id.
    const id = form.getAttribute("id") ?? "";
    const handler = handlers[id];
    if (!handler) return;
    e.preventDefault();
    if (this.env.readOnly && !VIEWER_FORMS.has(id)) {
      this.toast(t("只读访问：登录管理员后才能修改。"), true);
      return;
    }
    handler(this, form);
  }

  private onChange(e: Event): void {
    const target = e.target;
    if (!(target instanceof HTMLElement)) return;
    if (batchChange(this, target)) return;
    if (target instanceof HTMLSelectElement && target.id === "dataset-preparation-filter") {
      this.datasetPreparation = target.value;
      this.datasetPage = 1;
      this.render();
      return;
    }
    if (calibrationChange(this, target)) return;
    if (selectionChange(this, target)) return;
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
    if (target instanceof HTMLSelectElement && target.matches("[data-species-picker]")) { selectPlant(target); return; }
    if (target instanceof HTMLSelectElement && target.matches('#source-config-form [name="provider"]')) {
      applyProviderDefault(target);
      return;
    }
    if (target.id === "model-protocol" && target instanceof HTMLSelectElement) {
      updateAIPlaceholders(target);
      return;
    }
    if (target.id === "settings-proxy-mode" && target instanceof HTMLSelectElement) {
      showProxyURL(target);
      return;
    }
    if (target instanceof HTMLInputElement && target.dataset.sourceToggle) {
      toggleSource(this, target);
      return;
    }
    if (target.id === "source-check-enabled") {
      saveSourceCheck(this);
      return;
    }
    if (target.id === "automation-enabled" || target.id === "automation-interval") {
      saveAutomation(this);
      return;
    }
    if (target instanceof HTMLInputElement && target.dataset.overviewScreening) {
      const status = target.dataset.overviewScreening;
      const chosen = new Set(this.overviewScreening);
      if (target.checked) chosen.add(status); else chosen.delete(status);
      this.overviewScreening = Object.keys(SCREEN).filter((s) => chosen.has(s));
      localStorage.setItem("bibliocell-overview-screening", this.overviewScreening.join(","));
      this.render();
      return;
    }
    if (target.id === "overview-hide-preprint" && target instanceof HTMLInputElement) {
      this.overviewHidePreprint = target.checked;
      localStorage.setItem("bibliocell-overview-hide-preprint", target.checked ? "on" : "off");
      this.render();
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
      if (this.env.readOnly) {
        target.value = key === "screening" ? p.screening : p.reading;
        this.toast(t("只读访问：登录管理员后才能修改。"), true);
        return;
      }
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
    if (target.id === "batch-search") { batchSearch(this, target.value); return; }
    if (target.matches("[name=speciesName], [name=speciesLabel]")) {
      const picker = target.closest(".species-row")?.querySelector<HTMLSelectElement>("[data-species-picker]");
      if (picker) picker.value = "";
    }
    if (target.id === "field-dataset-search") { searchDatasetPicker(this, target.value); return; }
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
    if (paperFieldKey(e)) return;
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
      if (this.translation) this.clearTranslation();
      else if ($("#modal-root .modal")) this.closeModal();
      else if (this.drawer) this.closeDrawer();
      else if (this.screen) this.exitScreening();
      else if (this.datasetScreen) this.exitDatasetReview();
      else if (this.page === "agent" && this.agentPdf) closeAgentPdf(this);
      else document.body.classList.remove("nav-open");
      return;
    }
    if (this.screen && this.page === "papers" && !this.drawer && !$("#modal-root .modal") && !target?.matches("input,textarea,select") && !e.altKey && !e.ctrlKey && !e.metaKey && this.screenKey(e)) {
      e.preventDefault();
      return;
    }
    if (this.datasetScreen && this.page === "datasets" && !this.drawer && !$("#modal-root .modal") && !target?.matches("input,textarea,select") && !e.altKey && !e.ctrlKey && !e.metaKey && this.datasetReviewKey(e)) {
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
    // A checkbox or radio under the focus is not a place to type, so the sheet and search keys still work there.
    const typing = Boolean(target?.matches("input:not([type=checkbox]):not([type=radio]),textarea,select"));
    if (e.key === "?" && !typing && !$("#modal-root .modal")) {
      e.preventDefault();
      shortcutsModal(this);
      return;
    }
    if ((e.key === "/" || ((e.ctrlKey || e.metaKey) && e.key === "k")) && !typing) {
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
    if (key === "d" && !this.env.readOnly) {
      void this.dismissPaper(current.id);
      return true;
    }
    if (key === "a" && this.screen!.ai) {
      this.adoptJudgement(current.id);
      return true;
    }
    if (key === "t" && this.canTranslate()) {
      this.translateInScreening(current.id);
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
        ? matchingDatasets(this)
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

