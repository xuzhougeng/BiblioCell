// Stable IDs keep saved column choices independent of labels and table positions.
import { t } from "./i18n.ts";
export const PAPER_COLUMNS = [
  ["star", t("收藏")], ["title", t("文献")], ["journal", t("期刊")], ["year", t("发表时间")],
  ["species", t("研究对象")], ["datasets", t("关联数据集")], ["modality", t("技术")],
  ["fulltext", t("全文")], ["screening", t("收录状态")],
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
