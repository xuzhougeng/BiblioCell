import type { App } from "./app.ts";
import { Library, type DatasetAsset, type DatasetManifest, type DatasetReference, type DatasetSample } from "./mygo.ts";
import { esc, fileSize, filterDatasets, safeURL } from "./model.ts";
import { badge, errorMessage, field, options, setFormError, text } from "./ui.ts";
import { t } from "./i18n.ts";
import { cancelButton } from "./forms.ts";

export const ASSET_KINDS: Record<string,string> = { matrix:t("表达矩阵"), cell_metadata:t("细胞信息"), feature_metadata:t("基因信息"), pep:"PEP", cds:"CDS", gene_map:t("基因—转录本—蛋白映射"), raw_reads:t("原始测序文件"), other:t("其他 / 待检查") };
const MATRIX_KINDS = { unknown:t("矩阵数值类型待确认"), counts:t("原始 counts"), normalized:t("归一化值") };
const LOCAL_STATES: Record<string,string> = { none:t("未登记本地文件"), registered:t("已登记 · 待校验"), verified:t("文件已校验"), missing:t("文件不可读"), changed:t("文件内容已变化") };
const MATCH_STATES = { unresolved:t("待寻找"), candidate:t("候选版本"), matched:t("对应关系已确认") };
export const PREPARATION_FILTERS = { "":t("全部整理进度"), unallocated:t("未创建目录"), no_matrix:t("缺少矩阵记录"), pending_files:t("文件待校验"), verified:t("有已校验文件"), reference_pending:t("参考序列待匹配"), reference_matched:t("参考序列已匹配") };

export function referenceComplete(m: DatasetManifest): boolean {
  return m.plan.references.length > 0 && m.plan.references.every(r => r.status === "matched" && ["pep", "cds", "gene_map"].every(kind => m.assets.some(a => a.referenceKey === r.key && a.kind === kind && a.localState === "verified")));
}

export function matchesPreparation(m: DatasetManifest | undefined, filter: string): boolean {
  if (!filter) return true;
  if (filter === "unallocated") return !m?.allocated;
  if (filter === "no_matrix") return !m?.assets.some(a => a.kind === "matrix");
  if (filter === "pending_files") return !m?.assets.length || m.assets.some(a => a.localState !== "verified");
  if (filter === "verified") return !!m?.assets.some(a => a.localState === "verified");
  if (filter === "reference_pending") return !m || !referenceComplete(m);
  if (filter === "reference_matched") return !!m && referenceComplete(m);
  return true;
}

/** The table, keyboard navigation, review queue and export share one filtered set. */
export function matchingDatasets(app: App) {
  const manifests = new Map(app.state.datasetManifests?.map(m => [m.datasetId, m]));
  return filterDatasets(app.state.datasets, app.datasetSearch, id => app.paperById(id)?.title ?? "", app.datasetReview)
    .filter(d => matchesPreparation(manifests.get(d.id), app.datasetPreparation));
}

function manifest(app: App, id: string): DatasetManifest | undefined { return app.state.datasetManifests?.find(m => m.datasetId === id); }
const action = (name: string, id: string, label: string, extra = "") => `<button class="btn btn-secondary btn-compact" data-action="${name}" data-id="${esc(id)}" ${extra}>${label}</button>`;

export function datasetPreparationBadge(app: App, id: string): string {
  const m = manifest(app,id);
  if (!m || !app.env.real) return "";
  const verified = m.assets.filter(a => a.localState === "verified").length;
  return `<div class="dataset-preparation-badges">${badge(m.allocated ? t("目录已创建") : t("目录待创建"),m.allocated ? "blue" : "gray")}${badge(t("文件校验 {n}/{total}",{n:verified,total:m.assets.length}),verified ? "green":"gray")}</div>`;
}

export function renderDatasetAssets(app: App,id: string): string {
  if (!app.env.real) return "";
  const m = manifest(app,id);
  if (!m) return `<section class="detail-section"><p>${t("资源清单尚未加载，请刷新。")}</p></section>`;
  const write = !app.env.readOnly;
  const dataset = app.state.datasets.find(d => d.id === id)!;
  const geo = dataset.repositories.filter(r => r.repository.toLowerCase() === "geo" && /^GSE\d+$/i.test(r.accession));
  const controls = write ? `${action("dataset-prepare",id,t("创建存储目录"))}${app.env.desktop ? action("dataset-open-folder",id,t("打开目录")) : ""}${action("dataset-export-manifest",id,t("导出资源清单"))}${action("dataset-asset-edit",id,t("登记文件"))}` : "";
  const discovery = write ? geo.map(r => action("dataset-discover",id,t("发现 {accession} 文件",{accession:r.accession}),`data-accession="${esc(r.accession)}"`)).join("") : "";
  const rows = m.assets.map(a => {
    const url = safeURL(a.url);
    const source = url ? `<a class="text-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer">${t("来源文件")}</a>` : a.url ? `<code>${esc(a.url)}</code>` : "";
    return `<article class="dataset-asset"><div class="section-title"><strong>${esc(a.title)}</strong>${badge(ASSET_KINDS[a.kind] ?? a.kind)}</div><div class="dataset-asset-meta">${badge(LOCAL_STATES[a.localState] ?? a.localState,a.localState === "verified" ? "green":"gray")}${a.kind === "matrix" ? badge(MATRIX_KINDS[a.matrixKind as keyof typeof MATRIX_KINDS] ?? a.matrixKind):""}${source}</div>${a.sampleIds.length ? `<p>${t("样本")}：${esc(a.sampleIds.join(", "))}</p>`:""}${a.referenceKey ? `<p>${t("参考版本标识")}：${esc(a.referenceKey)}</p>`:""}${a.localPath ? `<code class="dataset-path">${esc(a.localPath)}</code>`:""}${a.checkedAt ? `<small>${t("上次校验")} ${esc(a.checkedAt)} · ${fileSize(a.sizeBytes)}</small>`:""}${a.sha256 ? `<details><summary>SHA-256</summary><code class="dataset-path">${esc(a.sha256)}</code></details>`:""}${a.notes ? `<p class="muted">${esc(a.notes)}</p>`:""}${write ? `<div class="section-actions">${action("dataset-asset-edit",id,t("编辑"),`data-asset="${esc(a.id)}"`)}${a.localPath ? action("dataset-asset-verify",id,t("校验本地文件"),`data-asset="${esc(a.id)}"`):""}${action("dataset-asset-remove",id,t("移除记录"),`data-asset="${esc(a.id)}"`)}</div>`:""}</article>`;
  }).join("");
  const samples = m.plan.samples.map(s => `<div class="dataset-plan-row"><div><strong>${esc(s.id)}</strong> ${esc(s.title)}<small>${esc([s.species,s.tissue,s.condition,s.replicate].filter(Boolean).join(" · "))}</small>${s.notes ? `<details><summary>${t("样本说明与处理记录")}</summary><p class="muted dataset-sample-notes">${esc(s.notes)}</p></details>`:""}</div>${write ? action("dataset-sample-edit",id,t("编辑"),`data-key="${esc(s.id)}"`):""}</div>`).join("");
  const references = m.plan.references.map(r => `<div class="dataset-plan-row"><div><strong>${esc(r.key)}</strong> ${badge(MATCH_STATES[r.status as keyof typeof MATCH_STATES] ?? r.status,r.status === "matched" ? "green":"amber")}<small>${esc([r.species,r.assembly,r.annotation].filter(Boolean).join(" · "))}</small><p>${esc(r.evidence)}</p></div>${write ? action("dataset-reference-edit",id,t("编辑"),`data-key="${esc(r.key)}"`):""}</div>`).join("");
  return `<section class="detail-section dataset-workspace"><h3>${t("数据集资源与存储")}</h3>${datasetPreparationBadge(app,id)}<p class="muted">${t("位置相对于设置中的存放文件夹。目录创建、文件校验与分析就绪分别记录。")}</p><code class="dataset-path">${esc(m.directory)}/</code><div class="dataset-asset-actions">${controls}${discovery}</div><p class="muted small">${t("GEO 发现的是整个系列的候选文件。请核对样本范围与文件内容；压缩包不会自动认定为表达矩阵。其他仓库可手动登记文件链接。")}</p>${rows ? `<details open class="dataset-file-list"><summary>${t("文件清单")} (${m.assets.length})</summary>${rows}</details>` : `<p class="muted">${t("尚未登记数据文件。")}</p>`}<p class="muted small">${t("校验仅确认文件可读并记录 SHA-256，不代表矩阵、注释和序列已通过分析验证。")}</p></section>`
    + `<section class="detail-section"><div class="section-title"><h3>${t("样本信息")} (${m.plan.samples.length})</h3>${write ? action("dataset-sample-edit",id,t("添加样本")):""}</div>${samples || `<p class="muted">${t("尚未整理样本。")}</p>`}</section>`
    + `<section class="detail-section"><div class="section-title"><h3>${t("参考序列与版本")}</h3>${write ? action("dataset-reference-edit",id,t("添加参考版本")):""}</div>${badge(referenceComplete(m) ? t("参考序列已匹配"):t("参考序列待匹配"),referenceComplete(m) ? "green":"amber")}<p class="muted small">${t("按物种、组装和注释版本登记参考；PEP、CDS 与映射文件通过版本标识关联，可共用 references/ 下的文件。非编码基因的缺失原因请记入匹配依据。")}</p>${references || `<p class="muted">${t("尚未登记参考版本。")}</p>`}</section>`;
}

function showForm(app: App,title: string,body: string,formID: string,id: string,key = "",revision = 0): void {
  app.showModal(title,`<form id="${formID}" data-dataset="${esc(id)}" data-key="${esc(key)}" data-revision="${revision}"><div class="form-grid">${body}</div><p id="form-error" class="form-error" role="alert"></p></form>`,`${cancelButton}<button class="btn btn-primary" type="submit" form="${formID}">${t("保存")}</button>`);
}

function editAsset(app: App,id: string,key: string): void {
  const m=manifest(app,id);if(!m)return;
  const a=m.assets.find(v=>v.id===key);
  showForm(app,t("登记数据文件"),field(t("文件名"),"title",a?.title??"","",true)
    + `<label class="form-field"><span>${t("文件类型")}</span><select class="input" name="kind">${options(ASSET_KINDS,a?.kind??"other")}</select></label>`
    + field(t("来源文件地址"),"url",a?.url??"","https://…",false,"text",true)
    + field(t("本地相对路径"),"localPath",a?.localPath??"",`${m.directory}/original/…`,false,"text",true)
    + `<p class="muted full-width">${t("请将文件放到存放文件夹内，再填写相对路径并校验。登记链接不会自动下载文件。")}</p>`
    + `<label class="form-field"><span>${t("矩阵数值")}</span><select class="input" name="matrixKind">${options(MATRIX_KINDS,a?.matrixKind??"unknown")}</select></label>`
    + `<label class="form-field"><span>${t("参考版本标识")}</span><select class="input" name="referenceKey">${options({"":t("未关联"),...Object.fromEntries(m.plan.references.map(r=>[r.key,r.key]))},a?.referenceKey??"")}</select></label>`
    + `<fieldset class="form-field full-width"><legend>${t("所属样本（可多选）")}</legend>${m.plan.samples.map(s=>`<label><input type="checkbox" name="sampleIds" value="${esc(s.id)}" ${a?.sampleIds.includes(s.id)?"checked":""}> ${esc(s.id)} · ${esc(s.title)}</label>`).join("") || t("先添加样本，再关联文件。")}</fieldset>`
    + `<label class="form-field full-width"><span>${t("整理备注")}</span><textarea class="textarea" name="notes" rows="3">${esc(a?.notes??"")}</textarea></label>`,"dataset-asset-form",id,key);
}

function editPlan(app: App,id: string,key: string,kind: "sample"|"reference"): void {
  const m=manifest(app,id);if(!m)return;
  if(kind==="sample"){
    const s=m.plan.samples.find(v=>v.id===key);
    showForm(app,t("样本信息"),field(t("样本编号"),"id",s?.id??"","GSM…",true)+field(t("样本名称"),"title",s?.title??"","")+field(t("物种"),"species",s?.species??"","")+field(t("组织"),"tissue",s?.tissue??"","")+field(t("处理条件"),"condition",s?.condition??"","")+field(t("生物学重复"),"replicate",s?.replicate??"","")+field(t("整理备注"),"notes",s?.notes??"","",false,"text",true),"dataset-sample-form",id,key,m.revision);
  }else{
    const r=m.plan.references.find(v=>v.key===key);
    showForm(app,t("参考序列与版本"),field(t("参考版本标识"),"key",r?.key??"","ath-tair10-araport11",true)+field(t("物种"),"species",r?.species??"","",true)+field(t("组装版本"),"assembly",r?.assembly??"","")+field(t("注释版本"),"annotation",r?.annotation??"","")+`<label class="form-field"><span>${t("匹配状态")}</span><select class="input" name="status">${options(MATCH_STATES,r?.status??"unresolved")}</select></label><label class="form-field full-width"><span>${t("匹配依据 / 缺失原因")}</span><textarea class="textarea" name="evidence" rows="4">${esc(r?.evidence??"")}</textarea></label>`,"dataset-reference-form",id,key,m.revision);
  }
}

async function run(app: App,el: HTMLElement,operation:()=>Promise<unknown>,success: string): Promise<void> {
  const button=el as HTMLButtonElement;button.disabled=true;
  try{await operation();await app.reload();app.toast(success);}catch(e){app.toast(errorMessage(e,t("操作失败，请重试。")),true);await app.reload();}finally{button.disabled=false;}
}

export const datasetAssetActions=(app: App):Record<string,(el:HTMLElement,id:string)=>void>=>({
  "dataset-prepare":(el,id)=>{void run(app,el,()=>Library.prepareDatasetDirectory(id),t("存储目录已创建，文件仍需登记与校验。"));},
  "dataset-open-folder":(el,id)=>{void run(app,el,()=>Library.openDatasetDirectory(id),t("已打开数据集目录。"));},
  "dataset-export-manifest":(el,id)=>{void run(app,el,()=>Library.exportDatasetManifest(id),t("已将最新资源清单写入数据集目录的 manifest.json。"));},
  "dataset-discover":(el,id)=>{void run(app,el,async()=>{app.toast(t("正在读取 GEO 系列与样本文件清单…"));const r=await Library.discoverGEOFiles(id,el.dataset.accession??"");app.toast(t("发现 {files} 个文件、{samples} 个样本，新增 {added} 条文件记录。",{files:r.filesFound,samples:r.samplesFound,added:r.filesAdded}));},t("GEO 发现完成，请核对文件内容与样本范围。"));},
  "dataset-asset-edit":(el,id)=>editAsset(app,id,el.dataset.asset??""),
  "dataset-sample-edit":(el,id)=>editPlan(app,id,el.dataset.key??"","sample"),
  "dataset-reference-edit":(el,id)=>editPlan(app,id,el.dataset.key??"","reference"),
  "dataset-asset-verify":(el,id)=>{void run(app,el,()=>Library.verifyDatasetAsset(id,el.dataset.asset??""),t("已读取文件并记录 SHA-256 校验结果。"));},
  "dataset-asset-remove":(el,id)=>{void run(app,el,()=>Library.removeDatasetAsset(id,el.dataset.asset??""),t("文件记录已移除，磁盘文件保留。"));},
});

export async function saveDatasetAssetForm(app: App,form: HTMLFormElement): Promise<void> {
  const id=form.dataset.dataset??"",key=form.dataset.key??"",m=manifest(app,id);if(!m)return;
  const data=new FormData(form), get=(name:string)=>text(data,name);
  try{
    await Library.saveDatasetAsset({id:key,datasetId:id,title:get("title"),kind:get("kind"),url:get("url"),localPath:get("localPath"),matrixKind:get("matrixKind"),referenceKey:get("referenceKey"),notes:get("notes"),sampleIds:data.getAll("sampleIds").map(String),sizeBytes:0,sha256:"",checkedAt:"",localState:"none"});
    app.closeModal();await app.reload();
  }catch(e){setFormError(errorMessage(e,t("保存失败。")));}
}

export async function saveDatasetPlanForm(app: App,form: HTMLFormElement): Promise<void> {
  const id=form.dataset.dataset??"",key=form.dataset.key??"",m=manifest(app,id);if(!m)return;
  const data=new FormData(form),get=(name:string)=>text(data,name),plan=structuredClone(m.plan);
  if(form.getAttribute("id")==="dataset-sample-form"){
    const s:DatasetSample={id:get("id"),title:get("title"),species:get("species"),tissue:get("tissue"),condition:get("condition"),replicate:get("replicate"),notes:get("notes")};
    const i=plan.samples.findIndex(v=>v.id===key);if(i<0)plan.samples.push(s);else plan.samples[i]=s;
  }else{
    const r:DatasetReference={key:get("key"),species:get("species"),assembly:get("assembly"),annotation:get("annotation"),status:get("status"),evidence:get("evidence")};
    const i=plan.references.findIndex(v=>v.key===key);if(i<0)plan.references.push(r);else plan.references[i]=r;
  }
  try{await Library.saveDatasetPlan(id,Number(form.dataset.revision),plan);app.closeModal();await app.reload();}catch(e){setFormError(errorMessage(e,t("保存失败。")));}
}
