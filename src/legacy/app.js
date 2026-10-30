(function () {
  'use strict';
  const KEY = 'plantcell-library-prototype-v2';
  const $ = (s, root = document) => root.querySelector(s);
  const $$ = (s, root = document) => Array.from(root.querySelectorAll(s));
  const esc = v => String(v == null ? '' : v).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const I = (n, size = 18) => typeof window.icon === 'function' ? window.icon(n, size) : '';
  const clone = x => JSON.parse(JSON.stringify(x));
  const safeURL = value => { try { const u = new URL(String(value).trim()); return ['https:', 'http:'].includes(u.protocol) && !u.username && !u.password ? u.href : ''; } catch (_) { return ''; } };
  const dateText = value => { const d = new Date(value); return isNaN(d.getTime()) ? '日期待补' : d.toLocaleDateString('zh-CN', {year:'numeric',month:'2-digit',day:'2-digit'}).replaceAll('/', '.'); };
  const normalizeDOI = value => String(value || '').trim().replace(/^https?:\/\/(?:dx\.)?doi\.org\//i, '').replace(/^doi:\s*/i, '').trim().toLowerCase();
  const validDOI = value => !value || /^10\.\d{4,9}\/[^\s<>"']+$/i.test(value);
  const uid = prefix => prefix + '-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 7);
  const SCREEN = { included: '已纳入', pending: '待判断', uncertain: '证据不足', excluded: '相关 / 排除' };
  const READING = { unread: '未读', skimmed: '已浏览', read: '已精读' };
  const FULL = { missing: '待补入口', linked: '已有入口', stored: '已保存文件' };
  const ROLE = { generated: '新生成', reused: '复用', reprocessed: '重新处理', benchmarked: '基准比较', unknown: '关系待确认' };
  const pageMeta = {
    overview: ['研究总览', '把文献收齐，让数据逐步完整。'],
    papers: ['文献库', '先收录，再阅读、关联与补全。'],
    datasets: ['数据集', '沿着文献找到数据，保留来源与复用关系。'],
    tasks: ['待补事项', '从整理状态生成，按需要逐步完成。'],
    sources: ['更新与来源', '记录发现入口和整理活动。'],
    schema: ['数据库结构', '三个核心对象，连接文献、数据与资料。'],
    settings: ['设置', '让研究库按照你的方式保存、阅读与工作。']
  };
  const realMode = window.PLANT_REAL_MODE === true;
  let state;
  let persistenceError = false;
  if (realMode) {
    state = clone(window.PLANT_INITIAL_STATE || {schemaVersion:1,papers:[],datasets:[],resources:[],activities:[]});
    document.body.classList.add('real-mode');
  } else {
    try {
      const saved = localStorage.getItem(KEY);
      const candidate = saved ? JSON.parse(saved) : null;
      state = candidate && candidate.schemaVersion === 2 && Array.isArray(candidate.papers) && Array.isArray(candidate.datasets) && Array.isArray(candidate.resources) ? candidate : clone(window.PLANT_DEMO_DATA);
    } catch (_) { state = clone(window.PLANT_DEMO_DATA); persistenceError = true; }
  }
  if (!Array.isArray(state.activities)) state.activities = [];
  let page = pageMeta[location.hash.slice(1)] ? location.hash.slice(1) : 'overview';
  let filters = { search:'', species:'', organ:'', year:'', modality:'', platform:'', fulltext:'', screening:'', starred:false, sort:'newest' };
  const emptyFilters = () => ({ search:'', species:'', organ:'', year:'', modality:'', platform:'', fulltext:'', screening:'', starred:false, sort:'newest' });
  let taskFilter = 'all';
  let datasetSearch = '';
  let drawer = null;
  let drawerTab = 'overview';
  let lastFocus = null;
  let modalLastFocus = null;
  let pendingImport = null;
  let searchTimer;

  function persist() {
    if (realMode) {
      if (typeof window.PLANT_SAVE_PAPERS !== 'function' || typeof window.PLANT_SAVE_DATASETS !== 'function') {
        persistenceError = true; toast('Go 数据库桥接不可用，本次修改尚未保存。', true); return false;
      }
      // One shared queue keeps papers before datasets so new references are valid.
      const paperSave = window.PLANT_SAVE_PAPERS(clone(state.papers));
      const datasetSave = window.PLANT_SAVE_DATASETS(clone(state.datasets));
      Promise.all([paperSave, datasetSave]).then(() => { persistenceError = false; }).catch(() => {
        persistenceError = true; toast('写入 SQLite 失败，请检查数据库后重试。', true);
      });
      return true;
    }
    try { localStorage.setItem(KEY, JSON.stringify(state)); persistenceError = false; return true; }
    catch (_) { persistenceError = true; toast('浏览器存储不可用，请导出 JSON 备份本次修改。', true); return false; }
  }
  function logActivity(message, kind = 'edit') {
    state.activities.unshift({ id: uid('act'), message, kind, at: new Date().toISOString() });
    state.activities = state.activities.slice(0, 100);
  }
  function toast(message, error = false) {
    const root = $('#toast-root'); if (!root) return;
    const el = document.createElement('div'); el.className = 'toast' + (error ? ' error' : '');
    el.setAttribute('role', error ? 'alert' : 'status');
    el.innerHTML = I(error ? 'alert' : 'check-circle', 18) + '<span>' + esc(message) + '</span>';
    root.appendChild(el); setTimeout(() => el.remove(), error ? 6500 : 3600);
  }
  function paperById(id) { return state.papers.find(p => p.id === id); }
  function paperDatasets(id) { return state.datasets.filter(d => (d.paperLinks || []).some(link => link.paperId === id)); }
  function paperResources(id) { return state.resources.filter(r => r.paperId === id); }
  function demoBadge(p) { return p.isDemo ? '<span class="demo-label">示例</span>' : ''; }
  function badge(label, color = 'gray') { return '<span class="badge ' + color + '">' + esc(label) + '</span>'; }
  function screenBadge(status) { return badge(SCREEN[status] || '待判断', status === 'included' ? 'green' : ['pending','uncertain'].includes(status) ? 'amber' : 'gray'); }
  function fullBadge(status) { return '<span class="status-dot ' + (status === 'missing' ? 'amber' : 'green') + '"></span><span>' + esc(FULL[status] || '待补入口') + '</span>'; }
  function empty(title, text, action = '') { return '<div class="empty-state"><div class="empty-icon">' + I('search', 28) + '</div><h3>' + esc(title) + '</h3><p>' + esc(text) + '</p>' + action + '</div>'; }
  function option(value, label, selected) { return '<option value="' + esc(value) + '"' + (String(value) === String(selected) ? ' selected' : '') + '>' + esc(label) + '</option>'; }
  function selectFilter(name, label, values) { return '<label class="filter-field"><span class="sr-only">' + label + '</span><select class="filter-select" data-filter="' + name + '" aria-label="' + label + '">' + option('', label, filters[name]) + values.map(v => option(typeof v === 'object' ? v.value : v, typeof v === 'object' ? v.label : v, filters[name])).join('') + '</select></label>'; }
  const arr = v => Array.isArray(v) ? v : [];
  let sources = arr(window.PLANT_SOURCES);
  const syncingSources = new Set();
  const speciesNames = p => arr(p.species).map(s => s && s.name ? s.name : '').filter(Boolean);
  const speciesDisplay = p => arr(p.species).map(s => s && (s.label || s.name) || '').filter(Boolean).join('、');
  const joinList = (v, sep = '、') => arr(v).filter(Boolean).join(sep);
  const listValues = {
    species: p => speciesNames(p),
    organ: p => arr(p.organs),
    modality: p => arr(p.modalities),
    platform: p => arr(p.platforms),
    year: p => p.year ? [String(p.year)] : []
  };
  function speciesLabelOf(name) { for (const p of state.papers) { for (const s of arr(p.species)) { if (s && s.name === name) return s.label || name; } } return name; }
  function unique(field) { const pick = listValues[field] || (() => []); return [...new Set(state.papers.flatMap(p => pick(p)).filter(Boolean))].sort((a, b) => String(a).localeCompare(String(b), 'zh-CN')); }
  const ARTICLE_TYPES = ['原创研究','方法研究','综述','相关资料'];
  const MODALITY_CHOICES = ['scRNA-seq','snRNA-seq','scATAC-seq','snATAC-seq','Multiome','sc-long-read','空间转录组'];
  const PLATFORM_CHOICES = ['10x Genomics Chromium','BD Rhapsody','Drop-seq','Microwell-seq','inDrop','Smart-seq2'];
  function checkboxGroup(name, values, selected, extraCustom = '') {
    const options = [...new Set([...values, ...arr(selected)])];
    return '<div class="checkbox-group" role="group">' + options.map(v => '<label class="checkbox-choice"><input type="checkbox" name="' + name + '" value="' + esc(v) + '"' + (arr(selected).includes(v) ? ' checked' : '') + '><span>' + esc(v) + '</span></label>').join('') + '</div>' + extraCustom;
  }
  const customField = (name, placeholder) => '<input class="input checkbox-custom" type="text" name="' + name + '" placeholder="' + esc(placeholder) + '" maxlength="500" aria-label="自定义补充">';
  const splitList = v => [...new Set(String(v || '').split(/[,，;；\n]/).map(s => s.trim()).filter(Boolean))];
  function normalizeDataset(d) {
    const list = (value, legacy) => {
      const values = Array.isArray(value) ? value : (legacy == null ? [] : [legacy]);
      return [...new Set(values.filter(v => typeof v === 'string').map(v => v.trim()).filter(Boolean))];
    };
    const legacySpecies = typeof d.species === 'string' ? d.species : '';
    d.species = Array.isArray(d.species) ? d.species.filter(s => s && typeof s.name === 'string' && s.name.trim()).map(s => ({name:s.name.trim(),label:String(s.label || '').trim()})) : (legacySpecies.trim() ? [{name:legacySpecies.trim(),label:String(d.speciesLabel || '').trim()}] : []);
    d.organs = list(d.organs, d.organ);
    d.modalities = list(d.modalities, d.modality);
    d.platforms = list(d.platforms, d.technology);
    d.repositories = arr(d.repositories).filter(r => r && typeof r === 'object').map(r => ({...r,repository:String(r.repository || '').trim().toLowerCase(),accession:String(r.accession || '').trim()}));
    if (!d.repositories.length && (d.repository || d.accession)) d.repositories = [{repository:String(d.repository || '').trim().toLowerCase(),accession:String(d.accession || '').trim()}];
    d.repository = d.repositories[0] ? d.repositories[0].repository : '';
    d.accession = d.repositories[0] ? d.repositories[0].accession : '';
    d.paperLinks = arr(d.paperLinks);
    return d;
  }
  state.datasets = state.datasets.map(normalizeDataset);
  function filteredPapers() {
    const q = filters.search.trim().toLocaleLowerCase();
    return state.papers.filter(p => {
      const hay = [p.title,p.authors,p.doi,p.pmid,arr(p.species).map(s => s ? ((s.name || '') + ' ' + (s.label || '')).trim() : '').join(' '),joinList(p.organs,' '),joinList(p.modalities,' '),joinList(p.platforms,' '),joinList(p.articleTypes,' '),p.abstract,(p.tags || []).join(' ')].join(' ').toLocaleLowerCase();
      return (!q || hay.includes(q)) && (!filters.species || listValues.species(p).includes(filters.species)) && (!filters.organ || listValues.organ(p).includes(filters.organ)) && (!filters.year || listValues.year(p).includes(filters.year)) && (!filters.modality || listValues.modality(p).includes(filters.modality)) && (!filters.platform || listValues.platform(p).includes(filters.platform)) && (!filters.fulltext || p.fulltext === filters.fulltext) && (!filters.screening || p.screening === filters.screening) && (!filters.starred || p.starred);
    }).sort((a,b) => filters.sort === 'year-desc' ? (Number(b.year) || 0) - (Number(a.year) || 0) : filters.sort === 'title' ? a.title.localeCompare(b.title, 'zh-CN') : String(b.addedAt || '').localeCompare(String(a.addedAt || '')));
  }
  function tasks() {
    const rows = [];
    state.papers.forEach(p => {
      if (['pending','uncertain'].includes(p.screening)) rows.push({key:p.id+'-screen', type:'screen', paper:p, title:p.screening==='uncertain'?'补充判断证据':'确认收录范围', subtitle:'核实研究对象与单细胞测序证据', color:'amber', icon:'check'});
      if (p.screening === 'included' && p.fulltext !== 'stored') rows.push({key:p.id+'-fulltext', type:'fulltext', paper:p, title:p.fulltext==='linked'?'获取全文正文':'补充全文入口', subtitle:p.fulltext==='linked'?'已有入口，正文文件仍待获取':'先记录可用入口，正文可稍后获取', color:'blue', icon:'file'});
      if (p.screening === 'included' && (!paperDatasets(p.id).length || paperDatasets(p.id).some(d => !d.accession || d.status !== 'verified'))) rows.push({key:p.id+'-data', type:'data', paper:p, title:'核对数据来源', subtitle:'补充 accession，区分新生成与复用', color:'purple', icon:'database'});
    });
    return rows;
  }
  function taskRows(items, compact = false) {
    return items.map(t => '<button class="task-item' + (compact ? ' compact' : '') + '" data-action="open-paper" data-id="' + esc(t.paper.id) + '"><span class="icon-wrap ' + t.color + '">' + I(t.icon, 18) + '</span><span class="task-content"><span class="task-title">' + esc(t.title) + '</span><span class="task-paper">' + esc(t.paper.title) + '</span>' + (!compact ? '<span class="task-subtitle muted">' + esc(t.subtitle) + '</span>' : '') + '</span>' + I('chevron-right', 16) + '</button>').join('');
  }
  function modalityBadges(p) {
    const list = arr(p.modalities);
    if (!list.length) return badge('待补');
    return '<div class="tech-badges">' + list.map(m => badge(m, m === 'snRNA-seq' || m === 'snATAC-seq' ? 'purple' : m === 'scATAC-seq' ? 'amber' : 'blue')).join('') + '</div>';
  }
  function paperTable(items, compact = false) {
    if (!items.length) return empty('没有找到匹配文献', '尝试调整筛选条件，或添加一篇新文献。', '<button class="btn btn-secondary" data-action="clear-filters">清空筛选</button>');
    return '<div class="table-wrap"><table class="paper-table' + (compact ? ' compact-table' : '') + '"><thead><tr><th class="col-star"><span class="sr-only">收藏</span></th><th>文献</th><th>研究对象</th><th>技术</th>' + (!compact ? '<th>全文</th>' : '') + '<th>收录状态</th><th class="col-arrow"></th></tr></thead><tbody>' + items.map(p => '<tr data-action="open-paper" data-id="' + esc(p.id) + '" tabindex="0" aria-label="查看 ' + esc(p.title) + '"><td><button class="btn-icon star-button' + (p.starred ? ' is-starred' : '') + '" data-action="toggle-star" data-id="' + esc(p.id) + '" aria-label="' + (p.starred ? '取消收藏' : '收藏') + '" aria-pressed="' + Boolean(p.starred) + '">' + I('star', 16) + '</button></td><td class="paper-main-cell"><div class="paper-title">' + esc(p.title) + '</div><div class="paper-meta">' + demoBadge(p) + '<span>' + esc(p.authors || '作者待补') + '</span><span class="meta-separator">·</span><span>' + esc(p.year || '年份待补') + '</span>' + (p.publicationStatus === 'preprint' ? '<span class="preprint-label">预印本</span>' : '') + '</div></td><td><div class="species-label">' + esc(speciesDisplay(p) || '物种待补') + '</div><div class="species-name">' + esc(joinList(p.organs) || '组织待补') + '</div></td><td>' + modalityBadges(p) + '</td>' + (!compact ? '<td><span class="fulltext-status">' + fullBadge(p.fulltext) + '</span></td>' : '') + '<td>' + screenBadge(p.screening) + '</td><td class="row-arrow">' + I('arrow-up-right', 16) + '</td></tr>').join('') + '</tbody></table></div>';
  }
  function progressRow(label, value, total, color = '') {
    const percentage = total ? Math.round(value/total*100) : 0;
    return '<div class="progress-row"><div class="progress-label"><span>' + label + '</span><span><strong>' + value + '</strong> / ' + total + '</span></div><div class="progress-track"><div class="progress-fill ' + color + '" style="width:' + percentage + '%"></div></div></div>';
  }
  function renderOverview() {
    const total = state.papers.length;
    const active = state.papers.filter(p => p.screening !== 'excluded');
    const missing = active.filter(p => p.screening==='included' && p.fulltext !== 'stored').length;
    const pending = state.papers.filter(p => ['pending','uncertain'].includes(p.screening)).length;
    const withData = state.papers.filter(p => paperDatasets(p.id).length).length;
    const withFulltext = state.papers.filter(p => p.fulltext !== 'missing').length;
    const recent = [...state.papers].sort((a,b) => String(b.addedAt).localeCompare(String(a.addedAt))).slice(0,5);
    const ts = tasks();
    const species = Object.entries(state.papers.reduce((obj,p) => { arr(p.species).forEach(s => { if (s && (s.label || s.name)) { const label = s.label || s.name; obj[label] = (obj[label] || 0)+1; } }); return obj; }, {})).sort((a,b)=>b[1]-a[1]).slice(0,5);
    const stats = [
      ['文献条目',total,'已纳入 '+state.papers.filter(p=>p.screening==='included').length+' 篇','book','papers'],
      ['关联数据集',state.datasets.length,'支持原始、复用与重新处理','database','datasets'],
      ['全文待补',missing,'已有入口，也可继续补正文','file','fulltext'],
      ['收录待判断',pending,'保留不确定项，减少遗漏','check','screen']
    ];
    return '<div class="overview-kicker"><span class="eyebrow">YOUR RESEARCH, IN ONE PLACE</span><span class="muted overview-date">'+I('calendar',15)+' '+new Date().toLocaleDateString('zh-CN',{month:'long',day:'numeric',weekday:'long'})+'</span></div><div class="stat-grid">' + stats.map((s,i)=>'<button class="stat-card" data-action="stat-nav" data-target="'+s[4]+'"><div class="stat-top"><span class="stat-label">'+s[0]+'</span><span class="stat-icon">'+I(s[3],18)+'</span></div><div class="stat-value">'+s[1]+'<span class="stat-unit">'+(i===1?'组':'篇')+'</span></div><div class="stat-meta">'+s[2]+I('arrow-right',14)+'</div></button>').join('')+'</div><div class="overview-grid"><section class="panel recent-panel"><div class="panel-header"><div><h2>最近收录</h2><p class="muted">新线索先留下，整理慢慢来</p></div><button class="text-link" data-page="papers">查看全部 '+I('arrow-right',16)+'</button></div>'+paperTable(recent,true)+'<div class="panel-footnote">'+I('info',14)+' 当前条目为原型示例，可通过新增或导入建立自己的目录。</div></section><section class="panel tasks-panel"><div class="panel-header"><div><h2>接下来整理</h2><p class="muted">按需要补齐信息</p></div><span class="count-pill">'+ts.length+'</span></div><div class="task-list">'+taskRows(ts.filter(t=>t.type==='screen').concat(ts.filter(t=>t.type==='data'),ts.filter(t=>t.type==='fulltext')).slice(0,4),true)+'</div><button class="panel-bottom-link" data-page="tasks">查看全部待补事项 '+I('arrow-right',16)+'</button></section></div><div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><div><h2>整理覆盖</h2><p class="muted">收录与完善，各自向前一步</p></div>'+I('grid',18)+'</div><div class="panel-body progress-list">'+progressRow('收录范围已判断',total-pending,total)+progressRow('已关联数据集',withData,total,'blue')+progressRow('已记录全文入口',withFulltext,total,'gold')+'</div></section><section class="panel"><div class="panel-header"><div><h2>研究物种</h2><p class="muted">当前目录中的文献分布</p></div><span class="muted">'+unique('species').length+' 个分类</span></div><div class="panel-body species-bars">'+species.map(([label,count])=>'<button class="species-bar-row" data-action="species-filter" data-label="'+esc(label)+'"><span>'+esc(label)+'</span><div class="species-bar-track"><div style="width:'+Math.round(count/Math.max(1,...species.map(s=>s[1]))*100)+'%"></div></div><strong>'+count+'</strong></button>').join('')+'</div></section></div>';
  }
  function renderPapers() {
    const items = filteredPapers();
    const filterCount = ['species','organ','year','modality','platform','fulltext','screening'].filter(k=>filters[k]).length + (filters.starred?1:0);
    return '<div class="section-toolbar"><div class="view-tabs"><button class="view-tab '+(!filters.starred?'active':'')+'" data-action="all-papers">全部文献 <span>'+state.papers.length+'</span></button><button class="view-tab '+(filters.starred?'active':'')+'" data-action="starred-papers">'+I('star',15)+' 我的收藏 <span>'+state.papers.filter(p=>p.starred).length+'</span></button></div><button class="btn btn-secondary" data-action="export-csv">'+I('download',16)+' 导出列表</button></div><section class="panel papers-panel"><div class="toolbar"><label class="table-search">'+I('search',18)+'<input id="paper-search" class="search-input" type="search" placeholder="搜索题目、物种、作者或 DOI…" value="'+esc(filters.search)+'" aria-label="搜索文献"></label><select class="filter-select" data-filter="sort" aria-label="排序方式">'+option('newest','最近收录',filters.sort)+option('year-desc','发表年份 ↓',filters.sort)+option('title','按标题排序',filters.sort)+'</select></div><div class="filter-row"><span class="filter-caption">'+I('filter',15)+' 筛选</span>'+selectFilter('species','全部物种',unique('species').map(v=>({value:v,label:speciesLabelOf(v)})))+selectFilter('organ','全部组织',unique('organ'))+selectFilter('year','全部年份',unique('year').reverse())+selectFilter('modality','全部技术',unique('modality'))+selectFilter('platform','全部平台',unique('platform'))+selectFilter('fulltext','全文情况',Object.entries(FULL).map(([value,label])=>({value,label})))+selectFilter('screening','收录状态',Object.entries(SCREEN).map(([value,label])=>({value,label})))+((filterCount||filters.search)?'<button class="text-link" data-action="clear-filters">清空'+(filterCount?' ('+filterCount+')':'')+'</button>':'')+'</div>'+paperTable(items)+'<div class="table-footer"><span>显示 '+items.length+' / '+state.papers.length+' 篇文献</span><span class="muted">点击文献查看详情与整理记录</span></div></section>';
  }
  function renderDatasets() {
    const q = datasetSearch.toLowerCase();
    const items = state.datasets.filter(d=>[d.title,d.id,speciesNames(d).join(' '),speciesDisplay(d),joinList(d.organs,' '),joinList(d.modalities,' '),joinList(d.platforms,' '),arr(d.repositories).map(r=>(r.repository || '')+' '+(r.accession || '')).join(' ')].join(' ').toLowerCase().includes(q));
    return '<div class="section-toolbar"><p class="muted">'+state.datasets.length+' 组数据 · 论文与数据集分别计数</p><button class="btn btn-secondary" data-action="new-dataset">'+I('plus',16)+' 添加数据集</button></div><div class="toolbar standalone-toolbar"><label class="table-search">'+I('search',18)+'<input id="dataset-search" class="search-input" type="search" placeholder="搜索物种、组织或 accession…" value="'+esc(datasetSearch)+'" aria-label="搜索数据集"></label></div>'+(items.length?'<div class="dataset-grid">'+items.map(d=>'<button class="dataset-card" data-action="open-dataset" data-id="'+esc(d.id)+'"><div class="dataset-top"><span class="icon-wrap green">'+I('database',20)+'</span>'+badge(d.status==='verified'?'已核实':d.status==='identified'?'线索已记录':'待整理',d.status==='verified'?'green':'gray')+'</div><h3>'+esc(d.title)+'</h3><p class="species-name">'+esc(speciesDisplay(d) || '物种待补')+'</p><div class="tag-list">'+badge(joinList(d.organs) || '组织待补')+badge(joinList(d.modalities) || '技术待补','blue')+demoBadge(d)+'</div><div class="dataset-accession"><span>'+esc(d.repository ? d.repository.toUpperCase() : '仓库待补')+'</span><code>'+esc(d.accession || '编号待补')+'</code></div><div class="dataset-footer"><span>'+I('book',15)+' '+(d.paperLinks || []).length+' 篇关联文献</span>'+I('arrow-up-right',17)+'</div></button>').join('')+'</div>':empty('暂无匹配数据集','从文献详情关联数据，或先创建一条数据集记录。'));
  }
  function renderTasks() {
    const all = tasks();
    const items = taskFilter==='all'?all:all.filter(t=>t.type===taskFilter);
    const tabs = [['all','全部待补'],['screen','收录待判断'],['fulltext','全文待补'],['data','数据待核实']];
    return '<div class="notice-box">'+I('info',18)+'<span>待补事项根据文献与数据状态自动生成。完成对应整理后，事项会自动移出列表。</span></div><div class="section-toolbar"><div class="view-tabs">'+tabs.map(([key,label])=>'<button class="view-tab '+(taskFilter===key?'active':'')+'" data-action="task-filter" data-filter-value="'+key+'">'+label+' <span>'+(key==='all'?all.length:all.filter(t=>t.type===key).length)+'</span></button>').join('')+'</div></div><section class="panel"><div class="task-list full-task-list">'+(items.length?taskRows(items):empty('这部分已整理完成','新的文献或数据线索进入后，需要补充的事项会出现在这里。'))+'</div></section>';
  }
  function renderSources() {
    const fallback = [
      {id:'pubmed',name:'PubMed',kind:'literature',description:'通过 NCBI E-utilities 检索论文与摘要。',homeUrl:'https://pubmed.ncbi.nlm.nih.gov/',supported:true,enabled:true,query:'',maxResults:20,recentRecords:[],recordCount:0},
      {id:'europe-pmc',name:'Europe PMC',kind:'literature',description:'检索文献、摘要与标识符。',homeUrl:'https://europepmc.org/',supported:true,enabled:true,query:'',maxResults:20,recentRecords:[],recordCount:0},
      {id:'crossref',name:'Crossref',kind:'literature',description:'检索 DOI 与出版信息。',homeUrl:'https://www.crossref.org/',supported:true,enabled:true,query:'',maxResults:20,recentRecords:[],recordCount:0},
      {id:'biorxiv',name:'bioRxiv',kind:'literature',description:'扫描最近 30 天的预印本并匹配关键词。',homeUrl:'https://www.biorxiv.org/',supported:true,enabled:true,query:'',maxResults:20,recentRecords:[],recordCount:0},
      {id:'sequence-repositories',name:'GEO / SRA / ENA',kind:'repository',description:'保存公共仓库 accession 线索。',homeUrl:'https://www.ebi.ac.uk/ena/browser/home',supported:true,enabled:true,query:'',maxResults:20,provider:'geo',recentRecords:[],recordCount:0},
      {id:'cncb-gsa',name:'CNCB GSA',kind:'repository',description:'站点需要浏览器安全检查，当前仅保留手动入口。',homeUrl:'https://ngdc.cncb.ac.cn/gsa/',supported:false,enabled:false,query:'',maxResults:20,recentRecords:[],recordCount:0}
    ];
    const items=sources.length?sources:fallback;
    const notice=realMode?'公开 API 仅在点击“同步”时访问，不会后台定时拉取。新文献进入“待判断”，重复 DOI / PMID 保留原记录；仓库结果只保存为 accession 线索。':'来源同步只写入真实 SQLite 数据库。请先在设置中关闭 Demo 演示模式，再配置和运行同步。';
    const cards=items.map(source=>{
      const running=syncingSources.has(source.id);
      const failed=source.lastRun&&source.lastRun.status==='failed';
      const stateLabel=!source.supported?'手动入口':running?'同步中':!source.enabled?'已停用':failed?'上次失败':'已配置';
      const stateColor=failed?'amber':source.supported&&source.enabled?'green':'gray';
      const last=source.lastSuccessAt?'最近成功：'+new Date(source.lastSuccessAt).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'最近同步：尚未成功';
      const query=source.id==='sequence-repositories'?String(source.provider||'geo').toUpperCase()+' · '+source.query:source.query;
      return '<section class="source-card"><div class="source-heading"><span class="source-letter">'+esc(source.name.slice(0,1))+'</span><div><h3>'+esc(source.name)+'</h3><p class="muted">'+(source.kind==='repository'?'数据仓库':'文献元数据')+'</p></div>'+badge(stateLabel,stateColor)+'</div><p class="source-description">'+esc(source.description)+'</p>'+(source.supported?'<div class="source-query" title="'+esc(query)+'"><code>'+esc(query||'查询待配置')+'</code></div>':'')+(failed?'<p class="source-error" role="alert">'+esc(source.lastRun.errorMessage||'同步失败，请检查配置和网络。')+'</p>':'')+'<div class="source-footer"><span class="muted">'+esc(last)+'</span><a href="'+esc(safeURL(source.homeUrl))+'" target="_blank" rel="noopener noreferrer" class="text-link">官方入口 '+I('arrow-up-right',14)+'</a></div><div class="source-actions"><button class="text-link" data-action="source-records" data-id="'+esc(source.id)+'">记录 '+Number(source.recordCount||0)+'</button>'+(source.supported?'<button class="btn btn-secondary" data-action="configure-source" data-id="'+esc(source.id)+'"'+(!realMode||running?' disabled':'')+'>配置</button><button class="btn btn-primary" data-action="sync-source" data-id="'+esc(source.id)+'"'+(!realMode||!source.enabled||running?' disabled':'')+'>'+I(running?'clock':'refresh',15)+(running?' 正在同步':' 同步')+'</button>':'')+'</div></section>';
    }).join('');
    return '<div class="notice-box">'+I('info',18)+'<span>'+notice+'</span></div><div class="source-grid">'+cards+'</div><section class="panel activity-panel"><div class="panel-header"><div><h2>整理活动</h2><p class="muted">记录当前会话中的整理与同步操作</p></div><button class="text-link" data-action="export-json">'+I('download',15)+' 导出备份</button></div>'+(state.activities.length?'<div class="activity-list">'+state.activities.slice(0,20).map(a=>'<div class="activity-item"><span class="activity-dot"></span><div><p>'+esc(a.message)+'</p><span class="muted">'+dateText(a.at)+' · '+new Date(a.at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})+'</span></div></div>').join('')+'</div>':empty('暂无整理活动','添加文献、保存笔记、导入数据或同步来源后，可以在这里查看记录。'))+'</section>';
  }
  function sourceConfigModal(id) {
    const source=sources.find(item=>item.id===id);if(!source||!source.supported)return;
    const hints={pubmed:'使用 PubMed 检索语法，例如 ("single cell"[Title/Abstract]) AND Arabidopsis[Title/Abstract]。','europe-pmc':'使用 Europe PMC 检索语法，例如 TITLE_ABS:"single cell" AND TITLE_ABS:plant。',crossref:'输入书目关键词；Crossref 按相关性返回 DOI 和出版信息。',biorxiv:'输入空格分隔的普通关键词，所有词都需匹配题目、摘要或分类；扫描最近 30 天，最多检查 10 页。','sequence-repositories':'GEO / SRA 使用 NCBI 检索语法；ENA 使用 Portal API 语法，例如 description="*single*" AND tax_tree(33090)。'};
    const provider=source.id==='sequence-repositories'?'<label class="form-field full-width"><span>检索仓库</span><select class="input" name="provider">'+option('geo','GEO（NCBI）',source.provider)+option('sra','SRA（NCBI）',source.provider)+option('ena','ENA（European Nucleotide Archive）',source.provider)+'</select></label>':'';
    const body='<form id="source-config-form" data-initial-query="'+esc(source.query)+'"><input type="hidden" name="id" value="'+esc(id)+'"><div class="settings-mode-row source-enable-row"><div><label for="source-enabled">允许手动同步</label><p>只有点击同步按钮才会发起网络请求。</p></div><input class="settings-switch" id="source-enabled" name="enabled" type="checkbox" role="switch" '+(source.enabled?'checked':'')+'></div><div class="form-grid">'+provider+'<label class="form-field full-width"><span>检索式 / 关键词 <b class="required">*</b></span><textarea class="textarea" name="query" rows="5" maxlength="4000" required>'+esc(source.query)+'</textarea><small class="form-hint">'+esc(hints[id]||'')+'</small></label><label class="form-field"><span>单次最多保存</span><input class="input" name="maxResults" type="number" min="1" max="100" step="1" value="'+esc(source.maxResults||20)+'" required></label></div><p id="form-error" class="form-error" role="alert"></p></form>';
    showModal('配置 '+source.name,body,'<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="source-config-form">'+I('check',16)+' 保存配置</button>',true);
  }
  function saveSourceConfig(form) {
    if(typeof window.PLANT_SAVE_SOURCE_CONFIG!=='function')return;
    const data=new FormData(form);const config={id:String(data.get('id')||''),enabled:data.get('enabled')==='on',query:String(data.get('query')||'').trim(),maxResults:Number(data.get('maxResults')),provider:String(data.get('provider')||'')};
    if(!config.query){$('#form-error').textContent='请填写检索式或关键词。';return;}
    if(!Number.isInteger(config.maxResults)||config.maxResults<1||config.maxResults>100){$('#form-error').textContent='单次结果数量需为 1–100。';return;}
    const button=$('button[form="source-config-form"]');if(button)button.disabled=true;
    window.PLANT_SAVE_SOURCE_CONFIG(config).then(status=>{sources=sources.map(item=>item.id===status.id?status:item);closeModal();render();toast(status.name+' 配置已保存。');}).catch(error=>{const message=error&&error.message?error.message:'保存失败，请重试。';$('#form-error').textContent=message;if(button)button.disabled=false;});
  }
  function sourceRecordsModal(id) {
    const source=sources.find(item=>item.id===id);
    if(!source){showModal('外部记录',empty('桌面应用中查看','浏览器预览不连接正式 SQLite；打开桌面应用并同步后，可在这里查看外部标识和官方链接。'),'<button class="btn btn-secondary" data-action="close-modal">关闭</button>');return;}
    const records=arr(source.recentRecords);
    const body=records.length?'<p class="muted">显示最近见到的 '+records.length+' 条；所有来源快照均保留在 SQLite。</p><div class="source-record-list">'+records.map(record=>'<div class="source-record-item"><div><strong>'+esc(record.title||record.externalId)+'</strong><code>'+esc(record.externalId)+'</code><span>'+esc(new Date(record.lastSeenAt).toLocaleString('zh-CN'))+'</span></div>'+(safeURL(record.url)?'<a class="btn-icon" href="'+esc(safeURL(record.url))+'" target="_blank" rel="noopener noreferrer" aria-label="打开外部记录">'+I('arrow-up-right',18)+'</a>':'')+'</div>').join('')+'</div>':empty('暂无外部记录','保存配置并点击同步后，外部标识和官方链接会显示在这里。');
    showModal(source.name+' · 外部记录',body,'<button class="btn btn-secondary" data-action="close-modal">关闭</button>',true);
  }
  async function syncSource(id) {
    if(syncingSources.has(id)||typeof window.PLANT_SYNC_SOURCE!=='function')return;
    syncingSources.add(id);render();
    try {
      const result=await window.PLANT_SYNC_SOURCE(id);
      sources=sources.map(item=>item.id===id?result.status:item);
      state.papers=arr(result.papers);
      const source=sources.find(item=>item.id===id);
      const message=(source?source.name:id)+' 同步完成：'+result.recordsSeen+' 条外部记录，新增 '+result.papersAdded+' 篇文献，'+result.duplicates+' 条已存在。';
      logActivity(message,'sync');toast(message);
    } catch(error) {
      toast('同步失败：'+(error&&error.message?error.message:'请检查网络和来源配置。'),true);
      if(typeof window.PLANT_LIST_SOURCES==='function'){try{sources=await window.PLANT_LIST_SOURCES();}catch(_) {}}
    } finally {syncingSources.delete(id);render();}
  }
  function renderSchema() {
    const entities = [
      ['01','文献','papers','题目、摘要、发表信息、收录与阅读状态。全文缺失也可以先收录。','book'],
      ['02','数据集','datasets','独立内部 ID，关联多个仓库编号，并记录数据可用性。','database'],
      ['03','资料与附件','resources','正文、补充材料、数据入口、矩阵和代码，保存来源与获取状态。','folder']
    ];
    return '<div class="section-toolbar"><p class="muted">面向个人使用，逐步补齐信息</p><div class="page-actions"><a class="btn btn-secondary" href="./downloads/plantcell-library-v0.1.zip" download>下载完整文件包</a><a class="btn btn-secondary" href="./downloads/README.md" download>'+I('file',16)+' 设计说明</a><a class="btn btn-primary" href="./downloads/schema.sql" download>'+I('download',16)+' 下载建表 SQL</a></div></div><div class="schema-grid">'+entities.map(e=>'<section class="schema-card"><span class="schema-number">'+e[0]+'</span><span class="icon-wrap green">'+I(e[4],22)+'</span><h2>'+e[1]+'</h2><code>'+e[2]+'</code><p>'+e[3]+'</p></section>').join('')+'</div><section class="panel"><div class="panel-header"><div><h2>关系如何连接</h2><p class="muted">关系单独保存，避免重复统计</p></div>'+I('link',20)+'</div><div class="panel-body relation-list"><div class="relation-note"><strong>文献 ↔ 数据集</strong><span>多对多，标记新生成、复用或重新处理；一篇文献可以关联多组数据。</span><code>paper_datasets</code></div><div class="relation-note"><strong>文献 / 数据集 ↔ 资料</strong><span>同一份补充材料可以服务多个对象，链接与实际获取文件分别记录。</span><code>resources + 关联表</code></div><div class="relation-note"><strong>内部对象 ↔ 外部标识</strong><span>DOI、PMID 和仓库 accession 用于识别与关联；保留原始来源信息。</span><code>identifiers / accession</code></div></div></section><div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><h2>状态分别记录</h2></div><div class="panel-body status-definitions"><p><strong>收录：</strong>待判断 · 证据不足 · 已纳入 · 相关 / 排除</p><p><strong>全文：</strong>待补入口 · 已有入口 · 已保存文件</p><p><strong>数据：</strong>待整理 · 线索已记录 · 已核实</p><p><strong>阅读：</strong>未读 · 已浏览 · 已精读</p></div></section><section class="panel"><div class="panel-header"><h2>下一步可逐步扩展</h2></div><div class="panel-body"><p class="muted">在相同数据集下补充生物样本、文库、测序 Run、参考基因组与细胞注释。每日采集使用独立日志与断点，人工笔记保留。</p><p class="muted">SQL 文件是后端数据库结构；当前网页原型在本浏览器保存操作，可通过 JSON 备份迁移。</p></div></section></div>';
  }
  function renderSettings() {
    const preferences = window.PLANT_SETTINGS || {databasePath:'',fontSize:15,mcpToken:''};
    const mcp = window.PLANT_MCP_INFO || {enabled:false,endpoint:'',token:'',tools:[],resources:[]};
    const desktop = window.PLANT_DESKTOP === true;
    const canSave = desktop && typeof window.PLANT_SAVE_SETTINGS === 'function';
    const currentPath = (window.PLANT_STATUS || {}).databasePath || preferences.databasePath || '桌面应用启动后显示';
    const disabled = canSave ? '' : 'disabled';
    const pendingPath = preferences.databasePath && preferences.databasePath !== currentPath;
    const pendingToken = preferences.mcpToken && preferences.mcpToken !== mcp.token;
    return '<div class="settings-layout"><form class="panel settings-panel" id="settings-form">'+
      '<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">'+I('leaf',21)+'</span><div><h2>数据模式</h2><p>演示与正式研究记录分别保存。</p></div></div>'+
      '<div class="settings-mode-row"><div><label for="demo-mode">Demo 演示模式</label><p>浏览虚构示例；关闭后使用真实 SQLite 数据库。</p></div><input class="settings-switch" id="demo-mode" type="checkbox" role="switch" '+(!realMode?'checked':'')+(desktop?'':' disabled')+' aria-label="Demo 演示模式"></div>'+
      '<div class="settings-inline-note">'+I('info',16)+'<span>切换后立即重新加载，不会复制、合并或清空任何一边的数据。MCP 始终连接正式数据库。</span></div></section>'+
      '<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">'+I('database',21)+'</span><div><h2>数据库存放位置</h2><p>选择正式文献库的 SQLite 文件。</p></div></div>'+
      '<div class="settings-field"><label for="settings-database-path">数据库文件路径</label><div class="settings-path-row"><input class="settings-path-input" id="settings-database-path" name="databasePath" type="text" value="'+esc(preferences.databasePath)+'" placeholder="在桌面应用中选择数据库位置" autocomplete="off" spellcheck="false" required '+disabled+'><button class="btn btn-secondary" type="button" data-action="choose-database" '+disabled+'>'+I('folder',16)+' 选择文件夹</button></div></div>'+
      '<div class="settings-active-path"><span>本次运行正在使用</span><code>'+esc(currentPath)+'</code></div>'+
      '<div class="settings-inline-note">'+I('info',16)+'<span>新路径在重启后生效。应用不会移动或覆盖旧数据库；如果新文件不存在，重启时会创建一个空数据库。</span></div></section>'+
      '<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">'+I('code',21)+'</span><div><h2>外部 AI · MCP</h2><p>允许本机 AI 查询正式库；认证后可导入和编辑。</p></div>'+badge(mcp.enabled?'已启用':'未启动',mcp.enabled?'green':'gray')+'</div>'+
      '<div class="settings-mcp-meta"><div><span>Streamable HTTP 地址</span><code>'+esc(mcp.enabled?mcp.endpoint:'启动桌面应用后监听 127.0.0.1:37841')+'</code></div><p><strong>工具</strong>'+esc((mcp.tools||[]).join(' · '))+'</p><p><strong>Schema 资源</strong>'+esc((mcp.resources||[]).join(' · '))+'</p></div>'+
      '<div class="settings-field"><label for="settings-mcp-token">Bearer Token</label><div class="settings-token-row"><input class="settings-path-input" id="settings-mcp-token" name="mcpToken" type="password" value="'+esc(preferences.mcpToken)+'" minlength="32" maxlength="256" autocomplete="off" spellcheck="false" required '+disabled+'><button class="btn btn-secondary" type="button" data-action="toggle-mcp-token" '+disabled+'>显示</button><button class="btn btn-secondary" type="button" data-action="copy-mcp-token" '+disabled+'>复制</button><button class="btn btn-secondary" type="button" data-action="regenerate-mcp-token" '+disabled+'>重新生成</button></div><p class="form-hint">32–256 个可见 ASCII 字符，不能包含空格。重新生成后，旧 Token 将在重启时失效。</p></div>'+
      '<div class="settings-inline-note">'+I('info',16)+'<span>无认证连接只读；写入使用 Authorization: Bearer &lt;TOKEN&gt;。Token 修改在重启后生效，且不会通过 MCP 返回。服务仅监听本机回环地址，不要转发到局域网或公网。</span></div></section>'+
      '<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">'+I('book',21)+'</span><div><h2>阅读样式</h2><p>只调整文字大小，不改变面板宽度与整体布局。</p></div></div>'+
      '<div class="settings-field"><div class="settings-label-row"><label for="settings-font-size">界面字号</label><output id="settings-font-size-value" for="settings-font-size">'+esc(preferences.fontSize)+' px</output></div><div class="settings-font-control"><span>小</span><input id="settings-font-size" name="fontSize" type="range" min="12" max="20" step="1" value="'+esc(preferences.fontSize)+'" '+disabled+'><span>大</span><button class="text-link" type="button" data-action="reset-font-size" '+disabled+'>恢复 15 px</button></div></div>'+
      '<div class="settings-font-preview"><span>阅读预览</span><p>从一片叶、一个细胞，到一篇可追溯的研究。</p><small>Plant single-cell research · 文献、数据与笔记</small></div></section>'+
      '<footer class="settings-footer"><p id="settings-save-status" role="status">'+(canSave?(pendingToken?'MCP Token 已更改，将在下次重启后使用。':(pendingPath?'数据库路径已更改，将在下次重启后使用。':'设置保存在本机用户配置目录。')):'浏览器预览仅用于查看；请在 mygo 桌面应用中修改设置。')+'</p><button class="btn btn-primary" type="submit" '+disabled+'>'+I('check',16)+' 保存设置</button></footer></form>'+
      '<aside class="settings-aside"><section class="panel settings-aside-card"><span class="eyebrow">YOUR WORKSPACE</span><h2>安静、清晰，<br>适合自己的阅读节奏。</h2><p>字号会同时应用到文献表格、详情抽屉、导航与笔记，并在下次启动时保留。</p><div class="settings-aside-rule"></div><dl><div><dt>当前模式</dt><dd>'+(!realMode?'Demo':'真实数据库')+'</dd></div><div><dt>正式数据</dt><dd>本地 SQLite</dd></div><div><dt>配置方式</dt><dd>自动持久化</dd></div></dl></section><div class="settings-restart-note">'+I('clock',17)+'<p><strong>数据库与 MCP 变更需要重启</strong><br>字体大小无需重启，保存前即可预览。</p></div></aside></div>';
  }
  function saveSettings(form) {
    if(typeof window.PLANT_SAVE_SETTINGS!=='function')return;
    const data=new FormData(form);const button=$('button[type=submit]',form);const status=$('#settings-save-status');
    const next={databasePath:String(data.get('databasePath')||'').trim(),fontSize:Number(data.get('fontSize')),mcpToken:String(data.get('mcpToken')||'').trim()};
    if(!next.databasePath){status.textContent='请填写数据库文件的绝对路径。';return;}
    if(!/^[\x21-\x7e]{32,256}$/.test(next.mcpToken)){status.textContent='MCP Token 需为 32–256 个可见 ASCII 字符，且不能包含空格。';return;}
    button.disabled=true;status.textContent='正在保存设置…';
    window.PLANT_SAVE_SETTINGS(next).then(result=>{
      window.PLANT_SETTINGS=result.settings;
      if(typeof window.PLANT_APPLY_FONT_SIZE==='function')window.PLANT_APPLY_FONT_SIZE(result.settings.fontSize);
      $('#settings-database-path').value=result.settings.databasePath;
      $('#settings-mcp-token').value=result.settings.mcpToken;
      const restartChanges=[];if(result.databaseChanged)restartChanges.push('数据库位置');if(result.mcpTokenChanged)restartChanges.push('MCP Token');
      status.textContent=restartChanges.length?'设置已保存。'+restartChanges.join('和')+'将在重启后生效。':'设置已保存，字号已应用。';
      toast(restartChanges.length?'设置已保存，'+restartChanges.join('和')+'将在重启后生效。':'设置已保存。');
    }).catch(error=>{status.textContent='保存失败：'+(error&&error.message?error.message:'请检查路径和权限后重试。');}).finally(()=>{button.disabled=false;});
  }
  function render() {
    if (!$('#app-content')) return;
    const renderer = {overview:renderOverview,papers:renderPapers,datasets:renderDatasets,tasks:renderTasks,sources:renderSources,schema:renderSchema,settings:renderSettings};
    $('#app-content').innerHTML = renderer[page]();
    if ($('.heading-actions')) $('.heading-actions').hidden = page === 'settings';
    if ($('#page-title')) $('#page-title').textContent = pageMeta[page][0];
    if ($('#page-subtitle')) $('#page-subtitle').textContent = pageMeta[page][1];
    if ($('#breadcrumb-current')) $('#breadcrumb-current').textContent = pageMeta[page][0];
    $$('[data-page]').forEach(el => { const active = el.dataset.page === page; el.classList.toggle('active', active); if(active) el.setAttribute('aria-current','page'); else el.removeAttribute('aria-current'); });
    if ($('#count-papers')) $('#count-papers').textContent = state.papers.length;
    if ($('#count-datasets')) $('#count-datasets').textContent = state.datasets.length;
    if ($('#count-tasks')) $('#count-tasks').textContent = tasks().length;
  }
  function navigate(next) {
    if (!pageMeta[next]) return;
    page = next; if(location.hash !== '#'+next) history.replaceState(null,'','#'+next);
    render(); document.body.classList.remove('sidebar-open','nav-open');
    if ($('#main-content')) $('#main-content').scrollTop = 0;
  }
  function closeDrawer() {
    drawer = null; if ($('#drawer-root')) $('#drawer-root').innerHTML = '';
    document.body.classList.remove('drawer-open');
    if (lastFocus && document.contains(lastFocus)) lastFocus.focus();
  }
  function closeModal() {
    if ($('#modal-root')) $('#modal-root').innerHTML = '';
    pendingImport = null; document.body.classList.remove('modal-open');
    if (modalLastFocus && document.contains(modalLastFocus)) modalLastFocus.focus();
  }
  function showModal(title, body, footer, wide = false) {
    modalLastFocus = document.activeElement;
    $('#modal-root').innerHTML = '<div class="modal-backdrop" data-action="close-modal"></div><section class="modal'+(wide?' modal-wide':'')+'" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-header"><h2 id="modal-title">'+esc(title)+'</h2><button class="btn-icon" data-action="close-modal" aria-label="关闭弹窗">'+I('x',20)+'</button></div><div class="modal-body">'+body+'</div><div class="modal-footer">'+footer+'</div></section>';
    document.body.classList.add('modal-open');
    requestAnimationFrame(()=> { const target = $('#modal-root .modal input:not([type=hidden])') || $('#modal-root .modal textarea') || $('#modal-root .modal select') || $('#modal-root .modal button'); if (target) target.focus(); });
  }
  function field(label, name, value = '', placeholder = '', required = false, type = 'text', span = false) {
    return '<label class="form-field'+(span?' full-width':'')+'"><span>'+label+(required?' <b class="required">*</b>':'')+'</span><input class="input" name="'+name+'" type="'+type+'" value="'+esc(value)+'" placeholder="'+esc(placeholder)+'"'+(required?' required':'')+' maxlength="'+(name==='title'?2000:500)+'"></label>';
  }
  function openPaper(id) {
    if (!paperById(id)) return;
    lastFocus = document.activeElement; drawer = {type:'paper', id}; drawerTab='overview'; renderDrawer();
  }
  function openDataset(id) {
    if (!state.datasets.find(d=>d.id===id)) return;
    lastFocus = document.activeElement; drawer = {type:'dataset', id}; drawerTab='overview'; renderDrawer();
  }
  function resourceRows(resources) {
    if (!resources.length) return '<div class="inline-empty">'+I('folder',24)+'<p>还没有记录资料</p><span class="muted">可以先保存全文、补充材料或数据页面的链接。</span></div>';
    const types = {fulltext:'全文入口',supplement:'补充材料',matrix:'表达矩阵',annotation:'细胞注释',code:'分析代码',data:'数据入口',other:'其他资料'};
    return resources.map(r=>'<div class="resource-item"><span class="icon-wrap blue">'+I(r.type==='fulltext'?'file':'link',18)+'</span><div class="resource-content"><strong>'+esc(r.title || types[r.type] || '资料链接')+'</strong><span class="muted">'+esc(types[r.type] || '其他资料')+' · 已记录链接'+(r.isDemo?' · 示例跳转至官方主页':'')+'</span></div>'+(safeURL(r.url)?'<a class="btn-icon" href="'+esc(safeURL(r.url))+'" target="_blank" rel="noopener noreferrer" aria-label="打开 '+esc(r.title)+'">'+I('arrow-up-right',18)+'</a>':'<span class="muted">链接待补</span>')+'</div>').join('');
  }
  function renderDrawer() {
    if (!drawer) return;
    const root = $('#drawer-root'); if(!root) return;
    if (drawer.type === 'paper') {
      const p = paperById(drawer.id); if(!p) return closeDrawer();
      const ds = paperDatasets(p.id); const resources = paperResources(p.id);
      let body = '';
      if(drawerTab==='overview') body = '<div class="drawer-status-row">'+screenBadge(p.screening)+badge(READING[p.reading] || '未读')+demoBadge(p)+'<button class="btn btn-secondary" data-action="edit-paper" data-id="'+esc(p.id)+'">编辑信息</button></div><div class="detail-grid"><div class="detail-item"><span>研究物种</span><strong>'+esc(speciesDisplay(p) || '待补')+'</strong><small>'+esc(joinList(speciesNames(p)) || '')+'</small></div><div class="detail-item"><span>组织 / 部位</span><strong>'+esc(joinList(p.organs) || '待补')+'</strong></div><div class="detail-item"><span>测序类型</span><strong>'+esc(joinList(p.modalities) || '待补')+'</strong></div><div class="detail-item"><span>测序平台</span><strong>'+esc(joinList(p.platforms) || '待补')+'</strong></div><div class="detail-item"><span>发表年份</span><strong>'+esc(p.year || '待补')+'</strong></div><div class="detail-item"><span>文献类型</span><strong>'+esc(joinList(p.articleTypes) || '待判断')+'</strong></div><div class="detail-item"><span>发表状态</span><strong>'+(p.publicationStatus==='preprint'?'预印本':p.publicationStatus==='published'?'正式论文':p.publicationStatus==='unpublished'?'未发表':'尚未确认')+'</strong></div></div><section class="detail-section"><h3>摘要</h3><p class="abstract-text">'+esc(p.abstract || '摘要尚未补充。先保留这条文献，之后可以继续完善。')+'</p></section><section class="detail-section"><h3>主题标签</h3><div class="tag-list">'+((p.tags || []).length?(p.tags || []).map(t=>'<span class="tag">'+esc(t)+'</span>').join(''):'<span class="muted">暂未设置标签</span>')+'</div></section><section class="detail-section"><h3>来源与标识</h3><dl class="detail-dl"><dt>DOI</dt><dd>'+(p.doi?'<a class="text-link" href="https://doi.org/'+esc(encodeURI(p.doi))+'" target="_blank" rel="noopener noreferrer">'+esc(p.doi)+I('arrow-up-right',13)+'</a>':'尚未记录')+'</dd><dt>PMID</dt><dd>'+esc(p.pmid || '尚未记录')+'</dd><dt>发现来源</dt><dd>'+esc(p.source || '手动添加')+'</dd><dt>加入时间</dt><dd>'+dateText(p.addedAt)+'</dd></dl></section><section class="detail-section"><h3>收录判断</h3><label class="form-field"><span class="sr-only">收录判断</span><select class="input" data-paper-setting="screening" data-id="'+esc(p.id)+'">'+Object.entries(SCREEN).map(([v,l])=>option(v,l,p.screening)).join('')+'</select></label><p class="muted small">'+esc(p.screeningReason || '可在笔记中补充判断依据。')+'</p></section>';
      if(drawerTab==='resources') body = '<section class="detail-section"><div class="section-title"><h3>关联数据集 <span class="count-pill">'+ds.length+'</span></h3><button class="text-link" data-action="link-dataset" data-id="'+esc(p.id)+'">'+I('plus',15)+' 关联数据</button></div>'+ (ds.length?ds.map(d=>'<button class="related-dataset" data-action="open-dataset" data-id="'+esc(d.id)+'"><span class="icon-wrap green">'+I('database',18)+'</span><span><strong>'+esc(d.title)+'</strong><small>'+esc(d.accession || '仓库编号待补')+'</small></span>'+badge(ROLE[(d.paperLinks.find(l=>l.paperId===p.id)||{}).role] || '关联','gray')+I('chevron-right',15)+'</button>').join(''):'<p class="muted">尚未关联数据。已有公共数据可以复用关联，无需重复创建。</p>')+'</section><section class="detail-section"><div class="section-title"><h3>资料与附件 <span class="count-pill">'+resources.length+'</span></h3><button class="text-link" data-action="add-resource" data-id="'+esc(p.id)+'" data-kind="paper">'+I('plus',15)+' 添加链接</button></div>'+resourceRows(resources)+'</section><div class="notice-box compact">'+I('info',16)+'<span>保存入口只表示链接已记录，不代表全文或数据文件已经获取。</span></div>';
      if(drawerTab==='notes') body = '<section class="detail-section"><h3>研究笔记</h3><p class="muted">记录关注的问题、判断依据和下一步计划。</p><label class="form-field"><span class="sr-only">研究笔记</span><textarea class="textarea notes-textarea" id="paper-notes" rows="15" maxlength="50000" placeholder="这篇文献值得关注什么？还有哪些信息需要补齐？">'+esc(p.notes || '')+'</textarea></label><button class="btn btn-primary" data-action="save-notes" data-id="'+esc(p.id)+'">'+I('check',16)+' 保存笔记</button></section><section class="detail-section"><h3>主题标签</h3><label class="form-field"><span class="muted">用逗号分隔，例如：根发育，跨物种，重点阅读</span><input class="input" id="paper-tags" value="'+esc((p.tags || []).join('，'))+'" maxlength="1000"></label><button class="btn btn-secondary" data-action="save-tags" data-id="'+esc(p.id)+'">保存标签</button></section>';
      root.innerHTML='<div class="drawer-backdrop" data-action="close-drawer"></div><aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title"><div class="drawer-header"><div class="drawer-eyebrow"><span>'+esc(p.id)+'</span><button class="btn-icon" data-action="close-drawer" aria-label="关闭详情">'+I('x',20)+'</button></div><h2 id="drawer-title">'+esc(p.title)+'</h2><p class="paper-meta">'+esc(p.authors || '作者待补')+' · '+esc(p.journal || '期刊待补')+'</p></div><div class="drawer-tabs">'+[['overview','文献概览'],['resources','数据与资料'],['notes','我的笔记']].map(([v,l])=>'<button class="drawer-tab '+(drawerTab===v?'active':'')+'" data-action="drawer-tab" data-tab="'+v+'">'+l+'</button>').join('')+'</div><div class="drawer-body">'+body+'</div><div class="drawer-footer"><label class="reading-select"><span>阅读进度</span><select class="input" data-paper-setting="reading" data-id="'+esc(p.id)+'">'+Object.entries(READING).map(([v,l])=>option(v,l,p.reading)).join('')+'</select></label><button class="btn '+(p.starred?'btn-primary':'btn-secondary')+'" data-action="toggle-star" data-id="'+esc(p.id)+'">'+I('star',16)+(p.starred?' 已收藏':' 收藏')+'</button></div></aside>';
    } else {
      const d = state.datasets.find(x=>x.id===drawer.id); if(!d) return closeDrawer();
      const available = {unknown:'尚未核实',public:'公开可访问',unavailable:'暂不可访问',embargoed:'尚未解禁',restricted:'需要申请'};
      root.innerHTML = '<div class="drawer-backdrop" data-action="close-drawer"></div><aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title"><div class="drawer-header"><div class="drawer-eyebrow"><span>'+esc(d.id)+'</span><button class="btn-icon" data-action="close-drawer" aria-label="关闭详情">'+I('x',20)+'</button></div><h2 id="drawer-title">'+esc(d.title)+'</h2><p class="species-name">'+esc(speciesDisplay(d))+'</p></div><div class="drawer-body"><div class="drawer-status-row">'+badge(d.status==='verified'?'已核实':d.status==='identified'?'线索已记录':'待整理',d.status==='verified'?'green':'gray')+demoBadge(d)+'</div><div class="detail-grid"><div class="detail-item"><span>仓库</span><strong>'+esc(d.repository || '待补')+'</strong></div><div class="detail-item"><span>Accession</span><strong>'+esc(d.accession || '编号待补')+'</strong></div><div class="detail-item"><span>研究组织</span><strong>'+esc(joinList(d.organs) || '待补')+'</strong></div><div class="detail-item"><span>测序类型</span><strong>'+esc(joinList(d.modalities) || '待补')+'</strong></div><div class="detail-item"><span>测序平台</span><strong>'+esc(joinList(d.platforms) || '待补')+'</strong></div><div class="detail-item"><span>细胞数</span><strong>'+(d.cellsReported==null?'尚未整理':esc(d.cellsReported))+'</strong></div><div class="detail-item"><span>原始数据</span><strong>'+esc(available[d.availability] || '尚未核实')+'</strong></div></div><section class="detail-section"><h3>关联文献</h3>'+((d.paperLinks || []).length?(d.paperLinks || []).map(l=>{ const p=paperById(l.paperId); return p?'<button class="related-dataset" data-action="open-paper" data-id="'+esc(p.id)+'"><span class="icon-wrap blue">'+I('book',18)+'</span><span><strong>'+esc(p.title)+'</strong><small>'+esc(p.year || '年份待补')+'</small></span>'+badge(ROLE[l.role] || '关联')+'</button>':''; }).join(''):'<p class="muted">尚未关联文献。</p>')+'</section><section class="detail-section"><div class="section-title"><h3>数据与资料入口</h3><button class="text-link" data-action="add-resource" data-id="'+esc(d.id)+'" data-kind="dataset">'+I('plus',15)+' 添加链接</button></div>'+resourceRows(state.resources.filter(r=>r.datasetId===d.id))+'</section><section class="detail-section"><h3>整理备注</h3><p class="abstract-text">'+esc(d.notes || '尚未填写。')+'</p></section></div><div class="drawer-footer"><span class="muted">独立记录数据，保留复用关系</span><button class="btn btn-primary" data-action="edit-dataset" data-id="'+esc(d.id)+'">编辑信息</button></div></aside>';
    }
    document.body.classList.add('drawer-open');
    requestAnimationFrame(()=>{ const el=$('#drawer-root button[data-action="close-drawer"]'); if(el) el.focus(); });
  }
  function addPaper() { paperModal(); }
  function speciesRow(entry = {}) {
    return '<div class="species-row"><input class="input" name="speciesName" type="text" value="'+esc(entry.name || '')+'" placeholder="例如 Arabidopsis thaliana" maxlength="500" aria-label="物种学名"><input class="input" name="speciesLabel" type="text" value="'+esc(entry.label || '')+'" placeholder="例如 拟南芥" maxlength="500" aria-label="物种中文名"><button type="button" class="btn-icon" data-action="remove-species-row" aria-label="移除该物种">'+I('x',16)+'</button></div>';
  }
  function paperModal(id = '') {
    const existing = id ? paperById(id) : null;
    if (id && !existing) return;
    const p = existing || {};
    const species = arr(p.species).length ? arr(p.species) : [{}];
    const form='<form id="paper-form"><input type="hidden" name="id" value="'+esc(id)+'"><div class="notice-box compact">'+I('info',16)+'<span>'+(existing?'补充文献信息，已有笔记、标签、阅读进度和数据关联会保留。':'先填写题目即可收录，其余信息之后再补。新条目不会标记为演示数据。')+'</span></div><div class="form-grid">'+field('文献题目','title',p.title||'','输入论文题目',true,'text',true)+field('DOI','doi',p.doi||'','10.xxxx/… 或 DOI 链接',false,'text',true)+field('作者','authors',p.authors||'','作者或研究团队')+field('发表年份','year',p.year||'','例如 2026',false,'number')+field('期刊 / 出版来源','journal',p.journal||'','暂时没有可以留空')+'<label class="form-field"><span>发表状态</span><select class="input" name="publicationStatus">'+Object.entries({unknown:'尚未确认',preprint:'预印本',published:'正式发表',unpublished:'未发表'}).map(([v,l])=>option(v,l,p.publicationStatus||'unknown')).join('')+'</select></label><label class="form-field"><span>文献类型（可多选）</span>'+checkboxGroup('articleTypes', ARTICLE_TYPES, arr(p.articleTypes).filter(v => ARTICLE_TYPES.includes(v)))+'</label>'+'<div class="form-field full-width"><span>研究物种（一篇文献可涉及多个）</span><div class="species-rows" id="species-rows">'+species.map(speciesRow).join('')+'</div><button type="button" class="text-link" data-action="add-species-row">'+I('plus',15)+' 添加物种</button></div>'+'<label class="form-field"><span>组织 / 部位（逗号分隔）</span><input class="input" name="organs" type="text" value="'+esc(joinList(p.organs, ', '))+'" placeholder="例如 根, 叶" maxlength="2000"></label>'+'<div class="form-field full-width"><span>测序类型（可多选）</span>'+checkboxGroup('modalities', MODALITY_CHOICES, arr(p.modalities), customField('modalitiesCustom','其他类型，逗号分隔'))+'</div>'+'<div class="form-field full-width"><span>测序平台（可多选）</span>'+checkboxGroup('platforms', PLATFORM_CHOICES, arr(p.platforms), customField('platformsCustom','其他平台，逗号分隔'))+'</div>'+'<label class="form-field full-width"><span>摘要</span><textarea class="textarea" name="abstract" rows="4" maxlength="50000" placeholder="可粘贴摘要，暂时没有也可以留空">'+esc(p.abstract||'')+'</textarea></label><label class="form-field full-width"><span>收录判断依据'+(p.screening==='excluded'?' <b class="required">*</b>':'')+'</span><textarea class="textarea" name="screeningReason" rows="2" maxlength="3000" placeholder="例如：实际分析了植物单核数据；或仅包含空间转录组">'+esc(p.screeningReason||'')+'</textarea></label></div><p id="form-error" class="form-error" role="alert"></p></form>';
    showModal(existing?'编辑文献信息':'添加文献',form,'<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="paper-form">'+I(existing?'check':'plus',16)+(existing?' 保存修改':' 添加到文献库')+'</button>',true);
  }
  function savePaper(form) {
    const f=new FormData(form); const id=String(f.get('id')||''); const existing=id?paperById(id):null;
    const title=String(f.get('title')||'').trim(); const doi=normalizeDOI(f.get('doi'));
    const yearRaw=String(f.get('year')||'').trim(); const year=yearRaw?Number(yearRaw):null;
    const reason=String(f.get('screeningReason')||'').trim(); const publicationStatus=String(f.get('publicationStatus')||'unknown');
    let error='';
    if(id&&!existing) error='原条目不存在，请关闭弹窗后重试。';
    else if(!title) error='请填写文献题目。';
    else if(!validDOI(doi)) error='DOI 格式不正确，请填写 10.xxxx/… 或完整 DOI 链接。';
    else if(doi && state.papers.some(p=>p.id!==id && normalizeDOI(p.doi)===doi)) error='这个 DOI 已在文献库中，请直接整理已有条目。';
    else if(year!==null && (!Number.isInteger(year)||year<1800||year>2100)) error='请填写 1800–2100 之间的年份，或留空。';
    else if(!['unknown','preprint','published','unpublished'].includes(publicationStatus)) error='请选择有效的发表状态。';
    else if(existing && existing.screening==='excluded' && !reason) error='相关 / 排除条目需要保留判断理由。';
    if(error){ $('#form-error').textContent=error; return; }
    const species=$$('#species-rows .species-row', form).map(row => ({name: String(row.querySelector('[name=speciesName]').value||'').trim(), label: String(row.querySelector('[name=speciesLabel]').value||'').trim()})).filter(s => s.name);
    const checked = name => f.getAll(name).map(String);
    const mergeList = (name, custom) => [...new Set([...checked(name), ...splitList(f.get(custom))])].slice(0, 50);
    const now=new Date().toISOString();
    const values={title,doi,authors:String(f.get('authors')||'').trim(),year,journal:String(f.get('journal')||'').trim(),publicationStatus,species,organs:splitList(f.get('organs')).slice(0,50),modalities:mergeList('modalities','modalitiesCustom'),platforms:mergeList('platforms','platformsCustom'),articleTypes:checked('articleTypes'),abstract:String(f.get('abstract')||'').trim(),screeningReason:reason,updatedAt:now};
    let p;
    if(existing) { p=existing; Object.assign(p, values); }
    else { p={...values,id:uid('P'),pmid:'',screening:'pending',fulltext:'missing',reading:'unread',starred:false,tags:[],notes:'',source:'手动添加',addedAt:now,isDemo:false}; state.papers.unshift(p); }
    logActivity((existing?'更新文献信息：':'添加文献：')+p.title,existing?'edit':'add'); persist(); closeModal();
    if(existing){render();renderDrawer();toast('文献信息已更新，笔记与关联保持不变。');}
    else {filters={...emptyFilters(),sort:filters.sort};navigate('papers');toast('已收录，全文与数据可以之后补充。');openPaper(p.id);}
  }
  function exclusionReasonModal(p) {
    showModal('补充排除理由','<form id="exclusion-reason-form"><input type="hidden" name="paperId" value="'+esc(p.id)+'"><p class="muted">为了保留筛选依据，请记录为何将这篇文献归为相关资料或排除。</p><label class="form-field"><span>判断理由 <b class="required">*</b></span><textarea class="textarea" name="reason" rows="4" maxlength="3000" required placeholder="例如：仅包含空间转录组，没有实际分析单细胞或单核测序数据"></textarea></label><p id="form-error" class="form-error" role="alert"></p></form>','<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="exclusion-reason-form">保存判断</button>');
  }
  function datasetModal(id = '', paperId = '') {
    const existing=state.datasets.find(d=>d.id===id); const d=existing || {};
    const repository=arr(d.repositories)[0] || {repository:d.repository || '',accession:d.accession || ''};
    const repositoryNames={geo:'GEO',gsa:'GSA / CNCB',sra:'SRA',ena:'ENA',bioproject:'BioProject',biosample:'BioSample',biostudies:'BioStudies',zenodo:'Zenodo',figshare:'Figshare',dryad:'Dryad',other:'其他'};
    const availabilityNames={unknown:'尚未核实',public:'公开可访问',unavailable:'暂不可访问',embargoed:'尚未解禁',restricted:'需要申请'};
    const speciesRows=arr(d.species).length?arr(d.species).map(speciesRow).join(''):speciesRow();
    const form='<form id="dataset-form"><input type="hidden" name="id" value="'+esc(id)+'"><input type="hidden" name="paperId" value="'+esc(paperId)+'"><div class="form-grid">'+
      field('数据集名称','title',d.title||'','例如：拟南芥根尖发育数据',true,'text',true)+
      '<div class="form-field full-width"><span>研究物种</span><div class="species-rows" id="species-rows">'+speciesRows+'</div><button class="text-link" type="button" data-action="add-species-row">'+I('plus',14)+' 添加物种</button></div>'+
      '<label class="form-field full-width"><span>组织 / 部位</span><input class="input" name="organs" value="'+esc(joinList(d.organs,'，'))+'" placeholder="多个组织用逗号分隔，例如：根，叶，茎尖"></label>'+
      '<div class="form-field full-width"><span>测序类型</span>'+checkboxGroup('modalities',MODALITY_CHOICES,d.modalities,customField('modalitiesCustom','补充其他类型，多个用逗号分隔'))+'</div>'+
      '<div class="form-field full-width"><span>测序平台</span>'+checkboxGroup('platforms',PLATFORM_CHOICES,d.platforms,customField('platformsCustom','补充其他平台，多个用逗号分隔'))+'</div>'+
      '<label class="form-field"><span>数据仓库</span><select class="input" name="repository">'+option('','待补充',repository.repository)+Object.entries(repositoryNames).map(([v,l])=>option(v,l,repository.repository)).join('')+'</select></label>'+
      field('Accession','accession',repository.accession||'','可先留空')+
      '<label class="form-field"><span>整理状态</span><select class="input" name="status">'+option('pending','待整理',d.status||'pending')+option('identified','线索已记录',d.status)+option('verified','已人工核实',d.status)+'</select></label>'+
      '<label class="form-field"><span>原始数据可用性</span><select class="input" name="availability">'+Object.entries(availabilityNames).map(([v,l])=>option(v,l,d.availability||'unknown')).join('')+'</select></label>'+
      '<label class="form-field full-width"><span>整理备注</span><textarea class="textarea" name="notes" rows="3" maxlength="50000">'+esc(d.notes||'')+'</textarea></label></div><p id="form-error" class="form-error" role="alert"></p></form>';
    showModal(existing?'编辑数据集':'添加数据集',form,'<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="dataset-form">保存数据集</button>',true);
  }
  function saveDataset(form) {
    const f=new FormData(form); const title=String(f.get('title')||'').trim(); const id=String(f.get('id')||''); const accession=String(f.get('accession')||'').trim(); const status=String(f.get('status')||'pending');
    if(!title || (status==='verified' && !accession)){ $('#form-error').textContent=!title?'请填写数据集名称。':'标记为已核实前，请先填写仓库编号。'; return; }
    const repository=String(f.get('repository')||'').trim().toLowerCase();
    if(accession && !repository){ $('#form-error').textContent='填写 Accession 时，请同时选择数据仓库。'; return; }
    const species=$$('#species-rows .species-row',form).map(row=>({name:String(row.querySelector('[name=speciesName]').value||'').trim(),label:String(row.querySelector('[name=speciesLabel]').value||'').trim()})).filter(s=>s.name);
    const checked=name=>f.getAll(name).map(String);
    const mergeList=(name,custom)=>[...new Set([...checked(name),...splitList(f.get(custom))])].slice(0,50);
    const existing=state.datasets.find(d=>d.id===id);const d=existing||{};
    const repositories=arr(d.repositories).map(clone);const previous=repositories[0] || ((d.repository||d.accession)?{repository:d.repository,accession:d.accession}:null);
    if(previous||repository||accession)repositories[0]={...(previous||{}),repository,accession};
    const values={title,species,organs:splitList(f.get('organs')).slice(0,50),modalities:mergeList('modalities','modalitiesCustom'),platforms:mergeList('platforms','platformsCustom'),repositories,repository,accession,status,availability:String(f.get('availability')||'unknown'),notes:String(f.get('notes')||'').trim(),updatedAt:new Date().toISOString()};
    let saved;
    if(existing) { saved=existing; Object.assign(saved,values); }
    else { saved={...values,id:uid('DS'),matrixAvailability:'unknown',annotationAvailability:'unknown',cellsReported:null,cellCountUnit:'',cellCountScope:'',paperLinks:[],isDemo:false,addedAt:new Date().toISOString()}; const pid=String(f.get('paperId')||''); if(paperById(pid)) saved.paperLinks.push({paperId:pid,role:'generated'}); state.datasets.push(saved); }
    normalizeDataset(saved);logActivity((id?'更新':'添加')+'数据集：'+title); persist(); closeModal(); render(); openDataset(saved.id); toast('数据集已保存。');
  }
  function linkDataset(paperId) {
    const available=state.datasets.filter(d=>!(d.paperLinks||[]).some(l=>l.paperId===paperId));
    showModal('关联数据集','<p class="muted">关联已有数据可保留复用关系，避免重复创建同一批数据。</p><form id="link-dataset-form"><input type="hidden" name="paperId" value="'+esc(paperId)+'"><label class="form-field"><span>选择数据集</span><select class="input" name="datasetId" required><option value="">请选择</option>'+available.map(d=>option(d.id,d.title,'')).join('')+'</select></label><label class="form-field"><span>这篇论文与数据的关系</span><select class="input" name="role">'+Object.entries(ROLE).map(([v,l])=>option(v,l,'reused')).join('')+'</select></label><p id="form-error" class="form-error" role="alert"></p></form>','<button class="btn btn-secondary" data-action="new-dataset-for-paper" data-id="'+esc(paperId)+'">创建新数据集</button><button class="btn btn-primary" type="submit" form="link-dataset-form">确认关联</button>');
  }
  function resourceModal(kind,id) {
    showModal('添加资料链接','<form id="resource-form"><input type="hidden" name="kind" value="'+esc(kind)+'"><input type="hidden" name="ownerId" value="'+esc(id)+'"><label class="form-field"><span>资料类型</span><select class="input" name="type">'+Object.entries({fulltext:'全文入口',supplement:'补充材料',data:'数据入口',matrix:'表达矩阵',annotation:'细胞注释',code:'分析代码',other:'其他资料'}).map(([v,l])=>option(v,l,kind==='paper'?'fulltext':'data')).join('')+'</select></label>'+field('资料名称','title','','例如：期刊全文页面',true)+field('链接地址','url','','https://…',true,'url')+'<p class="muted small">仅保存链接，不会自动下载正文或数据文件。</p><p id="form-error" class="form-error" role="alert"></p></form>','<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" type="submit" form="resource-form">保存链接</button>');
  }
  function saveResource(form) {
    const f=new FormData(form); const url=safeURL(f.get('url')); const title=String(f.get('title')||'').trim(); const kind=String(f.get('kind')); const ownerId=String(f.get('ownerId')); const type=String(f.get('type'));
    if(!url || !title){ $('#form-error').textContent=!title?'请填写资料名称。':'请填写有效的 HTTP 或 HTTPS 地址。'; return; }
    if(state.resources.some(r=>r.url===url && (kind==='paper'?r.paperId===ownerId:r.datasetId===ownerId))){ $('#form-error').textContent='这条链接已经保存。'; return; }
    state.resources.push({id:uid('R'),paperId:kind==='paper'?ownerId:null,datasetId:kind==='dataset'?ownerId:null,title,url,type,status:'linked',isDemo:false,addedAt:new Date().toISOString()});
    if(kind==='paper' && type==='fulltext'){ const p=paperById(ownerId); if(p && p.fulltext==='missing') p.fulltext='linked'; }
    logActivity('保存资料入口：'+title); persist(); closeModal(); render(); renderDrawer(); toast('链接已保存；文件获取状态保持未下载。');
  }
  function downloadFile(filename, content, mime) {
    const blob=new Blob([content], {type:mime}); const url=URL.createObjectURL(blob); const a=document.createElement('a');
    a.href=url; a.download=filename; document.body.appendChild(a); a.click(); a.remove(); setTimeout(()=>URL.revokeObjectURL(url),5000);
  }
  function exportCSV() {
    const list=filteredPapers();
    const csvCell=value=> { let s=String(value==null?'':value); if(/^[=+\-@\t\r]/.test(s)) s="'"+s; return '"'+s.replaceAll('"','""')+'"'; };
    const headers=['内部编号','题目','DOI','作者','年份','物种（中文名）','物种（学名）','组织','技术','测序平台','文献类型','收录状态','全文情况','阅读进度','关联数据集','标签','笔记','是否示例'];
    const rows=list.map(p=>[p.id,p.title,p.doi,p.authors,p.year,joinList(arr(p.species).map(s=>s&&(s.label||s.name)||'')),joinList(speciesNames(p)),joinList(p.organs,';'),joinList(p.modalities,';'),joinList(p.platforms,';'),joinList(p.articleTypes,';'),SCREEN[p.screening],FULL[p.fulltext],READING[p.reading],paperDatasets(p.id).map(d=>d.id).join(';'),(p.tags||[]).join(';'),p.notes,p.isDemo?'是':'否'].map(csvCell).join(','));
    downloadFile('plantcell-papers-'+new Date().toISOString().slice(0,10)+'.csv','\ufeff'+headers.map(csvCell).join(',')+'\r\n'+rows.join('\r\n'),'text/csv;charset=utf-8');
    toast('已导出当前筛选的 '+list.length+' 篇文献。');
  }
  function exportJSON() {
    downloadFile('plantcell-library-'+new Date().toISOString().slice(0,10)+'.json',JSON.stringify({...state,exportedAt:new Date().toISOString()},null,2),'application/json;charset=utf-8');
    toast('已导出文献、数据集、链接与笔记。');
  }
  function importJSON() {
    const input=document.createElement('input'); input.type='file'; input.accept='.json,application/json'; input.hidden=true; document.body.appendChild(input);
    input.addEventListener('change', async()=>{
      const file=input.files[0]; if(!file){input.remove();return;}
      try {
        if(file.size>10*1024*1024) throw new Error('请导入 10 MB 以内的 JSON 文件。');
        const data=JSON.parse(await file.text()); prepareImport(data);
      } catch(e){toast(e instanceof SyntaxError?'JSON 格式无效，请使用本工具导出的备份文件。':e.message || '导入失败，原有数据未改变。',true);}
      input.remove();
    });
    input.click();
  }
  function prepareImport(data) {
    if(!data || (data.schemaVersion!==1 && data.schemaVersion!==2) || !Array.isArray(data.papers) || !Array.isArray(data.datasets) || !Array.isArray(data.resources)) throw new Error('文件结构不匹配，请使用本工具导出的 JSON 备份。');
    if(data.papers.length>20000 || data.datasets.length>20000 || data.resources.length>50000) throw new Error('导入记录数量超出原型限制，请分批整理。');
    const textValue=(v,max=50000)=> { if(v==null)return ''; if(typeof v!=='string' && typeof v!=='number') throw new Error('有字段的文本类型不正确。'); const s=String(v); if(s.length>max) throw new Error('有字段内容过长。'); return s; };
    const enumValue=(v,allowed,fallback)=> { if(v==null || v==='')return fallback; if(!allowed.includes(v))throw new Error('存在不支持的状态值：'+String(v).slice(0,60)); return v; };
    // schemaVersion 1 备份使用单值字段（species/organ/modality/articleType），
    // 导入时自动包装成数组；新备份直接给数组。
    const speciesList=v=> {
      if(Array.isArray(v)) return v.filter(x=>x && typeof x==='object' && typeof x.name==='string' && x.name.trim()).slice(0,50).map(x=>({name:x.name.trim().slice(0,500),label:typeof x.label==='string'?x.label.trim().slice(0,500):''}));
      const s=textValue(v,500).trim(); return s?[{name:s.slice(0,500),label:''}]:[];
    };
    const stringList=(v,legacy)=> {
      const fromArray=Array.isArray(v)?v.filter(x=>typeof x==='string').map(x=>x.trim().slice(0,200)).filter(Boolean):[];
      if(fromArray.length) return [...new Set(fromArray)].slice(0,50);
      const s=textValue(v,200).trim() || textValue(legacy,200).trim();
      return s?[s.slice(0,200)]:[];
    };
    const next=clone(state); const idMap=new Map(); const dsMap=new Map(); const importedIds=new Set(); const importedDatasetIds=new Set();
    const stats={papers:0,datasets:0,resources:0,duplicates:0,renamed:0};
    for(const row of data.papers){
      if(!row || typeof row!=='object')throw new Error('文献条目格式不正确。');
      const oldId=textValue(row.id,100) || uid('IMP-P'); const title=textValue(row.title,2000).trim(); const doi=normalizeDOI(textValue(row.doi,500));
      if(!title || !validDOI(doi)) throw new Error('文献题目缺失或 DOI 格式不正确。');
      if(importedIds.has(oldId))throw new Error('导入文件中包含重复的文献 ID。'); importedIds.add(oldId);
      const year=row.year==null || row.year===''?null:Number(row.year);
      if(year!==null && (!Number.isInteger(year)||year<1800||year>2100))throw new Error('文献年份格式不正确。');
      const doiMatch=doi?next.papers.find(p=>normalizeDOI(p.doi)===doi):null;
      const idMatch=next.papers.find(p=>p.id===oldId);
      if(doiMatch || (idMatch && idMatch.title===title && normalizeDOI(idMatch.doi)===doi)){idMap.set(oldId,(doiMatch||idMatch).id);stats.duplicates++;continue;}
      const id=idMatch?uid('IMP-P'):oldId; if(idMatch)stats.renamed++;
      if(row.tags!=null && (!Array.isArray(row.tags)||row.tags.some(t=>typeof t!=='string')))throw new Error('主题标签应为文本数组。');
      const p={id,title,doi,pmid:textValue(row.pmid,100),authors:textValue(row.authors,2000),year,journal:textValue(row.journal,1000),species:speciesList(row.species),organs:stringList(row.organs,row.organ),modalities:stringList(row.modalities,row.modality),platforms:stringList(row.platforms),articleTypes:stringList(row.articleTypes,row.articleType),abstract:textValue(row.abstract),notes:textValue(row.notes),tags:(row.tags||[]).slice(0,50).map(t=>t.slice(0,100)),screening:enumValue(row.screening,['pending','included','excluded','uncertain'],'pending'),reading:enumValue(row.reading,['unread','skimmed','read'],'unread'),fulltext:enumValue(row.fulltext,['missing','linked','stored'],'missing'),publicationStatus:enumValue(row.publicationStatus,['unknown','published','preprint','unpublished'],'unknown'),starred:row.starred===true,isDemo:row.isDemo===true,source:textValue(row.source,300)||'JSON 导入',screeningReason:textValue(row.screeningReason,3000),addedAt:isNaN(Date.parse(row.addedAt))?new Date().toISOString():new Date(row.addedAt).toISOString(),updatedAt:new Date().toISOString()};
      idMap.set(oldId,id);next.papers.push(p);stats.papers++;
    }
    for(const row of data.datasets){
      if(!row || typeof row!=='object')throw new Error('数据集条目格式不正确。');
      const oldId=textValue(row.id,100)||uid('IMP-DS');const title=textValue(row.title,2000).trim();if(!title)throw new Error('数据集名称缺失。');
      if(importedDatasetIds.has(oldId))throw new Error('导入文件中包含重复的数据集 ID。');importedDatasetIds.add(oldId);
      if(row.paperLinks!=null && !Array.isArray(row.paperLinks))throw new Error('文献关联必须是数组。');
      const paperLinks=(row.paperLinks||[]).map(l=>{if(!l || typeof l!=='object')throw new Error('文献关联格式不正确。');const pid=idMap.get(String(l.paperId)) || (next.papers.some(p=>p.id===l.paperId)?l.paperId:null);if(!pid)throw new Error('数据集引用了不存在的文献。');return {paperId:pid,role:enumValue(l.role,['generated','reused','reprocessed','benchmarked','unknown'],'reused'),evidenceUrl:textValue(l.evidenceUrl),evidenceText:textValue(l.evidenceText),verified:l.verified===true};});
      const existing=next.datasets.find(d=>d.id===oldId);let d;
      if(existing && existing.title===title){d=existing;stats.duplicates++;paperLinks.forEach(l=>{if(!(d.paperLinks||[]).some(x=>x.paperId===l.paperId))d.paperLinks.push(l);});}
      else {
        const id=existing?uid('IMP-DS'):oldId;if(existing)stats.renamed++;
        const cells=row.cellsReported==null?null:Number(row.cellsReported);if(cells!==null&&(!Number.isInteger(cells)||cells<0))throw new Error('细胞数量应为非负整数或空值。');
        d=normalizeDataset({id,title,description:textValue(row.description),species:speciesList(row.species),organs:stringList(row.organs,row.organ),modalities:stringList(row.modalities,row.modality),platforms:stringList(row.platforms,row.technology),repositories:Array.isArray(row.repositories)?row.repositories:[],repository:textValue(row.repository,200).toLowerCase(),accession:textValue(row.accession,500),status:enumValue(row.status,['pending','identified','verified'],'pending'),curationLevel:textValue(row.curationLevel,100),availability:enumValue(row.availability,['unknown','public','unavailable','embargoed','restricted'],'unknown'),matrixAvailability:enumValue(row.matrixAvailability,['unknown','public','unavailable','embargoed','restricted'],'unknown'),annotationAvailability:enumValue(row.annotationAvailability,['unknown','public','unavailable','embargoed','restricted'],'unknown'),cellsReported:cells,cellCountUnit:textValue(row.cellCountUnit,100),cellCountScope:textValue(row.cellCountScope,1000),notes:textValue(row.notes),isDemo:row.isDemo===true,paperLinks,addedAt:isNaN(Date.parse(row.addedAt))?new Date().toISOString():new Date(row.addedAt).toISOString()});
        if(typeof row.species==='string'&&d.species[0])d.species[0].label=textValue(row.speciesLabel,500);
        next.datasets.push(d);stats.datasets++;
      }
      dsMap.set(oldId,d.id);
    }
    for(const row of data.resources){
      if(!row || typeof row!=='object')throw new Error('资料条目格式不正确。');
      const paperId=row.paperId?(idMap.get(String(row.paperId))||(next.papers.some(p=>p.id===row.paperId)?row.paperId:null)):null;
      const datasetId=row.datasetId?(dsMap.get(String(row.datasetId))||(next.datasets.some(d=>d.id===row.datasetId)?row.datasetId:null)):null;
      if((row.paperId&&!paperId)||(row.datasetId&&!datasetId)||(!paperId&&!datasetId))throw new Error('资料引用的文献或数据集不存在。');
      const url=safeURL(row.url);if(!url)throw new Error('资料链接必须是有效的 HTTP 或 HTTPS 地址。');
      if(next.resources.some(r=>r.url===url&&r.paperId===paperId&&r.datasetId===datasetId)){stats.duplicates++;continue;}
      const type=enumValue(row.type,['fulltext','supplement','data','matrix','annotation','code','other'],'other');
      next.resources.push({id:uid('IMP-R'),paperId,datasetId,url,type,title:textValue(row.title,1000)||'导入资料',status:'linked',isDemo:row.isDemo===true,addedAt:new Date().toISOString()});stats.resources++;
    }
    const summary='<p>检查完成，将追加以下内容：</p><div class="import-summary"><div><strong>'+stats.papers+'</strong><span>篇新文献</span></div><div><strong>'+stats.datasets+'</strong><span>组新数据集</span></div><div><strong>'+stats.resources+'</strong><span>个新链接</span></div></div><p class="muted">'+stats.duplicates+' 条已存在记录保留原值，不覆盖笔记与整理状态。'+(stats.renamed?' '+stats.renamed+' 个冲突 ID 将自动分配新 ID。':'')+'</p><p class="muted small">导入只合并记录，不会删除现有内容。导入文件中的文件保存标记仅作整理记录，本原型不会搬运 PDF 文件。</p>';
    showModal('确认合并导入',summary,'<button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-primary" data-action="confirm-import">合并导入</button>');
    pendingImport={next,stats};
  }
  function resetDemo() {
    showModal('恢复演示数据','<p>这会清除本浏览器里的新增条目、修改和笔记，并恢复最初的示例内容。</p><p class="muted">如有需要保留的整理结果，请先导出 JSON 备份。</p>','<button class="btn btn-secondary" data-action="export-json">先导出备份</button><button class="btn btn-secondary" data-action="close-modal">取消</button><button class="btn btn-danger" data-action="confirm-reset">恢复演示数据</button>');
  }
  document.addEventListener('click', e=>{
    const el=e.target.closest('[data-action]');
    if(el){
      const action=el.dataset.action;const id=el.dataset.id;
      if(realMode && ['import-json','confirm-import','reset-demo','confirm-reset','add-resource'].includes(action)){
        toast('真实模式当前支持文献、数据集与整理记录；附件和 JSON 导入将在后续版本接入 SQLite。',true);return;
      }
      if(action==='new-paper'){addPaper();return;}
      if(action==='configure-source'){sourceConfigModal(id);return;}
      if(action==='sync-source'){syncSource(id);return;}
      if(action==='source-records'){sourceRecordsModal(id);return;}
      if(action==='choose-database'){
        if(typeof window.PLANT_CHOOSE_DATABASE_PATH!=='function')return;
        el.disabled=true;window.PLANT_CHOOSE_DATABASE_PATH().then(path=>{if(path){$('#settings-database-path').value=path;$('#settings-save-status').textContent='已选择新位置，保存并重启后生效。';}}).catch(error=>{toast('无法选择文件夹：'+(error&&error.message?error.message:'请重试'),true);}).finally(()=>{el.disabled=false;});return;
      }
      if(action==='reset-font-size'){const input=$('#settings-font-size');if(input){input.value='15';input.dispatchEvent(new Event('input',{bubbles:true}));}return;}
      if(action==='toggle-mcp-token'){const input=$('#settings-mcp-token');if(input){const show=input.type==='password';input.type=show?'text':'password';el.textContent=show?'隐藏':'显示';}return;}
      if(action==='copy-mcp-token'){
        const input=$('#settings-mcp-token');if(!input||!input.value)return;
        if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(input.value).then(()=>toast('MCP Token 已复制。')).catch(()=>toast('复制失败，请显示 Token 后手动复制。',true));}
        else{input.type='text';input.select();const copied=document.execCommand('copy');input.type='password';toast(copied?'MCP Token 已复制。':'复制失败，请手动复制。',!copied);}return;
      }
      if(action==='regenerate-mcp-token'){
        const input=$('#settings-mcp-token');if(!input)return;const bytes=new Uint8Array(32);crypto.getRandomValues(bytes);input.value=Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('');input.type='text';const toggle=$('[data-action="toggle-mcp-token"]');if(toggle)toggle.textContent='隐藏';$('#settings-save-status').textContent='已生成新 Token，保存并重启后生效。';return;
      }
      if(action==='edit-paper'){paperModal(id);return;}
      if(action==='add-species-row'){const rows=$('#species-rows');if(rows){rows.insertAdjacentHTML('beforeend',speciesRow());const inputs=$$('.species-row',rows);inputs[inputs.length-1].querySelector('[name=speciesName]').focus();}return;}
      if(action==='remove-species-row'){const row=el.closest('.species-row');if(row)row.remove();return;}
      if(action==='open-paper'){openPaper(id);return;}
      if(action==='open-dataset'){openDataset(id);return;}
      if(action==='close-drawer'){closeDrawer();return;}
      if(action==='close-modal'){closeModal();return;}
      if(action==='drawer-tab'){drawerTab=el.dataset.tab;renderDrawer();return;}
      if(action==='export-csv'){exportCSV();return;}
      if(action==='export-json'){exportJSON();return;}
      if(action==='import-json'){importJSON();return;}
      if(action==='reset-demo'){resetDemo();return;}
      if(action==='confirm-reset'){state=clone(window.PLANT_DEMO_DATA);state.datasets=state.datasets.map(normalizeDataset);logActivity('恢复演示数据','reset');persist();closeModal();closeDrawer();filters=emptyFilters();navigate('overview');toast('已恢复示例内容。');return;}
      if(action==='confirm-import' && pendingImport){const next=pendingImport.next;const stats=pendingImport.stats;state=next;logActivity('导入 '+stats.papers+' 篇文献、'+stats.datasets+' 组数据集和 '+stats.resources+' 个链接','import');persist();closeModal();closeDrawer();render();toast('导入完成，已有记录保持原值。');return;}
      if(action==='toggle-star'){const p=paperById(id);if(p){p.starred=!p.starred;p.updatedAt=new Date().toISOString();persist();render();if(drawer)renderDrawer();}return;}
      if(action==='save-notes'){const p=paperById(id);if(p){p.notes=$('#paper-notes').value;p.updatedAt=new Date().toISOString();logActivity('保存笔记：'+p.title);persist();toast('笔记已保存。');}return;}
      if(action==='save-tags'){const p=paperById(id);if(p){p.tags=[...new Set($('#paper-tags').value.split(/[,，;；\n]/).map(s=>s.trim()).filter(Boolean))].slice(0,30);p.updatedAt=new Date().toISOString();persist();toast('标签已保存。');render();}return;}
      if(action==='clear-filters'){filters=emptyFilters();if($('#global-search'))$('#global-search').value='';render();return;}
      if(action==='all-papers'){filters.starred=false;render();return;}
      if(action==='starred-papers'){filters.starred=true;render();return;}
      if(action==='task-filter'){taskFilter=el.dataset.filterValue;render();return;}
      if(action==='stat-nav'){const target=el.dataset.target;if(['screen','fulltext'].includes(target)){taskFilter=target;navigate('tasks');}else navigate(target);return;}
      if(action==='species-filter'){let name='';for(const pp of state.papers){for(const s of arr(pp.species)){if(s&&(s.label===el.dataset.label||s.name===el.dataset.label)){name=s.name;break;}}if(name)break;}if(name){filters.species=name;navigate('papers');}return;}
      if(action==='new-dataset'){datasetModal();return;}
      if(action==='edit-dataset'){datasetModal(id);return;}
      if(action==='new-dataset-for-paper'){closeModal();datasetModal('',id);return;}
      if(action==='link-dataset'){linkDataset(id);return;}
      if(action==='add-resource'){resourceModal(el.dataset.kind,id);return;}
    }
    const nav=e.target.closest('[data-page]');if(nav)navigate(nav.dataset.page);
  });
  document.addEventListener('submit',e=>{
    const formId = e.target.getAttribute('id');
    if(formId==='paper-form'){e.preventDefault();savePaper(e.target);}
    else if(formId==='source-config-form'){e.preventDefault();saveSourceConfig(e.target);}
    else if(formId==='settings-form'){e.preventDefault();saveSettings(e.target);}
    else if(formId==='exclusion-reason-form'){
      e.preventDefault();const f=new FormData(e.target);const p=paperById(String(f.get('paperId')));const reason=String(f.get('reason')||'').trim();
      if(!p||!reason){$('#form-error').textContent='请填写判断理由。';return;}
      p.screening='excluded';p.screeningReason=reason;p.updatedAt=new Date().toISOString();logActivity('保存排除判断：'+p.title);persist();closeModal();render();renderDrawer();toast('判断与理由已保存。');
    }
    else if(formId==='dataset-form'){e.preventDefault();saveDataset(e.target);}
    else if(formId==='resource-form'){e.preventDefault();saveResource(e.target);}
    else if(formId==='link-dataset-form'){
      e.preventDefault();const f=new FormData(e.target);const d=state.datasets.find(x=>x.id===f.get('datasetId'));const pid=String(f.get('paperId'));
      if(!d||!paperById(pid)){ $('#form-error').textContent='请选择一个有效的数据集。';return; }
      if(!(d.paperLinks||[]).some(l=>l.paperId===pid))d.paperLinks.push({paperId:pid,role:String(f.get('role'))});
      logActivity('关联数据集：'+d.title);persist();closeModal();render();renderDrawer();toast('文献与数据集已关联。');
    }
  });
  document.addEventListener('change',e=>{
    if(e.target.matches('#source-config-form [name="provider"]')){
      const form=e.target.form;const query=form.elements.query;
      if(query.value===form.dataset.initialQuery){
        const common='("single cell"[All Fields] OR "single-cell"[All Fields] OR "single nucleus"[All Fields]) AND (plant[All Fields] OR Arabidopsis[Organism])';
        const defaults={geo:common+' AND gse[ETYP]',sra:common,ena:'description="*single*" AND tax_tree(33090)'};
        query.value=defaults[e.target.value]||query.value;form.dataset.initialQuery=query.value;
      }
      return;
    }
    if(e.target.id==='demo-mode'){
      localStorage.setItem('bibliocell-demo-mode',String(e.target.checked));
      location.reload();return;
    }
    if(e.target.matches('[data-filter]')){filters[e.target.dataset.filter]=e.target.value;render();}
    if(e.target.matches('[data-paper-setting]')){
      const p=paperById(e.target.dataset.id);if(!p)return;const key=e.target.dataset.paperSetting;const value=e.target.value;
      if(key==='screening'&&Object.hasOwn(SCREEN,value)){if(value==='excluded'&&!String(p.screeningReason||'').trim()){e.target.value=p.screening;exclusionReasonModal(p);return;}p.screening=value;}
      else if(key==='reading'&&Object.hasOwn(READING,value))p.reading=value;else return;
      p.updatedAt=new Date().toISOString();logActivity('更新'+(key==='reading'?'阅读进度':'收录判断')+'：'+p.title);persist();render();renderDrawer();toast('整理状态已保存。');
    }
  });
  document.addEventListener('input',e=>{
    if(e.target.id==='settings-font-size'){
      const size=Number(e.target.value);if($('#settings-font-size-value'))$('#settings-font-size-value').textContent=size+' px';
      if(typeof window.PLANT_APPLY_FONT_SIZE==='function')window.PLANT_APPLY_FONT_SIZE(size);
      if($('#settings-save-status'))$('#settings-save-status').textContent='正在预览 '+size+' px，点击“保存设置”后保留。';
    }
    if(e.target.id==='paper-search'||e.target.id==='dataset-search'){
      const id=e.target.id;const value=e.target.value;const start=e.target.selectionStart;
      clearTimeout(searchTimer);searchTimer=setTimeout(()=>{if(id==='paper-search')filters.search=value;else datasetSearch=value;render();const input=document.getElementById(id);if(input){input.focus();try{input.setSelectionRange(start,start);}catch(_){}}},140);
    }
  });
  document.addEventListener('keydown',e=>{
    if(e.key==='Escape'){if($('#modal-root .modal'))closeModal();else if(drawer)closeDrawer();else document.body.classList.remove('nav-open');return;}
    if(e.key==='Enter'&&e.target.id==='global-search'){e.preventDefault();filters.search=e.target.value;filters.species='';filters.organ='';filters.year='';filters.modality='';filters.platform='';filters.fulltext='';filters.screening='';filters.starred=false;navigate('papers');return;}
    if(e.key==='Enter'&&e.target.matches('tr[data-action]')){e.preventDefault();openPaper(e.target.dataset.id);return;}
    if((e.key==='/'||((e.ctrlKey||e.metaKey)&&e.key==='k'))&&!e.target.matches('input,textarea,select')){e.preventDefault();if($('#global-search'))$('#global-search').focus();return;}
    if(e.key==='Tab'){
      const container=$('#modal-root .modal')||$('#drawer-root .drawer');if(!container)return;
      const focusable=$$('button:not([disabled]), a[href], input:not([type=hidden]), select, textarea, [tabindex="0"]',container).filter(el=>el.getClientRects().length);
      if(!focusable.length)return;const first=focusable[0],last=focusable[focusable.length-1];
      if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}
      else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}
    }
  });
  window.addEventListener('hashchange',()=>{const next=location.hash.slice(1);if(pageMeta[next]){page=next;render();}});
  window.PlantApp={navigate,openPaper,openDataset,addPaper,exportCSV,exportJSON,importJSON,resetDemo,closeDrawer,closeModal};
  function boot(){render();if(persistenceError)toast('本地存储暂不可用，操作后请导出 JSON 备份。',true);}
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot);else boot();
})();

export {};
