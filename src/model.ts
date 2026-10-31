// Pure data logic shared by the UI and the node:test suite. No DOM access here.
import type { Dataset, DatasetPaperLink, DatasetRepository, Paper, PaperFile, SpeciesEntry } from "./mygo.ts";

export interface Resource {
  id: string;
  paperId: string | null;
  datasetId: string | null;
  type: string;
  title: string;
  url: string;
  status: string;
  isDemo: boolean;
  addedAt: string;
  localPath?: string;
  sizeBytes?: number;
}

/** fileResource shows a file stored in SQLite as a resource row. */
export const fileResource = (file: PaperFile): Resource => ({ ...file, datasetId: null, url: "", status: "stored", isDemo: false });

export const fileSize = (bytes = 0): string =>
  bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;

export interface Activity {
  id: string;
  message: string;
  kind: string;
  at: string;
}

export interface LibraryState {
  schemaVersion: 2;
  papers: Paper[];
  datasets: Dataset[];
  resources: Resource[];
  activities: Activity[];
}

export interface Filters {
  search: string;
  species: string;
  organ: string;
  year: string;
  modality: string;
  platform: string;
  fulltext: string;
  screening: string;
  starred: boolean;
  sort: string;
}

export const emptyFilters = (): Filters => ({
  search: "", species: "", organ: "", year: "", modality: "", platform: "", fulltext: "", screening: "", starred: false, sort: "newest",
});

export const SCREEN: Record<string, string> = { included: "已纳入", pending: "待判断", uncertain: "证据不足", excluded: "相关 / 排除" };
export const READING: Record<string, string> = { unread: "未读", skimmed: "已浏览", read: "已精读" };
export const FULL: Record<string, string> = { missing: "待补入口", linked: "已有入口", stored: "已保存文件" };
export const ROLE: Record<string, string> = { generated: "新生成", reused: "复用", reprocessed: "重新处理", benchmarked: "基准比较", unknown: "关系待确认" };
export const AVAILABILITY: Record<string, string> = { unknown: "尚未核实", public: "公开可访问", unavailable: "暂不可访问", embargoed: "尚未解禁", restricted: "需要申请" };
export const RESOURCE_TYPES: Record<string, string> = { fulltext: "全文入口", supplement: "补充材料", data: "数据入口", matrix: "表达矩阵", annotation: "细胞注释", code: "分析代码", other: "其他资料" };

export const PAGES = {
  overview: ["研究总览", "把文献收齐，让数据逐步完整。"],
  papers: ["文献库", "先收录，再阅读、关联与补全。"],
  datasets: ["数据集", "沿着文献找到数据，保留来源与复用关系。"],
  tasks: ["待补事项", "从整理状态生成，按需要逐步完成。"],
  sources: ["更新与来源", "记录发现入口和整理活动。"],
  schema: ["数据库结构", "三个核心对象，连接文献、数据与资料。"],
  settings: ["设置", "让研究库按照你的方式保存、阅读与工作。"],
} as const;
export type Page = keyof typeof PAGES;
export const isPage = (value: string): value is Page => Object.hasOwn(PAGES, value);

export const ARTICLE_TYPES = ["原创研究", "方法研究", "综述", "相关资料"];
export const MODALITY_CHOICES = ["scRNA-seq", "snRNA-seq", "scATAC-seq", "snATAC-seq", "Multiome", "sc-long-read", "空间转录组"];
export const PLATFORM_CHOICES = ["10x Genomics Chromium", "BD Rhapsody", "Drop-seq", "Microwell-seq", "inDrop", "Smart-seq2"];

const ESCAPES: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
/** esc makes any value safe to interpolate into innerHTML, attributes included. */
export const esc = (value: unknown): string => String(value ?? "").replace(/[&<>"']/g, (c) => ESCAPES[c] ?? c);

export const safeURL = (value: unknown): string => {
  try {
    const u = new URL(String(value).trim());
    return (u.protocol === "https:" || u.protocol === "http:") && !u.username && !u.password ? u.href : "";
  } catch {
    return "";
  }
};

export const dateText = (value: string): string => {
  const d = new Date(value);
  return isNaN(d.getTime()) ? "日期待补" : d.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).replaceAll("/", ".");
};

export const normalizeDOI = (value: unknown): string =>
  String(value ?? "").trim().replace(/^https?:\/\/(?:dx\.)?doi\.org\//i, "").replace(/^doi:\s*/i, "").trim().toLowerCase();
export const validDOI = (value: string): boolean => !value || /^10\.\d{4,9}\/[^\s<>"']+$/i.test(value);
export const uid = (prefix: string): string => `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
export const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
export const arr = <T>(value: T[] | null | undefined): T[] => (Array.isArray(value) ? value : []);
export const splitList = (value: unknown): string[] =>
  [...new Set(String(value ?? "").split(/[,，;；\n]/).map((s) => s.trim()).filter(Boolean))];
export const nowISO = (): string => new Date().toISOString();

type HasSpecies = { species: SpeciesEntry[] };
export const speciesNames = (p: HasSpecies): string[] => arr(p.species).map((s) => s?.name ?? "").filter(Boolean);
export const speciesDisplay = (p: HasSpecies): string => arr(p.species).map((s) => s?.label || s?.name || "").filter(Boolean).join("、");
export const joinList = (value: string[] | null | undefined, separator = "、"): string => arr(value).filter(Boolean).join(separator);

export type ListField = "species" | "organ" | "year" | "modality" | "platform";
export const listValues: Record<ListField, (p: Paper) => string[]> = {
  species: (p) => speciesNames(p),
  organ: (p) => arr(p.organs),
  modality: (p) => arr(p.modalities),
  platform: (p) => arr(p.platforms),
  year: (p) => (p.year ? [String(p.year)] : []),
};

export function uniqueValues(papers: Paper[], field: ListField): string[] {
  return [...new Set(papers.flatMap((p) => listValues[field](p)).filter(Boolean))].sort((a, b) => a.localeCompare(b, "zh-CN"));
}

export function speciesLabelOf(papers: Paper[], name: string): string {
  for (const p of papers) {
    for (const s of arr(p.species)) if (s?.name === name) return s.label || name;
  }
  return name;
}

/** speciesNameFor finds the scientific name behind a display label (or name). */
export function speciesNameFor(papers: Paper[], label: string): string {
  for (const p of papers) {
    for (const s of arr(p.species)) if (s && (s.label === label || s.name === label)) return s.name;
  }
  return "";
}

export function filterPapers(papers: Paper[], filters: Filters): Paper[] {
  const q = filters.search.trim().toLocaleLowerCase();
  return papers
    .filter((p) => {
      const hay = [
        p.title, p.authors, p.doi, p.pmid,
        arr(p.species).map((s) => (s ? `${s.name || ""} ${s.label || ""}`.trim() : "")).join(" "),
        joinList(p.organs, " "), joinList(p.modalities, " "), joinList(p.platforms, " "), joinList(p.articleTypes, " "),
        p.abstract, arr(p.tags).join(" "),
      ].join(" ").toLocaleLowerCase();
      return (!q || hay.includes(q))
        && (!filters.species || listValues.species(p).includes(filters.species))
        && (!filters.organ || listValues.organ(p).includes(filters.organ))
        && (!filters.year || listValues.year(p).includes(filters.year))
        && (!filters.modality || listValues.modality(p).includes(filters.modality))
        && (!filters.platform || listValues.platform(p).includes(filters.platform))
        && (!filters.fulltext || p.fulltext === filters.fulltext)
        && (!filters.screening || p.screening === filters.screening)
        && (!filters.starred || p.starred);
    })
    .sort((a, b) => filters.sort === "year-desc"
      ? (Number(b.year) || 0) - (Number(a.year) || 0)
      : filters.sort === "title"
        ? a.title.localeCompare(b.title, "zh-CN")
        : String(b.addedAt || "").localeCompare(String(a.addedAt || "")));
}

export const paperDatasets = (datasets: Dataset[], paperId: string): Dataset[] =>
  datasets.filter((d) => arr(d.paperLinks).some((link) => link.paperId === paperId));

export type TaskType = "screen" | "fulltext" | "data";
export interface TaskRow {
  key: string;
  type: TaskType;
  paper: Paper;
  title: string;
  subtitle: string;
  color: string;
  icon: string;
}

export function tasksFor(papers: Paper[], datasets: Dataset[]): TaskRow[] {
  const rows: TaskRow[] = [];
  for (const p of papers) {
    if (p.screening === "pending" || p.screening === "uncertain") {
      rows.push({ key: `${p.id}-screen`, type: "screen", paper: p, title: p.screening === "uncertain" ? "补充判断证据" : "确认收录范围", subtitle: "核实研究对象与单细胞测序证据", color: "amber", icon: "check" });
    }
    if (p.screening === "included" && p.fulltext !== "stored") {
      rows.push({ key: `${p.id}-fulltext`, type: "fulltext", paper: p, title: p.fulltext === "linked" ? "获取全文正文" : "补充全文入口", subtitle: p.fulltext === "linked" ? "已有入口，正文文件仍待获取" : "先记录可用入口，正文可稍后获取", color: "blue", icon: "file" });
    }
    const linked = paperDatasets(datasets, p.id);
    if (p.screening === "included" && (!linked.length || linked.some((d) => !d.accession || d.status !== "verified"))) {
      rows.push({ key: `${p.id}-data`, type: "data", paper: p, title: "核对数据来源", subtitle: "补充 accession，区分新生成与复用", color: "purple", icon: "database" });
    }
  }
  return rows;
}

type Rec = Record<string, unknown>;
const isRec = (value: unknown): value is Rec => typeof value === "object" && value !== null;
const str = (value: unknown): string => (value == null ? "" : String(value));

export const emptyRepository = (): DatasetRepository => ({
  id: "", repository: "", accession: "", recordKind: "", url: "", publicStatus: "", mappingScope: "", evidenceUrl: "", evidenceText: "", verified: false,
});

/**
 * normalizeDataset turns any stored shape (Go record, prototype localStorage
 * entry with single-value organ/modality/technology, or import row) into a
 * full Dataset whose repository/accession mirror the first repository entry.
 */
export function normalizeDataset(input: object): Dataset {
  const raw = input as Rec;
  const list = (value: unknown, legacy?: unknown): string[] => {
    const values: unknown[] = Array.isArray(value) ? value : legacy == null ? [] : [legacy];
    return [...new Set(values.filter((v): v is string => typeof v === "string").map((v) => v.trim()).filter(Boolean))];
  };
  let species: SpeciesEntry[];
  if (Array.isArray(raw.species)) {
    species = (raw.species as unknown[]).filter(isRec)
      .filter((s) => typeof s.name === "string" && s.name.trim())
      .map((s) => ({ name: str(s.name).trim(), label: str(s.label).trim() }));
  } else {
    const legacy = str(raw.species).trim();
    species = legacy ? [{ name: legacy, label: str(raw.speciesLabel).trim() }] : [];
  }
  let repositories: DatasetRepository[] = (Array.isArray(raw.repositories) ? (raw.repositories as unknown[]) : []).filter(isRec).map((r) => ({
    id: str(r.id), repository: str(r.repository).trim().toLowerCase(), accession: str(r.accession).trim(),
    recordKind: str(r.recordKind), url: str(r.url), publicStatus: str(r.publicStatus), mappingScope: str(r.mappingScope),
    evidenceUrl: str(r.evidenceUrl), evidenceText: str(r.evidenceText), verified: r.verified === true,
  }));
  if (!repositories.length && (raw.repository || raw.accession)) {
    repositories = [{ ...emptyRepository(), repository: str(raw.repository).trim().toLowerCase(), accession: str(raw.accession).trim() }];
  }
  const paperLinks: DatasetPaperLink[] = (Array.isArray(raw.paperLinks) ? (raw.paperLinks as unknown[]) : []).filter(isRec).map((l) => ({
    paperId: str(l.paperId), role: str(l.role), evidenceUrl: str(l.evidenceUrl), evidenceText: str(l.evidenceText), verified: l.verified === true,
  }));
  const cells = typeof raw.cellsReported === "number" ? raw.cellsReported : null;
  return {
    id: str(raw.id), title: str(raw.title), description: str(raw.description), notes: str(raw.notes),
    species, organs: list(raw.organs, raw.organ), modalities: list(raw.modalities, raw.modality), platforms: list(raw.platforms, raw.technology),
    repositories, repository: repositories[0]?.repository ?? "", accession: repositories[0]?.accession ?? "",
    status: str(raw.status) || "pending", curationLevel: str(raw.curationLevel),
    availability: str(raw.availability) || "unknown", matrixAvailability: str(raw.matrixAvailability) || "unknown",
    annotationAvailability: str(raw.annotationAvailability) || "unknown",
    cellsReported: cells, cellCountUnit: str(raw.cellCountUnit), cellCountScope: str(raw.cellCountScope),
    paperLinks, addedAt: str(raw.addedAt), updatedAt: str(raw.updatedAt), isDemo: raw.isDemo === true,
  };
}

/** isLibraryState accepts a prototype v2 localStorage blob or JSON export. */
export function isLibraryState(value: unknown): value is LibraryState {
  return isRec(value) && value.schemaVersion === 2 && Array.isArray(value.papers) && Array.isArray(value.datasets) && Array.isArray(value.resources);
}

export function csvText(papers: Paper[], datasets: Dataset[]): string {
  const cell = (value: unknown): string => {
    let s = str(value);
    if (/^[=+\-@\t\r]/.test(s)) s = `'${s}`;
    return `"${s.replaceAll('"', '""')}"`;
  };
  const headers = ["内部编号", "题目", "DOI", "作者", "年份", "物种（中文名）", "物种（学名）", "组织", "技术", "测序平台", "文献类型", "收录状态", "全文情况", "阅读进度", "关联数据集", "标签", "笔记", "是否示例"];
  const rows = papers.map((p) => [
    p.id, p.title, p.doi, p.authors, p.year,
    joinList(arr(p.species).map((s) => s?.label || s?.name || "")), joinList(speciesNames(p)),
    joinList(p.organs, ";"), joinList(p.modalities, ";"), joinList(p.platforms, ";"), joinList(p.articleTypes, ";"),
    SCREEN[p.screening], FULL[p.fulltext], READING[p.reading],
    paperDatasets(datasets, p.id).map((d) => d.id).join(";"), arr(p.tags).join(";"), p.notes, p.isDemo ? "是" : "否",
  ].map(cell).join(","));
  return `﻿${headers.map(cell).join(",")}\r\n${rows.join("\r\n")}`;
}

export interface ImportStats {
  papers: number;
  datasets: number;
  resources: number;
  duplicates: number;
  renamed: number;
}

const SCREENINGS = ["pending", "included", "excluded", "uncertain"];
const READINGS = ["unread", "skimmed", "read"];
const FULLTEXTS = ["missing", "linked", "stored"];
const PUBLICATION_STATUSES = ["unknown", "published", "preprint", "unpublished"];
const AVAILABILITIES = ["unknown", "public", "unavailable", "embargoed", "restricted"];
const ROLES = ["generated", "reused", "reprocessed", "benchmarked", "unknown"];

/**
 * prepareImport validates a JSON backup (schemaVersion 1 or 2) and merges it
 * into a copy of the current state. Existing records keep their values; the
 * returned stats describe what the merge would add. Throws on any invalid row.
 */
export function prepareImport(data: unknown, current: LibraryState): { next: LibraryState; stats: ImportStats } {
  if (!isRec(data) || (data.schemaVersion !== 1 && data.schemaVersion !== 2) || !Array.isArray(data.papers) || !Array.isArray(data.datasets) || !Array.isArray(data.resources)) {
    throw new Error("文件结构不匹配，请使用本工具导出的 JSON 备份。");
  }
  const paperRows = data.papers as unknown[];
  const datasetRows = data.datasets as unknown[];
  const resourceRows = data.resources as unknown[];
  if (paperRows.length > 20000 || datasetRows.length > 20000 || resourceRows.length > 50000) {
    throw new Error("导入记录数量超出原型限制，请分批整理。");
  }
  const textValue = (v: unknown, max = 50000): string => {
    if (v == null) return "";
    if (typeof v !== "string" && typeof v !== "number") throw new Error("有字段的文本类型不正确。");
    const s = String(v);
    if (s.length > max) throw new Error("有字段内容过长。");
    return s;
  };
  const enumValue = (v: unknown, allowed: string[], fallback: string): string => {
    if (v == null || v === "") return fallback;
    if (typeof v !== "string" || !allowed.includes(v)) throw new Error(`存在不支持的状态值：${String(v).slice(0, 60)}`);
    return v;
  };
  // schemaVersion 1 backups carry single-value species/organ/modality/articleType; wrap them into arrays.
  const speciesList = (v: unknown): SpeciesEntry[] => {
    if (Array.isArray(v)) {
      return (v as unknown[]).filter(isRec).filter((x) => typeof x.name === "string" && x.name.trim()).slice(0, 50)
        .map((x) => ({ name: str(x.name).trim().slice(0, 500), label: typeof x.label === "string" ? x.label.trim().slice(0, 500) : "" }));
    }
    const s = textValue(v, 500).trim();
    return s ? [{ name: s.slice(0, 500), label: "" }] : [];
  };
  const stringList = (v: unknown, legacy?: unknown): string[] => {
    const fromArray = Array.isArray(v) ? (v as unknown[]).filter((x): x is string => typeof x === "string").map((x) => x.trim().slice(0, 200)).filter(Boolean) : [];
    if (fromArray.length) return [...new Set(fromArray)].slice(0, 50);
    const s = textValue(v, 200).trim() || textValue(legacy, 200).trim();
    return s ? [s.slice(0, 200)] : [];
  };
  const isoDate = (v: unknown): string => (typeof v === "string" && !isNaN(Date.parse(v)) ? new Date(v).toISOString() : nowISO());

  const next = clone(current);
  const idMap = new Map<string, string>();
  const dsMap = new Map<string, string>();
  const importedIds = new Set<string>();
  const importedDatasetIds = new Set<string>();
  const stats: ImportStats = { papers: 0, datasets: 0, resources: 0, duplicates: 0, renamed: 0 };

  for (const row of paperRows) {
    if (!isRec(row)) throw new Error("文献条目格式不正确。");
    const oldId = textValue(row.id, 100) || uid("IMP-P");
    const title = textValue(row.title, 2000).trim();
    const doi = normalizeDOI(textValue(row.doi, 500));
    if (!title || !validDOI(doi)) throw new Error("文献题目缺失或 DOI 格式不正确。");
    if (importedIds.has(oldId)) throw new Error("导入文件中包含重复的文献 ID。");
    importedIds.add(oldId);
    const year = row.year == null || row.year === "" ? null : Number(row.year);
    if (year !== null && (!Number.isInteger(year) || year < 1800 || year > 2100)) throw new Error("文献年份格式不正确。");
    const doiMatch = doi ? next.papers.find((p) => normalizeDOI(p.doi) === doi) : undefined;
    const idMatch = next.papers.find((p) => p.id === oldId);
    const kept = doiMatch ?? (idMatch && idMatch.title === title && normalizeDOI(idMatch.doi) === doi ? idMatch : undefined);
    if (kept) {
      idMap.set(oldId, kept.id);
      stats.duplicates++;
      continue;
    }
    const id = idMatch ? uid("IMP-P") : oldId;
    if (idMatch) stats.renamed++;
    if (row.tags != null && (!Array.isArray(row.tags) || (row.tags as unknown[]).some((t) => typeof t !== "string"))) throw new Error("主题标签应为文本数组。");
    const tags = (Array.isArray(row.tags) ? (row.tags as string[]) : []).slice(0, 50).map((t) => t.slice(0, 100));
    const paper: Paper = {
      id, title, doi, pmid: textValue(row.pmid, 100), authors: textValue(row.authors, 2000), year, journal: textValue(row.journal, 1000),
      species: speciesList(row.species), organs: stringList(row.organs, row.organ), modalities: stringList(row.modalities, row.modality),
      platforms: stringList(row.platforms), articleTypes: stringList(row.articleTypes, row.articleType),
      abstract: textValue(row.abstract), notes: textValue(row.notes), tags,
      screening: enumValue(row.screening, SCREENINGS, "pending"), reading: enumValue(row.reading, READINGS, "unread"),
      fulltext: enumValue(row.fulltext, FULLTEXTS, "missing"), publicationStatus: enumValue(row.publicationStatus, PUBLICATION_STATUSES, "unknown"),
      starred: row.starred === true, isDemo: row.isDemo === true, source: textValue(row.source, 300) || "JSON 导入",
      screeningReason: textValue(row.screeningReason, 3000), addedAt: isoDate(row.addedAt), updatedAt: nowISO(),
    };
    idMap.set(oldId, id);
    next.papers.push(paper);
    stats.papers++;
  }

  for (const row of datasetRows) {
    if (!isRec(row)) throw new Error("数据集条目格式不正确。");
    const oldId = textValue(row.id, 100) || uid("IMP-DS");
    const title = textValue(row.title, 2000).trim();
    if (!title) throw new Error("数据集名称缺失。");
    if (importedDatasetIds.has(oldId)) throw new Error("导入文件中包含重复的数据集 ID。");
    importedDatasetIds.add(oldId);
    if (row.paperLinks != null && !Array.isArray(row.paperLinks)) throw new Error("文献关联必须是数组。");
    const paperLinks: DatasetPaperLink[] = (Array.isArray(row.paperLinks) ? (row.paperLinks as unknown[]) : []).map((l) => {
      if (!isRec(l)) throw new Error("文献关联格式不正确。");
      const pid = idMap.get(String(l.paperId)) ?? (next.papers.some((p) => p.id === l.paperId) ? String(l.paperId) : null);
      if (!pid) throw new Error("数据集引用了不存在的文献。");
      return { paperId: pid, role: enumValue(l.role, ROLES, "reused"), evidenceUrl: textValue(l.evidenceUrl), evidenceText: textValue(l.evidenceText), verified: l.verified === true };
    });
    const existing = next.datasets.find((d) => d.id === oldId);
    let d: Dataset;
    if (existing && existing.title === title) {
      d = existing;
      stats.duplicates++;
      for (const l of paperLinks) if (!d.paperLinks.some((x) => x.paperId === l.paperId)) d.paperLinks.push(l);
    } else {
      const id = existing ? uid("IMP-DS") : oldId;
      if (existing) stats.renamed++;
      const cells = row.cellsReported == null ? null : Number(row.cellsReported);
      if (cells !== null && (!Number.isInteger(cells) || cells < 0)) throw new Error("细胞数量应为非负整数或空值。");
      d = normalizeDataset({
        id, title, description: textValue(row.description), species: speciesList(row.species),
        organs: stringList(row.organs, row.organ), modalities: stringList(row.modalities, row.modality), platforms: stringList(row.platforms, row.technology),
        repositories: Array.isArray(row.repositories) ? row.repositories : [],
        repository: textValue(row.repository, 200).toLowerCase(), accession: textValue(row.accession, 500),
        status: enumValue(row.status, ["pending", "identified", "verified"], "pending"), curationLevel: textValue(row.curationLevel, 100),
        availability: enumValue(row.availability, AVAILABILITIES, "unknown"), matrixAvailability: enumValue(row.matrixAvailability, AVAILABILITIES, "unknown"),
        annotationAvailability: enumValue(row.annotationAvailability, AVAILABILITIES, "unknown"),
        cellsReported: cells, cellCountUnit: textValue(row.cellCountUnit, 100), cellCountScope: textValue(row.cellCountScope, 1000),
        notes: textValue(row.notes), isDemo: row.isDemo === true, paperLinks, addedAt: isoDate(row.addedAt),
      });
      const firstSpecies = d.species[0];
      if (typeof row.species === "string" && firstSpecies) firstSpecies.label = textValue(row.speciesLabel, 500);
      next.datasets.push(d);
      stats.datasets++;
    }
    dsMap.set(oldId, d.id);
  }

  for (const row of resourceRows) {
    if (!isRec(row)) throw new Error("资料条目格式不正确。");
    const paperId = row.paperId ? idMap.get(String(row.paperId)) ?? (next.papers.some((p) => p.id === row.paperId) ? String(row.paperId) : null) : null;
    const datasetId = row.datasetId ? dsMap.get(String(row.datasetId)) ?? (next.datasets.some((d) => d.id === row.datasetId) ? String(row.datasetId) : null) : null;
    if ((row.paperId && !paperId) || (row.datasetId && !datasetId) || (!paperId && !datasetId)) throw new Error("资料引用的文献或数据集不存在。");
    const url = safeURL(row.url);
    if (!url) throw new Error("资料链接必须是有效的 HTTP 或 HTTPS 地址。");
    if (next.resources.some((r) => r.url === url && r.paperId === paperId && r.datasetId === datasetId)) {
      stats.duplicates++;
      continue;
    }
    const type = enumValue(row.type, Object.keys(RESOURCE_TYPES), "other");
    next.resources.push({ id: uid("IMP-R"), paperId, datasetId, url, type, title: textValue(row.title, 1000) || "导入资料", status: "linked", isDemo: row.isDemo === true, addedAt: nowISO() });
    stats.resources++;
  }
  return { next, stats };
}
