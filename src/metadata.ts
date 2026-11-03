import { speciesText } from "./species.ts";
import type { App } from "./app.ts";
import { Library, type MetadataClaim, type MetadataProposal, type Paper } from "./mygo.ts";
import { arr, clone, esc, repositoryLabel } from "./model.ts";
import { configured, modelFor } from "./models.ts";
import { enqueue } from "./store.ts";
import { $, I, errorMessage } from "./ui.ts";
import { t, tv } from "./i18n.ts";

const labels: Record<string, string> = {
  species: t("研究物种"), organs: t("组织 / 部位"), modalities: t("测序类型"),
  platforms: t("测序平台"), articleTypes: t("文献类型"), dataset: t("关联数据集"),
};
type Field = "species" | "organs" | "modalities" | "platforms" | "articleTypes";
const fields: Field[] = ["species", "organs", "modalities", "platforms", "articleTypes"];
const runs = new WeakMap<App, Set<string>>();
const drafts = new WeakMap<HTMLFormElement, { before: Paper; proposal: MetadataProposal }>();

export function metadataButton(app: App, p: Paper): string {
  if (!app.env.desktop || !app.env.real) return "";
  const modelReady = configured(modelFor(app.env.settings, "chat"));
  const fulltext = app.paperResources(p.id).some(r => r.type === "fulltext" && r.localPath);
  const busy = runs.get(app)?.has(p.id);
  const hint = !modelReady ? t("请先在设置中配置 AI 对话模型。") : !fulltext ? t("请先导入全文。") : t("使用 AI 对话模型读取已保存的全文，生成可核对的填写建议。");
  return `<div class="metadata-toolbar"><button class="btn btn-secondary" data-action="extract-paper-metadata" data-id="${esc(p.id)}" title="${esc(hint)}" ${!modelReady || !fulltext || busy ? "disabled" : ""}>${I("sparkle", 16)} ${t("AI 从全文填写")}</button><span class="muted small">${esc(hint)}</span></div>`;
}

export function metadataValue(claim: MetadataClaim): string {
  if (claim.field === "species") return arr(claim.species).map(s => speciesText(s, true)).join("; ");
  if (claim.field === "dataset") return `${repositoryLabel(claim.repository)} ${claim.accession}`;
  return arr(claim.values).map(tv).join("; ");
}

export function metadataSelectedByDefault(p: Paper, claim: MetadataClaim, linkedRepositories: { repository: string; accession: string }[]): boolean {
  if (claim.field === "dataset") return !linkedRepositories.some(r => r.repository.toLowerCase() === claim.repository.toLowerCase() && r.accession.toLowerCase() === claim.accession.toLowerCase());
  return fields.includes(claim.field as Field) && !p[claim.field as Field]?.length;
}

function reviewHTML(app: App, p: Paper, proposal: MetadataProposal): string {
  const linked = app.paperDatasets(p.id).flatMap(d => arr(d.repositories));
  const rows = arr(proposal.claims).map((claim, index) => {
    const current = claim.field === "dataset" ? app.paperDatasets(p.id).map(d => d.accession || d.title).join("; ")
      : claim.field === "species" ? arr(p.species).map(s => speciesText(s, true)).join("; ")
        : fields.includes(claim.field as Field) ? arr(p[claim.field as Exclude<Field, "species">]).map(tv).join("; ") : "";
    return `<label class="metadata-claim"><span class="metadata-choice"><input type="checkbox" name="claim" value="${index}" ${metadataSelectedByDefault(p, claim, linked) ? "checked" : ""}><strong>${esc(labels[claim.field] ?? claim.field)}</strong></span><span class="muted small">${t("当前：{value}", { value: esc(current || t("待补")) })}</span><strong>${esc(metadataValue(claim))}</strong><blockquote>${esc(claim.excerpt)}${claim.page ? `<span class="muted small evidence-page">${t("PDF 第 {n} 页", { n: claim.page })}</span>` : ""}</blockquote></label>`;
  }).join("");
  return `<p class="muted small">${t("来源全文：{title}", { title: esc(proposal.fileTitle) })}${proposal.extractor ? ` · ${esc(proposal.extractor)}` : ""}</p><p>${t("默认勾选空缺字段；勾选已有字段将替换其内容。数据集编号与关联仍需核实。")}</p>`
    + (proposal.truncated ? `<p class="notice-box">${t("全文较长，本次读取了开头和结尾部分，未覆盖的内容可能遗漏。")}</p>` : "")
    + (proposal.omitted ? `<p class="muted small">${t("已过滤 {n} 条缺少原文依据或格式不完整的建议。", { n: proposal.omitted })}</p>` : "")
    + (rows ? `<form id="metadata-apply-form">${rows}<p id="metadata-error" class="form-error" role="alert"></p></form>` : `<p>${t("未提取到有原文依据的字段。可以检查全文文本层，或手动填写。")}</p>`);
}

export async function extractPaperMetadata(app: App, id: string): Promise<void> {
  const p = app.paperById(id);
  if (!p || !app.env.real || !app.env.desktop) return;
  const pending = runs.get(app) ?? new Set<string>();
  if (pending.has(id)) { app.toast(t("正在读取全文并提取研究信息…")); return; }
  pending.add(id); runs.set(app, pending);
  const before = clone(p);
  app.showModal(t("AI 从全文填写"), `<div id="metadata-review"><p class="muted">${t("正在读取全文并提取研究信息…")}</p></div>`, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button>`, true);
  const root = $("#metadata-review");
  try {
    const proposal = await Library.extractPaperMetadata(id);
    if (!root || $("#metadata-review") !== root || !app.paperById(id)) return;
    if (proposal.paperId !== id) throw new Error(t("提取结果与当前文献不匹配，请重试。"));
    root.innerHTML = reviewHTML(app, before, proposal);
    const form = root.querySelector<HTMLFormElement>("#metadata-apply-form");
    if (form) {
      drafts.set(form, { before, proposal });
      root.closest(".modal")?.querySelector(".modal-footer")?.insertAdjacentHTML("beforeend", `<button class="btn btn-primary" type="submit" form="metadata-apply-form">${t("应用所选建议")}</button>`);
    }
  } catch (error) {
    if (root && $("#metadata-review") === root) root.innerHTML = `<p class="form-error" role="alert">${esc(errorMessage(error, t("全文提取失败，请重试。")))}</p><button class="btn btn-secondary" data-action="extract-paper-metadata" data-id="${esc(id)}">${t("重试")}</button>`;
  } finally { pending.delete(id); }
}

export async function saveMetadata(app: App, form: HTMLFormElement): Promise<void> {
  const draft = drafts.get(form);
  if (!draft || form.dataset.saving === "true") return;
  const selected = new Set(new FormData(form).getAll("claim").map(Number));
  const claims = draft.proposal.claims.filter((_claim, index) => selected.has(index));
  const error = form.querySelector("#metadata-error");
  if (!claims.length) { if (error) error.textContent = t("请至少选择一项建议。"); return; }
  form.dataset.saving = "true";
  const button = form.closest(".modal")?.querySelector<HTMLButtonElement>('[form="metadata-apply-form"]');
  if (button) button.disabled = true;
  try {
    await enqueue(() => Library.applyPaperMetadata(draft.before, { ...draft.proposal, claims }));
    if (document.contains(form)) app.closeModal();
    await app.reload();
    app.toast(t("全文建议已应用，原文依据已保存。"));
  } catch (reason) {
    if (error && document.contains(form)) error.textContent = errorMessage(reason, t("保存失败，请重试。"));
    else app.toast(errorMessage(reason, t("保存失败，请重试。")), true);
  } finally {
    form.dataset.saving = "false";
    if (button) button.disabled = false;
  }
}
