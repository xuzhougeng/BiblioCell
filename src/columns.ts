// Stable IDs keep saved column choices independent of labels and table positions.
export const PAPER_COLUMNS = [
  ["star", "收藏"], ["title", "文献"], ["journal", "期刊"], ["year", "发表时间"],
  ["species", "研究对象"], ["datasets", "关联数据集"], ["modality", "技术"],
  ["fulltext", "全文"], ["screening", "收录状态"],
] as const;
export type PaperColumn = typeof PAPER_COLUMNS[number][0];
export const COLUMN_STORAGE_KEY = "bibliocell-paper-columns-v1";
export const defaultPaperColumns = (): PaperColumn[] => PAPER_COLUMNS.map(([id]) => id);
export function parsePaperColumns(raw: string | null): PaperColumn[] {
  try {
    const value: unknown = JSON.parse(raw ?? "null");
    if (!Array.isArray(value)) return defaultPaperColumns();
    const columns = PAPER_COLUMNS.filter(([id]) => value.includes(id)).map(([id]) => id);
    return columns.length ? columns : defaultPaperColumns();
  } catch { return defaultPaperColumns(); }
}
export function loadPaperColumns(): PaperColumn[] {
  try { return parsePaperColumns(localStorage.getItem(COLUMN_STORAGE_KEY)); }
  catch { return defaultPaperColumns(); }
}
