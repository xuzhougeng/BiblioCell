# 导出

筛选完一批文献后，结果可以从文献库页和数据集页右上角的“导出”按钮导出，也可以用命令行 `export`。桌面应用弹出保存对话框，由你选择位置；Demo 模式导出的是浏览器里的 Demo 数据，真实数据库模式导出的是正式库，两者都不会写入数据库。在线浏览器预览里文件直接下载。总览页“整理覆盖”面板有“导出进度报告”的快捷入口。

文献库页的范围可选“当前筛选结果”“全部已收录”或“全部文献”，数据集页可选“当前搜索结果”或“全部数据集”。格式：

| 格式 | 文件 | 内容 |
|---|---|---|
| `bibtex` | `.bib` | 每篇一个 `@article`，题目加双层花括号保留大小写，`\&`、`\%`、`\_` 等已转义，作者 `A; B` 写成 `A and B`，附 DOI、URL、PMID、物种/组织/测序类型/平台作 `keywords`，预印本加 `note = {Preprint}`。引用键为“第一作者姓 + 年份 + 题目首个长词”，重复时加 a、b 后缀 |
| `papers-csv` | `.csv` | 每行一篇：`id, title, authors, journal, year, doi, pmid, publication_status, species, species_label, organs, modalities, platforms, article_types, screening, screening_reason, fulltext, reading, tags, dataset_accessions, dataset_ids, source, added_at`。多值字段用 `; ` 连接 |
| `report` | `.md` | 筛选进度报告：纳入 / 不纳入 / 证据不足 / 待判断数量、待处理的 agent 建议、来源更新新增未核查数、已收录文献的全文覆盖；数据集整理状态与可用性；按物种、测序类型、组织的数据集数（含已核实、公开）和已收录文献数；最后列出已收录文献的题目、年份、DOI 和 accession。`-title` 自定义标题 |
| `datasets-csv` | `.csv` | 每行一个数据集：`accessions`、`repositories`、`accession_kinds`（如 `geo/series; sra/run`）、`verified_accessions`、整理状态、可用性、细胞数、关联文献 DOI / ID / 角色 |
| `datasets-summary` | `.csv` | 按 `-group-by species|organ|modality` 聚合：数据集数、已核实数、公开数、关联文献数、accession 数。多物种的数据集在每个物种下各计一次，没填的记为 `(unspecified)` |
| `download-script` | `.sh` | 批量下载脚本。按 accession 前缀分类：GEO 系列用 wget 取 `suppl/` 和 SOFT family 文件；SRA 的 SRP/SRX/SRS/SRR 用 `prefetch` + `fasterq-dump`；ENA / DDBJ 的 ERR/ERP/PRJEB 等从 ENA 文件报告取 fastq 链接；PRJNA 用 entrez-direct 展开成 SRR 再 prefetch；GSA、BioStudies、Zenodo 等没有批量工具的列为 `manual`，附页面地址。每条命令上方注释数据集题目和关联文献 DOI。应用只生成文件，不运行；`OUT=/data/scrna bash xxx.sh` 指定目标目录 |
| `accessions` | `.txt` | 每行一个 accession，去重，可直接 `xargs prefetch` 或交给其他工具 |

CSV 带 UTF-8 BOM 和 CRLF，Excel 直接打开；以 `=`、`+`、`-`、`@` 开头的单元格前面加 `'`，防止被当公式执行。accession 的识别以前缀为准：记成 GEO 但编号是 `SRR…` 的按 SRA 处理，前缀不认识的沿用记录的仓库名。

命令行的 `-q`、`-screening` 筛选文献（和 `papers list` 一样），`-status` 筛选数据集；数据集格式把 `-q` 用在数据集上。

```bash
./BiblioCell export report -title "Week 44" -o report.md
./BiblioCell export download-script -status verified > download.sh
./BiblioCell export datasets-summary -group-by modality
```

示例：

```bibtex
@article{zhang2024singlecell,
  title = {{Single-cell atlas of Arabidopsis \& rice roots}},
  author = {Zhang, Wei and O'Brien, Ann},
  journal = {Nature Plants},
  year = {2024},
  doi = {10.1000/abc\_1},
  url = {https://doi.org/10.1000/abc\_1},
  keywords = {Arabidopsis thaliana, root, scRNA-seq, 10x Genomics Chromium},
}
```

```bash
# ---- GEO (series) ----
# Root atlas data (doi: 10.1000/abc_1)
geo_series GSE123456 'https://ftp.ncbi.nlm.nih.gov/geo/series/GSE123nnn/GSE123456/'

# ---- SRA (prefetch + fasterq-dump) ----
# Root atlas data (doi: 10.1000/abc_1)
sra SRP111111
```

---

[← 返回文档目录](README.md)
