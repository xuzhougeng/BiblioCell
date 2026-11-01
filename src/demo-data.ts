// All records in this file are fictional, clearly labelled demonstration data.
import type { Dataset, Paper } from "./mygo.ts";
import type { LibraryState, Resource } from "./model.ts";

type Specimen = [
  title: string, species: [name: string, label: string][], organs: string[], modalities: string[], platforms: string[],
  year: number, screening: string, fulltext: string, reading: string, starred: boolean,
];

const specimens: Specimen[] = [
  ["拟南芥根尖发育的单细胞转录组图谱", [["Arabidopsis thaliana", "拟南芥"]], ["根"], ["scRNA-seq"], ["10x Genomics Chromium"], 2026, "included", "linked", "read", true],
  ["水稻根系盐胁迫响应的单核转录组研究", [["Oryza sativa", "水稻"]], ["根"], ["snRNA-seq"], ["10x Genomics Chromium"], 2026, "included", "missing", "skimmed", true],
  ["玉米幼叶细胞类型与染色质可及性", [["Zea mays", "玉米"]], ["叶"], ["scATAC-seq"], ["10x Genomics Chromium"], 2026, "pending", "missing", "unread", false],
  ["拟南芥与水稻根部细胞注释方法的比较评估", [["Arabidopsis thaliana", "拟南芥"], ["Oryza sativa", "水稻"], ["Zea mays", "玉米"]], ["根"], ["scRNA-seq", "snRNA-seq"], ["10x Genomics Chromium", "BD Rhapsody"], 2026, "included", "linked", "skimmed", true],
  ["番茄果实成熟过程中的细胞状态变化", [["Solanum lycopersicum", "番茄"]], ["果实"], ["scRNA-seq"], ["10x Genomics Chromium"], 2025, "included", "missing", "unread", false],
  ["大豆根瘤形成的单细胞表达图谱", [["Glycine max", "大豆"]], ["根瘤", "根"], ["scRNA-seq"], ["Microwell-seq"], 2025, "included", "linked", "read", false],
  ["小麦幼穗发育的单核转录组分析", [["Triticum aestivum", "小麦"]], ["花序"], ["snRNA-seq"], ["10x Genomics Chromium"], 2025, "included", "missing", "unread", false],
  ["拟南芥叶片的 RNA 与 ATAC 联合测量", [["Arabidopsis thaliana", "拟南芥"]], ["叶"], ["Multiome"], ["10x Genomics Chromium"], 2025, "included", "linked", "skimmed", false],
  ["棉花纤维发育中的细胞异质性", [["Gossypium hirsutum", "棉花"]], ["纤维"], ["scRNA-seq"], ["BD Rhapsody"], 2024, "pending", "missing", "unread", false],
  ["杨树形成层分化的单核转录组图谱", [["Populus trichocarpa", "杨树"]], ["茎"], ["snRNA-seq"], ["10x Genomics Chromium"], 2024, "included", "missing", "unread", false],
  ["拟南芥根部公共数据的重新分析", [["Arabidopsis thaliana", "拟南芥"]], ["根"], ["scRNA-seq"], ["10x Genomics Chromium"], 2024, "included", "linked", "read", true],
  ["苔藓再生过程中的单细胞表达变化", [["Physcomitrium patens", "小立碗藓"]], ["原丝体"], ["scRNA-seq"], ["Microwell-seq"], 2023, "pending", "missing", "unread", false],
  ["植物单细胞长读长转录本识别方法", [["Arabidopsis thaliana", "拟南芥"], ["Oryza sativa", "水稻"]], ["根"], ["sc-long-read"], ["10x Genomics Chromium"], 2023, "included", "missing", "unread", false],
  ["植物组织空间表达研究的资料记录", [["Oryza sativa", "水稻"]], ["叶"], ["空间转录组"], [], 2022, "excluded", "missing", "skimmed", false],
];

/** demoState builds a fresh copy of the demonstration library. */
export function demoState(): LibraryState {
  const papers: Paper[] = specimens.map((r, i) => {
    const at = new Date(Date.UTC(2026, 9, 30 - i, 2, 30)).toISOString();
    return {
      id: `DEMO-P-${String(i + 1).padStart(3, "0")}`,
      title: r[0],
      species: r[1].map(([name, label]) => ({ name, label })),
      organs: r[2], modalities: r[3], platforms: r[4],
      articleTypes: i === 13 ? ["相关资料"] : i === 3 ? ["原创研究", "方法研究"] : i === 10 || i === 12 ? ["方法研究"] : ["原创研究"],
      year: r[5], screening: r[6], fulltext: r[7], fulltextUrl: "", reading: r[8], starred: r[9],
      reviewedAt: "", proposedScreening: "", proposedReason: "", batchId: "",
      doi: "", pmid: "", authors: `示例研究组 ${String.fromCharCode(65 + i)}`,
      journal: i === 3 || i === 10 || i === 12 ? "方法研究 · 示例" : "原创研究 · 示例",
      publicationStatus: i === 2 || i === 11 ? "preprint" : "published",
      abstract: `此条目为网页原型的虚构示例，用于展示${r[1].map((s) => s[1]).join("与")}${r[2][0] ?? ""}相关研究的整理方式，并不对应一篇真实发表论文。正式使用时可以在这里保存原始摘要、实验条件、研究对象和测序方法。文献可以先收录，全文及数据集信息在之后逐步补充。`,
      tags: i === 3 || i === 10 ? ["跨研究比较", "数据复用"] : i === 1 ? ["胁迫响应", "重点关注"] : i === 7 ? ["多组学"] : [`${r[2][0] ?? ""}发育`],
      notes: i === 0 ? "示例笔记：优先补齐取样部位、发育阶段，以及各重复与文库的对应关系。" : "",
      source: ["Europe PMC", "PubMed", "bioRxiv", "手动整理"][i % 4] ?? "手动整理",
      addedAt: at, updatedAt: at,
      screeningReason: i === 13 ? "示例：仅包含空间转录组，作为相关资料保留。" : r[6] === "pending" ? "示例：等待更多信息确认实际数据类型。" : "示例：实际产生或分析植物单细胞 / 单核测序数据。",
      isDemo: true,
    };
  });
  const paperAt = (index: number): Paper => papers[index] as Paper;
  const links = [[0], [1], [4], [5], [6], [7], [9], [12]];
  const datasets: Dataset[] = links.map((indexes, i) => {
    const p = paperAt(indexes[0] ?? 0);
    // The first demo dataset carries a fictional repository record, so the
    // accession chips and repo links have something to show in the preview.
    const repositories = i === 0
      ? [{ id: "DEMO-REPO-001", repository: "geo", accession: "GSE000001", recordKind: "", url: "https://www.ncbi.nlm.nih.gov/geo/", publicStatus: "public", mappingScope: "exact", evidenceUrl: "", evidenceText: "示例编号，点击跳转 GEO 官网首页。", verified: false }]
      : [];
    return {
      id: `DEMO-DS-${String(i + 1).padStart(3, "0")}`,
      title: `${p.species.map((s) => s.label).join("·")} · ${p.organs[0] ?? ""} · ${p.modalities[0] ?? ""}`,
      description: "", notes: i === 0 ? "演示数据集：编号为虚构示例，点击后跳转 GEO 官网首页。" : "这是演示数据集，尚未关联真实仓库编号或文件。",
      species: p.species.slice(0, 1), organs: p.organs.slice(0, 1), modalities: p.modalities.slice(0, 1), platforms: [],
      repositories, repository: repositories[0]?.repository ?? "", accession: repositories[0]?.accession ?? "",
      status: i < 3 ? "identified" : "pending", curationLevel: "",
      availability: "unknown", matrixAvailability: "unknown", annotationAvailability: "unknown",
      cellsReported: null, cellCountUnit: "", cellCountScope: "",
      paperLinks: indexes.map((index) => ({ paperId: paperAt(index).id, role: "generated", evidenceUrl: "", evidenceText: "", verified: false })),
      reviewedAt: "", batchId: "", addedAt: p.addedAt, updatedAt: p.addedAt, isDemo: true,
    };
  });
  const link = (dataset: number, paper: number, role: string) =>
    datasets[dataset]?.paperLinks.push({ paperId: paperAt(paper).id, role, evidenceUrl: "", evidenceText: "", verified: false });
  link(0, 3, "reused");
  link(0, 10, "reprocessed");
  link(1, 3, "reused");
  const resources: Resource[] = papers.filter((p) => p.fulltext === "linked").map((p, i) => ({
    id: `DEMO-R-${String(i + 1).padStart(3, "0")}`, paperId: p.id, datasetId: null,
    type: "fulltext", title: "全文检索入口 · 示例", url: "https://europepmc.org/",
    status: "linked", isDemo: true, addedAt: p.addedAt,
  }));
  return { schemaVersion: 2, papers, datasets, resources, activities: [] };
}
