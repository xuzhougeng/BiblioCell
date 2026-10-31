// Application state, rendering and event delegation.
import type { Config, MCPInfo, Paper, SourceStatus, Status } from "./mygo.ts";
import { demoState } from "./demo-data.ts";
import { renderDrawer } from "./drawer.ts";
import type { Evidence } from "./mygo.ts";
import { fileActions } from "./files.ts";
import {
  datasetModal, exclusionReasonModal, importSummaryModal, linkDatasetModal, paperModal, resetDemoModal, resourceModal,
  saveDataset, saveDatasetLink, saveExclusionReason, savePaper, saveResource,
} from "./forms.ts";
import {
  PAGES, READING, SCREEN, clone, csvText, decideScreening, emptyFilters, esc, filterPapers, isPage, nowISO, paperDatasets, prepareImport,
  speciesNameFor, tasksFor, uid, type Filters, type ImportStats, type LibraryState, type Page, type Resource, type TaskRow, type TaskType,
} from "./model.ts";
import { applyProviderDefault, renderSources, saveSourceConfig, sourceConfigModal, sourceRecordsModal, syncSource } from "./sources.ts";
import { reviewActions } from "./review.ts";
import { previewFontSize, renderSettings, saveSettings, settingsActions } from "./settings.ts";
import { SaveRejected, type LibraryStore } from "./store.ts";
import { $, $$, I, downloadFile, errorMessage, speciesRow } from "./ui.ts";
import { renderDatasets, renderOverview, renderPapers, renderSchema, renderTasks } from "./views.ts";

/** Env describes the runtime: desktop vs browser preview, real vs demo data, and backend-provided info. */
export interface Env {
  desktop: boolean;
  real: boolean;
  settings: Config;
  mcp: MCPInfo;
  status: Status | null;
  sources: SourceStatus[];
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
  drawer: Drawer | null = null;
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
    if (this.env.real) window.addEventListener("focus", () => {
      if (!this.drawer && !$("#modal-root .modal")) void this.reload();
    });
    this.render();
    if (this.warning) this.toast(this.warning, true);
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
      this.toast(errorMessage(error, "保存失败，请重试。"), true);
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
      this.toast(`刷新失败：${errorMessage(error, "请重新打开应用。")}`, true);
    }
  }
  logActivity(message: string, kind = "edit"): void {
    this.state.activities.unshift({ id: uid("act"), message, kind, at: nowISO() });
    this.state.activities = this.state.activities.slice(0, 100);
  }

  // ---- rendering ----
  render(): void {
    const content = $("#app-content");
    if (!content) return;
    const renderers: Record<Page, () => string> = {
      overview: () => renderOverview(this),
      papers: () => renderPapers(this),
      datasets: () => renderDatasets(this),
      tasks: () => renderTasks(this),
      sources: () => renderSources(this),
      schema: () => renderSchema(),
      settings: () => renderSettings(this),
    };
    content.innerHTML = renderers[this.page]();
    const actions = $(".heading-actions");
    if (actions) actions.hidden = this.page === "settings";
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

  // ---- modal ----
  showModal(title: string, body: string, footer: string, wide = false): void {
    this.modalLastFocus = document.activeElement;
    const root = $("#modal-root");
    if (!root) return;
    root.innerHTML = `<div class="modal-backdrop" data-action="close-modal"></div><section class="modal${wide ? " modal-wide" : ""}" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-header"><h2 id="modal-title">${esc(title)}</h2><button class="btn-icon" data-action="close-modal" aria-label="关闭弹窗">${I("x", 20)}</button></div><div class="modal-body">${body}</div><div class="modal-footer">${footer}</div></section>`;
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
    this.toast(`已导出当前筛选的 ${list.length} 篇文献。`);
  }
  exportJSON(): void {
    downloadFile(`plantcell-library-${nowISO().slice(0, 10)}.json`, JSON.stringify({ ...this.state, exportedAt: nowISO() }, null, 2), "application/json;charset=utf-8");
    this.toast("已导出文献、数据集、链接与笔记。");
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
        if (file.size > 10 * 1024 * 1024) throw new Error("请导入 10 MB 以内的 JSON 文件。");
        const data: unknown = JSON.parse(await file.text());
        this.pendingImport = prepareImport(data, this.state);
        importSummaryModal(this, this.pendingImport.stats);
      } catch (error) {
        this.toast(error instanceof SyntaxError ? "JSON 格式无效，请使用本工具导出的备份文件。" : errorMessage(error, "导入失败，原有数据未改变。"), true);
      }
      input.remove();
    });
    input.click();
  }
  private confirmImport(): void {
    if (!this.pendingImport) return;
    const { next, stats } = this.pendingImport;
    this.state = next;
    this.logActivity(`导入 ${stats.papers} 篇文献、${stats.datasets} 组数据集和 ${stats.resources} 个链接`, "import");
    this.persist();
    this.closeModal();
    this.closeDrawer();
    this.render();
    this.toast("导入完成，已有记录保持原值。");
  }
  private confirmReset(): void {
    this.state = demoState();
    this.logActivity("恢复演示数据", "reset");
    this.persist();
    this.closeModal();
    this.closeDrawer();
    this.filters = emptyFilters();
    this.navigate("overview");
    this.toast("已恢复示例内容。");
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
    "add-species-row": () => {
      const rows = $("#species-rows");
      if (!rows) return;
      rows.insertAdjacentHTML("beforeend", speciesRow());
      $$<HTMLElement>(".species-row", rows).at(-1)?.querySelector<HTMLInputElement>("[name=speciesName]")?.focus();
    },
    "remove-species-row": (el) => el.closest(".species-row")?.remove(),
    "open-paper": (_el, id) => this.openPaper(id),
    "open-dataset": (_el, id) => this.openDataset(id),
    "close-drawer": () => this.closeDrawer(),
    "close-modal": () => this.closeModal(),
    "drawer-tab": (el) => {
      this.drawerTab = el.dataset.tab ?? "overview";
      this.renderDrawer(true);
    },
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
      this.logActivity(`保存笔记：${p.title}`);
      this.persist();
      this.toast("笔记已保存。");
    },
    "save-tags": (_el, id) => {
      const raw = $<HTMLInputElement>("#paper-tags")?.value ?? "";
      const tags = [...new Set(raw.split(/[,，;；\n]/).map((s) => s.trim()).filter(Boolean))].slice(0, 30);
      if (!this.touchPaper(id, (p) => { p.tags = tags; })) return;
      this.persist();
      this.toast("标签已保存。");
      this.render();
    },
    "clear-filters": () => {
      this.filters = emptyFilters();
      const search = $<HTMLInputElement>("#global-search");
      if (search) search.value = "";
      this.render();
    },
    "all-papers": () => {
      this.filters.starred = false;
      this.render();
    },
    "starred-papers": () => {
      this.filters.starred = true;
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
      this.logActivity(`采纳 Agent 建议：${p.title}`);
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast("已采纳，判断记为人工确认。");
    },
    "dismiss-proposal": (_el, id) => {
      if (!this.touchPaper(id, (p) => {
        p.proposedScreening = "";
        p.proposedReason = "";
      })) return;
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast("已忽略这条建议。");
    },
    ...settingsActions(this),
    ...fileActions(this),
    ...reviewActions(this),
  };

  private onClick(e: MouseEvent): void {
    const target = e.target;
    if (!(target instanceof Element)) return;
    const el = target.closest<HTMLElement>("[data-action]");
    if (el) {
      const action = el.dataset.action ?? "";
      if (this.env.real && DEMO_ONLY_ACTIONS.has(action)) {
        this.toast("真实模式暂不支持资料链接和 JSON 导入；文献文件可在详情的“数据与资料”中导入。", true);
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
      "paper-form": savePaper, "source-config-form": saveSourceConfig, "settings-form": saveSettings,
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
    if (target instanceof HTMLSelectElement && target.matches('#source-config-form [name="provider"]')) {
      applyProviderDefault(target);
      return;
    }
    if (target.id === "demo-mode" && target instanceof HTMLInputElement) {
      localStorage.setItem("bibliocell-demo-mode", String(target.checked));
      location.reload();
      return;
    }
    if (target instanceof HTMLSelectElement && target.dataset.filter) {
      const key = target.dataset.filter as keyof Filters;
      if (key !== "starred" && key in this.filters) {
        this.filters[key] = target.value;
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
      this.logActivity(`更新${key === "reading" ? "阅读进度" : "收录判断"}：${p.title}`);
      this.persist();
      this.render();
      this.renderDrawer();
      this.toast("整理状态已保存。");
    }
  }

  private onInput(e: Event): void {
    const target = e.target;
    if (!(target instanceof HTMLInputElement)) return;
    if (target.id === "settings-font-size") {
      previewFontSize(target);
      return;
    }
    if (target.id === "paper-search" || target.id === "dataset-search") {
      const { id, value, selectionStart } = target;
      clearTimeout(this.searchTimer);
      this.searchTimer = setTimeout(() => {
        if (id === "paper-search") this.filters.search = value;
        else this.datasetSearch = value;
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
    if (e.key === "Escape") {
      if ($("#modal-root .modal")) this.closeModal();
      else if (this.drawer) this.closeDrawer();
      else document.body.classList.remove("nav-open");
      return;
    }
    if (e.key === "Enter" && target instanceof HTMLInputElement && target.id === "global-search") {
      e.preventDefault();
      this.filters = { ...emptyFilters(), search: target.value, sort: this.filters.sort };
      this.navigate("papers");
      return;
    }
    if (e.key === "Enter" && target?.matches("tr[data-action]")) {
      e.preventDefault();
      this.openPaper(target.dataset.id ?? "");
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
}

