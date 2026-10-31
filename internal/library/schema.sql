-- BiblioCell · schema v5
-- SQLite 3.38+。每个数据库连接都必须启用 PRAGMA foreign_keys = ON。
-- 首先保存文献；全文、数据编号和样本信息均可后补。未知数量使用 NULL。
PRAGMA foreign_keys = ON;

BEGIN;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 采集程序只更新文献信息；人工笔记、标签与判断在 paper_curation 中。
CREATE TABLE IF NOT EXISTS papers (
    id TEXT PRIMARY KEY NOT NULL,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    abstract TEXT,
    authors_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(authors_json) AND json_type(authors_json) = 'array'),
    journal TEXT,
    -- 初筛获得的研究范围：即使尚未关联数据集也能按组织、技术和处理筛选。
    -- 原文与来源放 field_evidence；人工修正存 paper_curation 的 override 字段。
    organs_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(organs_json) AND json_type(organs_json) = 'array'),
    modalities_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(modalities_json) AND json_type(modalities_json) = 'array'),
    -- 测序平台与组织、技术一样是数组：一篇文献可用 10x 与 BD 等多个平台。
    platforms_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(platforms_json) AND json_type(platforms_json) = 'array'),
    conditions_text TEXT,
    -- 物种与文章类型是多值字段；species_json / article_types_json 是权威存储。
    -- species_text / species_label / article_type 已废弃：应用不再写入，仅为读取
    -- 旧库（迁移回填时取其值）而保留列定义；新记录保持 NULL / 'unknown'。
    species_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(species_json) AND json_type(species_json) = 'array'),
    species_text TEXT,
    species_label TEXT,
    source TEXT,
    online_date TEXT,
    issue_year INTEGER CHECK (issue_year IS NULL OR issue_year BETWEEN 1800 AND 2200),
    publication_state TEXT NOT NULL DEFAULT 'unknown'
        CHECK (publication_state IN ('unknown','preprint','published','unpublished')),
    article_type TEXT NOT NULL DEFAULT 'unknown'
        CHECK (article_type IN ('unknown','original','method','review','other')),
    article_types_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(article_types_json) AND json_type(article_types_json) = 'array'),
    is_retracted INTEGER NOT NULL DEFAULT 0 CHECK (is_retracted IN (0,1)),
    canonical_url TEXT,
    fulltext_checked_at TEXT,
    data_checked_at TEXT,
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    source_updated_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- DOI 存裸 DOI：小写、去空格，不存 https://doi.org/ 前缀。
-- 标识符缺失时不插入占位符；(namespace, value) 是跨文献的唯一键。
CREATE TABLE IF NOT EXISTS paper_identifiers (
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL CHECK (namespace IN ('doi','pmid','pmcid','biorxiv','arxiv','other')),
    value TEXT NOT NULL CHECK (length(trim(value)) > 0 AND value = trim(value)),
    raw_value TEXT,
    PRIMARY KEY (namespace, value),
    CHECK (namespace <> 'doi' OR
        (value = lower(value) AND value LIKE '10.%/%' AND instr(value, ' ') = 0))
);
CREATE INDEX IF NOT EXISTS idx_identifiers_paper ON paper_identifiers(paper_id);

CREATE TABLE IF NOT EXISTS paper_relations (
    source_paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    target_paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL CHECK (relation_type IN
        ('preprint_of','version_of','correction_of','retraction_of','related_to')),
    evidence_url TEXT,
    evidence_text TEXT,
    verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    PRIMARY KEY (source_paper_id, target_paper_id, relation_type),
    CHECK (source_paper_id <> target_paper_id)
);
CREATE INDEX IF NOT EXISTS idx_paper_relations_target ON paper_relations(target_paper_id);

CREATE TABLE IF NOT EXISTS paper_curation (
    paper_id TEXT PRIMARY KEY NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    screening_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (screening_status IN ('pending','included','excluded','uncertain')),
    screening_reason TEXT,
    screening_method TEXT NOT NULL DEFAULT 'unreviewed'
        CHECK (screening_method IN ('unreviewed','rule','model','human')),
    screening_reviewed_at TEXT,
    reading_status TEXT NOT NULL DEFAULT 'unread'
        CHECK (reading_status IN ('unread','skimmed','read')),
    priority TEXT NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low','normal','high')),
    starred INTEGER NOT NULL DEFAULT 0 CHECK (starred IN (0,1)),
    tags_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(tags_json) AND json_type(tags_json) = 'array'),
    -- NULL 表示使用来源值；[] / 空字符串可显式覆盖成空。采集程序不得写这些列。
    organs_override_json TEXT CHECK (organs_override_json IS NULL OR
        (json_valid(organs_override_json) AND json_type(organs_override_json) = 'array')),
    modalities_override_json TEXT CHECK (modalities_override_json IS NULL OR
        (json_valid(modalities_override_json) AND json_type(modalities_override_json) = 'array')),
    conditions_override_text TEXT,
    notes TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK (screening_status <> 'excluded' OR length(trim(coalesce(screening_reason,''))) > 0)
);
CREATE TRIGGER IF NOT EXISTS tr_paper_curation_init
AFTER INSERT ON papers BEGIN
    INSERT INTO paper_curation(paper_id) VALUES (NEW.id);
END;

-- Dataset 是内部整理单位，不能用 GSE、BioProject 或 DOI 替代主键。
CREATE TABLE IF NOT EXISTS datasets (
    id TEXT PRIMARY KEY NOT NULL,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    description TEXT,
    notes TEXT NOT NULL DEFAULT '',
    -- 多值研究范围以 JSON 为权威存储。tissue_text / modality / technology 已废弃：
    -- 应用不再写入，仅为读取旧库（迁移回填时取其值）而保留列定义，新记录保持
    -- NULL / 'unknown'。dataset_taxa 是可选的标准化物种补充。
    species_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(species_json) AND json_type(species_json) = 'array'),
    organs_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(organs_json) AND json_type(organs_json) = 'array'),
    modalities_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(modalities_json) AND json_type(modalities_json) = 'array'),
    platforms_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(platforms_json) AND json_type(platforms_json) = 'array'),
    dataset_kind TEXT NOT NULL DEFAULT 'original'
        CHECK (dataset_kind IN ('original','derived','collection')),
    modality TEXT NOT NULL DEFAULT 'unknown'
        CHECK (modality IN ('unknown','scRNA','snRNA','scATAC','snATAC',
            'multiome','scDNA','scMethyl','longread','other','mixed')),
    tissue_text TEXT,
    material_text TEXT,
    developmental_stage TEXT,
    conditions_text TEXT,
    technology TEXT,
    chemistry_version TEXT,
    n_cells_reported INTEGER CHECK (n_cells_reported IS NULL OR n_cells_reported >= 0),
    cell_count_unit TEXT CHECK (cell_count_unit IS NULL OR cell_count_unit IN
        ('cells','nuclei','observations','not_specified')),
    cell_count_scope TEXT,
    biological_replicates INTEGER CHECK (biological_replicates IS NULL OR biological_replicates >= 1),
    reference_assembly TEXT,
    reference_annotation TEXT,
    raw_availability TEXT NOT NULL DEFAULT 'unknown'
        CHECK (raw_availability IN ('unknown','available','unavailable','embargoed','restricted')),
    matrix_availability TEXT NOT NULL DEFAULT 'unknown'
        CHECK (matrix_availability IN ('unknown','available','unavailable','embargoed','restricted')),
    annotation_availability TEXT NOT NULL DEFAULT 'unknown'
        CHECK (annotation_availability IN ('unknown','available','unavailable','embargoed','restricted')),
    availability_checked_at TEXT,
    curation_level TEXT NOT NULL DEFAULT 'indexed'
        CHECK (curation_level IN ('indexed','samples_checked','files_verified','matrix_standardized','reprocessed')),
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK (n_cells_reported IS NULL OR
        (cell_count_unit IS NOT NULL AND length(trim(coalesce(cell_count_scope,''))) > 0))
);

-- source_dataset_id 为被描述的数据集，target_dataset_id 为它指向的数据集。
-- 例：整合数据集 A derived_from 原始数据集 B/C，应保存 A→B、A→C 两条关系。
-- 一个数据集可有多个来源；part_of 表示 source 是 target 集合的一部分。
CREATE TABLE IF NOT EXISTS dataset_relations (
    source_dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    target_dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL CHECK (relation_type IN
        ('derived_from','subset_of','part_of','overlap')),
    evidence_url TEXT,
    evidence_text TEXT,
    verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    PRIMARY KEY (source_dataset_id, target_dataset_id, relation_type),
    CHECK (source_dataset_id <> target_dataset_id)
);
CREATE INDEX IF NOT EXISTS idx_dataset_relations_target ON dataset_relations(target_dataset_id, relation_type);

CREATE TABLE IF NOT EXISTS repository_records (
    id TEXT PRIMARY KEY NOT NULL,
    repository TEXT NOT NULL CHECK (repository IN
        ('geo','sra','ena','gsa','bioproject','biosample','biostudies','zenodo','figshare','dryad','other')),
    accession TEXT NOT NULL CHECK (length(trim(accession)) > 0 AND accession = trim(accession)),
    record_kind TEXT NOT NULL DEFAULT 'unknown'
        CHECK (record_kind IN ('unknown','project','study','sample','experiment','run','analysis','collection','other')),
    url TEXT,
    public_status TEXT NOT NULL DEFAULT 'unknown'
        CHECK (public_status IN ('unknown','public','embargoed','restricted','withdrawn')),
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata_json)),
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    last_checked_at TEXT,
    UNIQUE (repository, accession)
);

-- 一条外部仓库记录可覆盖多个内部数据集；一个内部数据集也可分散于多个仓库。
CREATE TABLE IF NOT EXISTS dataset_repositories (
    dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    repository_record_id TEXT NOT NULL REFERENCES repository_records(id) ON DELETE CASCADE,
    mapping_scope TEXT NOT NULL DEFAULT 'unknown'
        CHECK (mapping_scope IN ('unknown','exact','subset','overlap','related')),
    evidence_url TEXT,
    evidence_text TEXT,
    verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    PRIMARY KEY (dataset_id, repository_record_id)
);
CREATE INDEX IF NOT EXISTS idx_dataset_repositories_record ON dataset_repositories(repository_record_id);

CREATE TABLE IF NOT EXISTS paper_datasets (
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('generated','reused','reprocessed','benchmarked','unknown')),
    evidence_url TEXT,
    evidence_text TEXT,
    verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    PRIMARY KEY (paper_id, dataset_id, role)
);
CREATE INDEX IF NOT EXISTS idx_paper_datasets_dataset ON paper_datasets(dataset_id);

CREATE TABLE IF NOT EXISTS taxa (
    taxid INTEGER PRIMARY KEY CHECK (taxid > 0),
    scientific_name TEXT NOT NULL CHECK (length(trim(scientific_name)) > 0),
    common_name TEXT,
    family TEXT,
    lineage_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(lineage_json) AND json_type(lineage_json) = 'array'),
    taxonomy_version TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS paper_taxa (
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    taxid INTEGER NOT NULL REFERENCES taxa(taxid),
    name_as_reported TEXT,
    PRIMARY KEY (paper_id, taxid)
);
CREATE INDEX IF NOT EXISTS idx_paper_taxa_taxid ON paper_taxa(taxid);
CREATE TABLE IF NOT EXISTS dataset_taxa (
    dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    taxid INTEGER NOT NULL REFERENCES taxa(taxid),
    name_as_reported TEXT,
    PRIMARY KEY (dataset_id, taxid)
);
CREATE INDEX IF NOT EXISTS idx_dataset_taxa_taxid ON dataset_taxa(taxid);

-- 附件可以只有远程 URL，也可只有用户手动导入的本地文件。
-- available 表示检查可访问；stored/parsed 表示本地已有文件，二者独立。
CREATE TABLE IF NOT EXISTS resources (
    id TEXT PRIMARY KEY NOT NULL,
    resource_type TEXT NOT NULL CHECK (resource_type IN
        ('fulltext','supplement','matrix','cell_metadata','feature_metadata','cell_annotation',
         'marker_table','raw_reads','code','reference','other')),
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    format TEXT,
    url TEXT,
    local_path TEXT,
    access_status TEXT NOT NULL DEFAULT 'unknown'
        CHECK (access_status IN ('unknown','available','unavailable','embargoed','restricted')),
    local_state TEXT NOT NULL DEFAULT 'none'
        CHECK (local_state IN ('none','stored','parsed')),
    size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
    checksum_algorithm TEXT CHECK (checksum_algorithm IS NULL OR checksum_algorithm IN ('sha256','md5')),
    checksum_value TEXT,
    license TEXT,
    last_checked_at TEXT,
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK (length(trim(coalesce(url,''))) > 0 OR length(trim(coalesce(local_path,''))) > 0),
    CHECK (local_state = 'none' OR length(trim(coalesce(local_path,''))) > 0),
    CHECK ((checksum_algorithm IS NULL AND checksum_value IS NULL) OR
        (checksum_algorithm IS NOT NULL AND checksum_value IS NOT NULL AND
         ((checksum_algorithm = 'sha256' AND length(checksum_value) = 64 AND checksum_value NOT GLOB '*[^0-9a-f]*') OR
          (checksum_algorithm = 'md5' AND length(checksum_value) = 32 AND checksum_value NOT GLOB '*[^0-9a-f]*'))))
);
CREATE INDEX IF NOT EXISTS idx_resources_type ON resources(resource_type, access_status);
CREATE TABLE IF NOT EXISTS paper_resources (
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    resource_id TEXT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    PRIMARY KEY (paper_id, resource_id)
);
CREATE INDEX IF NOT EXISTS idx_paper_resources_resource ON paper_resources(resource_id);
CREATE TABLE IF NOT EXISTS dataset_resources (
    dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    resource_id TEXT NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    PRIMARY KEY (dataset_id, resource_id)
);
CREATE INDEX IF NOT EXISTS idx_dataset_resources_resource ON dataset_resources(resource_id);

-- 各来源独立保存水位。只把完整成功的批次提交为下次增量的起点。
CREATE TABLE IF NOT EXISTS source_states (
    source_id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('literature','repository','manual','seed')),
    base_url TEXT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    query_text TEXT,
    query_version TEXT NOT NULL DEFAULT '1',
    config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
    committed_watermark TEXT,
    committed_cursor TEXT,
    last_successful_run_id TEXT REFERENCES harvest_runs(id),
    last_success_at TEXT,
    CHECK ((committed_watermark IS NULL AND committed_cursor IS NULL) OR last_successful_run_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS harvest_runs (
    id TEXT PRIMARY KEY NOT NULL,
    source_id TEXT NOT NULL REFERENCES source_states(source_id),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','running','succeeded','partial','failed')),
    query_text TEXT,
    query_version TEXT NOT NULL DEFAULT '1',
    window_start TEXT,
    window_end TEXT,
    cursor_start TEXT,
    cursor_next TEXT,
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    finished_at TEXT,
    records_seen INTEGER NOT NULL DEFAULT 0 CHECK (records_seen >= 0),
    records_new INTEGER NOT NULL DEFAULT 0 CHECK (records_new >= 0),
    records_updated INTEGER NOT NULL DEFAULT 0 CHECK (records_updated >= 0),
    error_message TEXT,
    UNIQUE (id, source_id),
    CHECK (window_start IS NULL OR window_end IS NULL OR window_start <= window_end),
    CHECK (status NOT IN ('succeeded','partial','failed') OR finished_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_harvest_source_time ON harvest_runs(source_id, started_at DESC);

CREATE TRIGGER IF NOT EXISTS tr_source_checkpoint_guard
BEFORE UPDATE OF committed_watermark, committed_cursor, last_successful_run_id ON source_states
WHEN NEW.last_successful_run_id IS NOT NULL BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM harvest_runs h WHERE h.id = NEW.last_successful_run_id
          AND h.source_id = NEW.source_id AND h.status = 'succeeded'
    ) THEN RAISE(ABORT, 'checkpoint requires a succeeded run from the same source') END;
END;

CREATE TRIGGER IF NOT EXISTS tr_source_checkpoint_insert_guard
BEFORE INSERT ON source_states
WHEN NEW.last_successful_run_id IS NOT NULL BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM harvest_runs h WHERE h.id = NEW.last_successful_run_id
          AND h.source_id = NEW.source_id AND h.status = 'succeeded'
    ) THEN RAISE(ABORT, 'initial checkpoint requires a succeeded run from the same source') END;
END;

-- 保存来源快照。相同源记录内容未变则不再重复插入，变更后保留新快照。
CREATE TABLE IF NOT EXISTS source_records (
    id TEXT PRIMARY KEY NOT NULL,
    source_id TEXT NOT NULL REFERENCES source_states(source_id),
    external_id TEXT NOT NULL,
    harvest_run_id TEXT,
    payload_sha256 TEXT NOT NULL
        CHECK (length(payload_sha256) = 64 AND payload_sha256 NOT GLOB '*[^0-9a-f]*'),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    source_url TEXT,
    source_updated_at TEXT,
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    paper_id TEXT REFERENCES papers(id) ON DELETE SET NULL,
    dataset_id TEXT REFERENCES datasets(id) ON DELETE SET NULL,
    UNIQUE (source_id, external_id, payload_sha256),
    FOREIGN KEY (harvest_run_id, source_id) REFERENCES harvest_runs(id, source_id)
);
CREATE INDEX IF NOT EXISTS idx_source_records_external ON source_records(source_id, external_id);
CREATE INDEX IF NOT EXISTS idx_source_records_paper ON source_records(paper_id);

-- 只对关键字段保存证据：收录判断、物种、数据关联、细胞数量等。
-- 不需要为每个自动填充字段立即建一条证据记录。
CREATE TABLE IF NOT EXISTS field_evidence (
    id TEXT PRIMARY KEY NOT NULL,
    target_paper_id TEXT REFERENCES papers(id) ON DELETE CASCADE,
    target_dataset_id TEXT REFERENCES datasets(id) ON DELETE CASCADE,
    target_resource_id TEXT REFERENCES resources(id) ON DELETE CASCADE,
    target_repository_record_id TEXT REFERENCES repository_records(id) ON DELETE CASCADE,
    field_name TEXT NOT NULL CHECK (length(trim(field_name)) > 0),
    raw_value TEXT,
    normalized_value TEXT,
    source_url TEXT,
    source_record_id TEXT REFERENCES source_records(id),
    source_resource_id TEXT REFERENCES resources(id),
    source_location TEXT,
    excerpt TEXT,
    extraction_method TEXT NOT NULL CHECK (extraction_method IN ('source','parser','model','human')),
    extractor_version TEXT,
    review_status TEXT NOT NULL DEFAULT 'unreviewed'
        CHECK (review_status IN ('unreviewed','verified','conflict','rejected')),
    observed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK ((target_paper_id IS NOT NULL) + (target_dataset_id IS NOT NULL) +
           (target_resource_id IS NOT NULL) + (target_repository_record_id IS NOT NULL) = 1),
    CHECK (source_url IS NOT NULL OR source_record_id IS NOT NULL OR source_resource_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_evidence_paper ON field_evidence(target_paper_id, field_name);
CREATE INDEX IF NOT EXISTS idx_evidence_dataset ON field_evidence(target_dataset_id, field_name);

CREATE INDEX IF NOT EXISTS idx_papers_date ON papers(online_date DESC);
CREATE INDEX IF NOT EXISTS idx_papers_first_seen ON papers(first_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_curation_screening ON paper_curation(screening_status, priority);
CREATE INDEX IF NOT EXISTS idx_curation_reading ON paper_curation(reading_status);
CREATE INDEX IF NOT EXISTS idx_datasets_modality ON datasets(modality, curation_level);

-- 状态由附件和关联计算，避免复制一份 fulltext_status 后与实际附件脱节。
CREATE VIEW IF NOT EXISTS v_paper_overview AS
SELECT p.*, c.screening_status, c.screening_reason, c.screening_method,
       c.reading_status, c.priority, c.starred, c.tags_json, c.notes,
       coalesce(c.organs_override_json, p.organs_json) AS effective_organs_json,
       coalesce(c.modalities_override_json, p.modalities_json) AS effective_modalities_json,
       coalesce(c.conditions_override_text, p.conditions_text) AS effective_conditions_text,
       CASE
         WHEN EXISTS (SELECT 1 FROM paper_resources pr JOIN resources r ON r.id = pr.resource_id
              WHERE pr.paper_id = p.id AND r.resource_type = 'fulltext' AND r.local_state IN ('stored','parsed')) THEN 'stored'
         WHEN EXISTS (SELECT 1 FROM paper_resources pr JOIN resources r ON r.id = pr.resource_id
              WHERE pr.paper_id = p.id AND r.resource_type = 'fulltext' AND r.url IS NOT NULL
              AND r.access_status IN ('unknown','available')) THEN 'linked'
         WHEN EXISTS (SELECT 1 FROM paper_resources pr JOIN resources r ON r.id = pr.resource_id
              WHERE pr.paper_id = p.id AND r.resource_type = 'fulltext') THEN 'unavailable'
         WHEN p.fulltext_checked_at IS NULL THEN 'not_checked'
         ELSE 'missing' END AS fulltext_status,
       (SELECT count(DISTINCT pd.dataset_id) FROM paper_datasets pd WHERE pd.paper_id = p.id) AS dataset_count,
       CASE
         WHEN EXISTS (SELECT 1 FROM paper_datasets pd WHERE pd.paper_id = p.id)
          AND NOT EXISTS (
            SELECT 1 FROM paper_datasets pd WHERE pd.paper_id = p.id AND (
              pd.verified = 0 OR (
                NOT EXISTS (SELECT 1 FROM dataset_repositories dr WHERE dr.dataset_id = pd.dataset_id AND dr.verified = 1)
                AND NOT EXISTS (SELECT 1 FROM dataset_resources dsr JOIN resources r ON r.id = dsr.resource_id
                    WHERE dsr.dataset_id = pd.dataset_id AND (r.access_status = 'available' OR r.local_state IN ('stored','parsed')))
              )
            )
          ) THEN 'verified'
         WHEN EXISTS (SELECT 1 FROM paper_datasets pd WHERE pd.paper_id = p.id AND pd.verified = 1) THEN 'partial'
         WHEN EXISTS (SELECT 1 FROM paper_datasets pd WHERE pd.paper_id = p.id) THEN 'linked'
         WHEN p.data_checked_at IS NULL THEN 'not_checked'
         ELSE 'missing' END AS data_link_status
FROM papers p JOIN paper_curation c ON c.paper_id = p.id;

CREATE VIEW IF NOT EXISTS v_dataset_overview AS
SELECT d.*,
       (SELECT count(DISTINCT paper_id) FROM paper_datasets pd WHERE pd.dataset_id = d.id) AS paper_count,
       (SELECT count(*) FROM dataset_repositories dr WHERE dr.dataset_id = d.id) AS repository_record_count,
       (SELECT count(*) FROM dataset_resources dsr WHERE dsr.dataset_id = d.id) AS resource_count
FROM datasets d;

-- 同一篇文献可有多条待办；排除的文献默认不进入全文和数据待补队列。
CREATE VIEW IF NOT EXISTS v_work_queue AS
SELECT id AS paper_id, title, priority, 'screening' AS task_kind, '待判断收录范围' AS task_label
FROM v_paper_overview WHERE screening_status IN ('pending','uncertain')
UNION ALL
SELECT id, title, priority, 'fulltext', '待补全文'
FROM v_paper_overview WHERE screening_status = 'included' AND fulltext_status <> 'stored'
UNION ALL
SELECT id, title, priority, 'data_link', '待核实数据关联'
FROM v_paper_overview WHERE screening_status = 'included' AND data_link_status <> 'verified';

INSERT OR IGNORE INTO schema_migrations(version, description)
VALUES (1, '文献优先：数据集、附件、人工整理与采集追踪'),
       (2, '文献原型映射：物种显示名称与发现来源'),
       (3, '多值研究范围：物种、测序平台、组织、技术与文章类型改为 JSON 数组'),
       (4, '数据集多值研究范围、物种回填与人工整理备注'),
       (5, '公开来源配置、手动同步与外部记录快照');
PRAGMA user_version = 5;
COMMIT;
