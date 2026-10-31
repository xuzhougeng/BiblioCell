// Persistence for papers, datasets, resources and activities. The real store
// talks to the Go backend through the generated typed client; the demo store
// keeps everything in localStorage. Both expose the same two methods.
import { Library } from "./mygo.ts";
import { demoState } from "./demo-data.ts";
import { fileResource, isLibraryState, normalizeDataset, type LibraryState } from "./model.ts";

export interface LibraryStore {
  load(): Promise<LibraryState>;
  save(state: LibraryState): Promise<void>;
}

let queue: Promise<unknown> = Promise.resolve();
/** enqueue serializes backend writes so papers land before the datasets that reference them. */
export function enqueue<T>(operation: () => Promise<T>): Promise<T> {
  const next = queue.catch(() => {}).then(operation);
  queue = next;
  return next;
}

/** sqliteStore persists to the Go-backed database. Stored files change through their own calls, never through save. */
export const sqliteStore: LibraryStore = {
  async load() {
    const [papers, datasets, files] = await Promise.all([Library.listPapers(), Library.listDatasets(), Library.listPaperFiles()]);
    return { schemaVersion: 2, papers, datasets: datasets.map(normalizeDataset), resources: files.map(fileResource), activities: [] };
  },
  save(state) {
    return enqueue(async () => {
      await Library.savePapers(state.papers);
      await Library.saveDatasets(state.datasets);
    }).catch(() => {
      throw new Error("写入 SQLite 失败，请检查数据库后重试。");
    });
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
        throw new Error("浏览器存储不可用，请导出 JSON 备份本次修改。");
      }
    },
  };
}
