// Persistence for papers, datasets, resources and activities. The real store
// talks to the Go backend through the generated typed client; the demo store
// keeps everything in localStorage. Both expose the same two methods.
import { Library } from "./mygo.ts";
import { demoState } from "./demo-data.ts";
import { changedItems, fileResource, isLibraryState, normalizeDataset, type LibraryState } from "./model.ts";
import { t } from "./i18n.ts";

export interface LibraryStore {
  load(): Promise<LibraryState>;
  /** save writes next; saved is the state it was last in sync with. */
  save(next: LibraryState, saved: LibraryState): Promise<void>;
}

/** SaveRejected means the database kept its own version, so the window must read it again. */
export class SaveRejected extends Error {}

let queue: Promise<unknown> = Promise.resolve();
/** enqueue serializes backend writes so papers land before the datasets that reference them. */
export function enqueue<T>(operation: () => Promise<T>): Promise<T> {
  const next = queue.catch(() => {}).then(operation);
  queue = next;
  return next;
}

/** sqliteStore persists to the Go-backed database. Stored files change through their own calls, never through save. */
export const sqliteStore: LibraryStore = {
  load() {
    return enqueue<LibraryState>(async () => {
      const [papers, datasets, files, evidence] = await Promise.all([Library.listPapers(), Library.listDatasets(), Library.listPaperFiles(), Library.listEvidence()]);
      return { schemaVersion: 2, papers, datasets: datasets.map(normalizeDataset), resources: files.map(fileResource), evidence, activities: [] };
    });
  },
  save(next, saved) {
    const papers = changedItems(next.papers, saved.papers).map(([after, before]) => ({ before: before ?? null, after }));
    const datasets = changedItems(next.datasets, saved.datasets).map(([after, before]) => ({ before: before ?? null, after }));
    return enqueue(async () => {
      const conflicts = papers.length ? await Library.savePaperEdits(papers) : [];
      return datasets.length ? [...conflicts, ...await Library.saveDatasetEdits(datasets)] : conflicts;
    }).then(
      (conflicts) => {
        if (conflicts.length) throw new SaveRejected(t("《{titles}》的同一内容刚被其他来源修改，这次改动未保存，已重新加载。", { titles: conflicts.join(t("》《")) }));
      },
      () => {
        throw new SaveRejected(t("写入 SQLite 失败，已重新加载数据库中的内容。"));
      },
    );
  },
};

// ponytail: key kept from the web prototype so earlier demo edits survive the rename.
export const DEMO_STORAGE_KEY = "plantcell-library-prototype-v2";

/** localStore keeps the demo library in the browser only; load() rejects when storage is unusable. */
export function localStore(storage: Storage, key = DEMO_STORAGE_KEY): LibraryStore {
  return {
    async load() {
      const saved = storage.getItem(key);
      const candidate: unknown = saved ? JSON.parse(saved) : null;
      if (!isLibraryState(candidate)) return demoState();
      candidate.datasets = candidate.datasets.map(normalizeDataset);
      if (!Array.isArray(candidate.activities)) candidate.activities = [];
      return candidate;
    },
    async save(state) {
      try {
        storage.setItem(key, JSON.stringify(state));
      } catch {
        throw new Error(t("浏览器存储不可用，请导出 JSON 备份本次修改。"));
      }
    },
  };
}
