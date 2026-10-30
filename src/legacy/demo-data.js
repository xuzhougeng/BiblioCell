/* All records in this file are fictional, clearly labelled demonstration data. */
(function () {
  'use strict';
  // [title, species[[name,label]…], organs[…], modalities[…], platforms[…], year, screening, fulltext, reading, starred]
  const specimens = [
    ['拟南芥根尖发育的单细胞转录组图谱', [['Arabidopsis thaliana','拟南芥']], ['根'], ['scRNA-seq'], ['10x Genomics Chromium'], 2026, 'included', 'linked', 'read', true],
    ['水稻根系盐胁迫响应的单核转录组研究', [['Oryza sativa','水稻']], ['根'], ['snRNA-seq'], ['10x Genomics Chromium'], 2026, 'included', 'missing', 'skimmed', true],
    ['玉米幼叶细胞类型与染色质可及性', [['Zea mays','玉米']], ['叶'], ['scATAC-seq'], ['10x Genomics Chromium'], 2026, 'pending', 'missing', 'unread', false],
    ['拟南芥与水稻根部细胞注释方法的比较评估', [['Arabidopsis thaliana','拟南芥'],['Oryza sativa','水稻'],['Zea mays','玉米']], ['根'], ['scRNA-seq','snRNA-seq'], ['10x Genomics Chromium','BD Rhapsody'], 2026, 'included', 'linked', 'skimmed', true],
    ['番茄果实成熟过程中的细胞状态变化', [['Solanum lycopersicum','番茄']], ['果实'], ['scRNA-seq'], ['10x Genomics Chromium'], 2025, 'included', 'missing', 'unread', false],
    ['大豆根瘤形成的单细胞表达图谱', [['Glycine max','大豆']], ['根瘤','根'], ['scRNA-seq'], ['Microwell-seq'], 2025, 'included', 'linked', 'read', false],
    ['小麦幼穗发育的单核转录组分析', [['Triticum aestivum','小麦']], ['花序'], ['snRNA-seq'], ['10x Genomics Chromium'], 2025, 'included', 'missing', 'unread', false],
    ['拟南芥叶片的 RNA 与 ATAC 联合测量', [['Arabidopsis thaliana','拟南芥']], ['叶'], ['Multiome'], ['10x Genomics Chromium'], 2025, 'included', 'linked', 'skimmed', false],
    ['棉花纤维发育中的细胞异质性', [['Gossypium hirsutum','棉花']], ['纤维'], ['scRNA-seq'], ['BD Rhapsody'], 2024, 'pending', 'missing', 'unread', false],
    ['杨树形成层分化的单核转录组图谱', [['Populus trichocarpa','杨树']], ['茎'], ['snRNA-seq'], ['10x Genomics Chromium'], 2024, 'included', 'missing', 'unread', false],
    ['拟南芥根部公共数据的重新分析', [['Arabidopsis thaliana','拟南芥']], ['根'], ['scRNA-seq'], ['10x Genomics Chromium'], 2024, 'included', 'linked', 'read', true],
    ['苔藓再生过程中的单细胞表达变化', [['Physcomitrium patens','小立碗藓']], ['原丝体'], ['scRNA-seq'], ['Microwell-seq'], 2023, 'pending', 'missing', 'unread', false],
    ['植物单细胞长读长转录本识别方法', [['Arabidopsis thaliana','拟南芥'],['Oryza sativa','水稻']], ['根'], ['sc-long-read'], ['10x Genomics Chromium'], 2023, 'included', 'missing', 'unread', false],
    ['植物组织空间表达研究的资料记录', [['Oryza sativa','水稻']], ['叶'], ['空间转录组'], [], 2022, 'excluded', 'missing', 'skimmed', false]
  ];
  const papers = specimens.map((r, i) => ({
    id: 'DEMO-P-' + String(i + 1).padStart(3, '0'),
    title: r[0],
    species: r[1].map(([name, label]) => ({ name, label })),
    organs: r[2], modalities: r[3], platforms: r[4],
    articleTypes: i === 13 ? ['相关资料'] : i === 3 ? ['原创研究', '方法研究'] : i === 10 || i === 12 ? ['方法研究'] : ['原创研究'],
    year: r[5], screening: r[6], fulltext: r[7], reading: r[8], starred: r[9],
    doi: '', pmid: '', authors: '示例研究组 ' + String.fromCharCode(65 + i),
    journal: i === 3 || i === 10 || i === 12 ? '方法研究 · 示例' : '原创研究 · 示例',
    publicationStatus: i === 2 || i === 11 ? 'preprint' : 'published',
    abstract: '此条目为网页原型的虚构示例，用于展示' + r[1].map(s => s[1]).join('与') + r[2][0] + '相关研究的整理方式，并不对应一篇真实发表论文。正式使用时可以在这里保存原始摘要、实验条件、研究对象和测序方法。文献可以先收录，全文及数据集信息在之后逐步补充。',
    tags: i === 3 || i === 10 ? ['跨研究比较', '数据复用'] : i === 1 ? ['胁迫响应', '重点关注'] : i === 7 ? ['多组学'] : [r[2][0] + '发育'],
    notes: i === 0 ? '示例笔记：优先补齐取样部位、发育阶段，以及各重复与文库的对应关系。' : '',
    source: ['Europe PMC', 'PubMed', 'bioRxiv', '手动整理'][i % 4],
    addedAt: new Date(Date.UTC(2026, 9, 30 - i, 2, 30)).toISOString(),
    updatedAt: new Date(Date.UTC(2026, 9, 30 - i, 2, 30)).toISOString(),
    screeningReason: i === 13 ? '示例：仅包含空间转录组，作为相关资料保留。' : r[6] === 'pending' ? '示例：等待更多信息确认实际数据类型。' : '示例：实际产生或分析植物单细胞 / 单核测序数据。',
    isDemo: true
  }));
  const speciesText = p => p.species.map(s => s.label).join('·');
  const links = [[0], [1], [4], [5], [6], [7], [9], [12]];
  const datasets = links.map((indexes, i) => {
    const p = papers[indexes[0]];
    return { id: 'DEMO-DS-' + String(i + 1).padStart(3, '0'), title: speciesText(p) + ' · ' + p.organs[0] + ' · ' + p.modalities[0],
      species: p.species[0].name, speciesLabel: p.species[0].label, organ: p.organs[0], modality: p.modalities[0],
      accession: '', repository: ['GEO', 'GSA', 'ENA'][i % 3], availability: 'unknown',
      matrixAvailability: 'unknown', annotationAvailability: 'unknown',
      status: i < 3 ? 'identified' : 'pending', cellsReported: null,
      notes: '这是演示数据集，尚未关联真实仓库编号或文件。', isDemo: true,
      paperLinks: indexes.map(index => ({ paperId: papers[index].id, role: 'generated' })),
      addedAt: p.addedAt };
  });
  datasets[0].paperLinks.push({ paperId: papers[3].id, role: 'reused' }, { paperId: papers[10].id, role: 'reprocessed' });
  datasets[1].paperLinks.push({ paperId: papers[3].id, role: 'reused' });
  const resources = papers.filter(p => p.fulltext === 'linked').map((p, i) => ({
    id: 'DEMO-R-' + String(i + 1).padStart(3, '0'), paperId: p.id, datasetId: null,
    type: 'fulltext', title: '全文检索入口 · 示例', url: 'https://europepmc.org/',
    status: 'linked', isDemo: true, addedAt: p.addedAt
  }));
  window.PLANT_DEMO_DATA = { schemaVersion: 2, papers, datasets, resources, activities: [] };
})();
