import type { App } from "./app.ts";
import { Library, type Dataset, type Paper, type TrashEntry } from "./mygo.ts";
import { clone, esc, type LibraryState } from "./model.ts";
import { enqueue } from "./store.ts";
import { t } from "./i18n.ts";

export interface DemoTrashEntry extends TrashEntry {
  paper?: Paper;
  dataset?: Dataset;
  resources: LibraryState["resources"];
  evidence: LibraryState["evidence"];
  manifests: LibraryState["datasetManifests"];
}

/** Archive demo data before it leaves the active arrays, just as SQLite retains its rows. */
export function archiveDemo(state: LibraryState, kind: string, ids: Set<string>): void {
  state.trash ??= [];
  const records = kind === "paper" ? state.papers : state.datasets;
  for (const record of records.filter(r => ids.has(r.id))) {
    const belongs = (r: { paperId?: string | null; datasetId?: string | null }) => kind === "paper" ? r.paperId === record.id : r.datasetId === record.id;
    state.trash.push({ kind, id: record.id, title: record.title,
      identifier: kind === "paper" ? (record as Paper).doi || (record as Paper).pmid : (record as Dataset).accession,
      deletedAt: new Date().toISOString(),
      ...(kind === "paper" ? { paper: clone(record as Paper) } : { dataset: clone(record as Dataset) }),
      resources: state.resources.filter(belongs), evidence: (state.evidence ?? []).filter(belongs),
      manifests: kind === "dataset" ? (state.datasetManifests ?? []).filter(m => m.datasetId === record.id) : [],
    });
  }
}

export async function openTrash(app: App): Promise<void> {
  if (app.env.readOnly) return;
  try {
    const entries = app.env.real ? await enqueue(() => Library.listTrash()) : app.state.trash ?? [];
    app.showModal(t("回收站"), `<p class="muted">${t("已删除的记录不会出现在日常列表中，来源更新和重新导入也不会自动加回。附件与关联保留，可在这里恢复。")}</p>`
      + (entries.length ? `<div class="trash-list">${entries.map(e => `<div class="trash-entry"><div><span class="muted">${e.kind === "paper" ? t("文献") : t("数据集")} · ${esc(e.deletedAt.slice(0, 10))}</span><strong>${esc(e.title)}</strong><small>${esc(e.identifier || e.id)}</small></div><button class="btn btn-secondary" data-action="restore-trash" data-kind="${esc(e.kind)}" data-id="${esc(e.id)}">${t("恢复")}</button></div>`).join("")}</div>` : `<p>${t("回收站为空。")}</p>`),
      `<button class="btn btn-secondary" data-action="close-modal">${t("关闭")}</button>`);
  } catch { app.toast(t("读取回收站失败，请重试。"), true); }
}

export async function restoreTrash(app: App, button: HTMLElement): Promise<void> {
  if (app.env.readOnly || button.hasAttribute("disabled")) return;
  button.setAttribute("disabled", "");
  const { kind = "", id = "" } = button.dataset;
  try {
    if (app.env.real) {
      await enqueue(() => Library.restoreTrash(kind, id));
      await app.reload();
    } else {
      const next = clone(app.state);
      const entry = next.trash?.find(e => e.kind === kind && e.id === id);
      if (!entry) return;
      if (entry.paper) next.papers.push(entry.paper);
      if (entry.dataset) next.datasets.push(entry.dataset);
      next.resources.push(...entry.resources.filter(r => !next.resources.some(x => x.id === r.id)));
      next.evidence = [...next.evidence ?? [], ...entry.evidence ?? []];
      next.datasetManifests = [...next.datasetManifests ?? [], ...entry.manifests ?? []];
      next.trash = next.trash!.filter(e => e !== entry);
      await app.replaceDemoState(next);
    }
    await openTrash(app);
    app.toast(t("已从回收站恢复。"));
  } catch { app.toast(t("恢复失败，请重试。"), true); }
  finally { button.removeAttribute("disabled"); }
}
