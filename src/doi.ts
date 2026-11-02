// DOI metadata lookup: fills the paper form from Crossref / Europe PMC, and
// leaves every field editable when the lookup fails.
import type { App } from "./app.ts";
import type { Paper } from "./mygo.ts";
import { ARTICLE_TYPES, normalizeDOI, validDOI } from "./model.ts";
import { Library } from "./mygo.ts";
import { t } from "./i18n.ts";
import { $, errorMessage } from "./ui.ts";
import { paperModal } from "./forms.ts";

type Metadata = Pick<Paper, "title" | "doi" | "pmid" | "authors" | "year" | "journal" | "publicationStatus" | "articleTypes" | "abstract" | "source">;

const stripTags = (value: string): string =>
  new DOMParser().parseFromString(value, "text/html").body.textContent?.replace(/\s+/g, " ").trim() ?? "";

/** crossrefInBrowser serves the web preview, which has no Go backend; Crossref allows CORS. */
async function crossrefInBrowser(doi: string): Promise<Metadata> {
  const response = await fetch(`https://api.crossref.org/works/${encodeURIComponent(doi).replace(/%2F/gi, "/")}`);
  if (response.status === 404) throw new Error(t("没有找到这个 DOI 的书目信息"));
  if (!response.ok) throw new Error(`Crossref HTTP ${response.status}`);
  const m = (await response.json()).message ?? {};
  const authors = (m.author ?? []).map((a: { given?: string; family?: string }) => [a.given, a.family].filter(Boolean).join(" ")).filter(Boolean);
  return {
    title: stripTags(m.title?.[0] ?? ""), doi, pmid: "", authors: authors.join("; "),
    year: m.published?.["date-parts"]?.[0]?.[0] ?? null, journal: m["container-title"]?.[0] ?? "",
    publicationStatus: m.type === "posted-content" ? "preprint" : "published",
    articleTypes: m.type === "review-article" ? ["综述"] : [], abstract: stripTags(m.abstract ?? ""), source: "Crossref", // i18n-ignore
  };
}

export const lookupDOI = (app: App, doi: string): Promise<Metadata> =>
  app.env.real ? Library.lookupDOI(doi) : crossrefInBrowser(doi);

const setStatus = (message: string, error = false): void => {
  const el = $("#doi-status");
  if (!el) return;
  el.textContent = message;
  el.classList.toggle("is-error", error);
};

/** fillFormFromDOI looks up the DOI in the open paper form and writes what it finds into the fields, marking each change for review. */
export async function fillFormFromDOI(app: App): Promise<void> {
  const form = $<HTMLFormElement>("#paper-form");
  const button = $<HTMLButtonElement>("#doi-lookup");
  if (!form || !button || button.disabled) return;
  const input = form.elements.namedItem("doi") as HTMLInputElement;
  const doi = normalizeDOI(input.value);
  if (!doi || !validDOI(doi)) {
    setStatus(t("请先填写 DOI，例如 10.1038/s41586-…"), true);
    input.focus();
    return;
  }
  button.disabled = true;
  form.querySelectorAll(".is-fetched").forEach((el) => el.classList.remove("is-fetched"));
  setStatus(t("正在从 Crossref / Europe PMC 获取…"));
  try {
    const m = await lookupDOI(app, doi);
    if (!document.contains(form)) return;
    let changed = 0;
    const put = (name: string, value: string): void => {
      const el = form.elements.namedItem(name) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
      if (!el || !value || el.value === value) return;
      el.value = value;
      el.closest(".form-field")?.classList.add("is-fetched");
      changed++;
    };
    put("doi", m.doi || doi);
    put("title", m.title);
    put("authors", m.authors);
    put("year", m.year ? String(m.year) : "");
    put("journal", m.journal);
    put("pmid", m.pmid);
    put("abstract", m.abstract);
    if (m.publicationStatus) put("publicationStatus", m.publicationStatus);
    for (const type of m.articleTypes ?? []) {
      const box = form.querySelector<HTMLInputElement>(`input[name="articleTypes"][value="${CSS.escape(type)}"]`);
      if (box && ARTICLE_TYPES.includes(type) && !box.checked) {
        box.checked = true;
        box.closest(".form-field")?.classList.add("is-fetched");
        changed++;
      }
    }
    setStatus(changed
      ? t("已获取，更新了 {n} 项（高亮显示），保存前请核对。", { n: changed })
      : t("已获取，现有信息与来源一致。"));
  } catch (error) {
    if (!document.contains(form)) return;
    setStatus(t("未能自动获取：{reason}。可直接在下方手动补充。", { reason: errorMessage(error, t("请检查网络后重试")) }), true);
    const blank = ["title", "authors", "year", "journal"].map((n) => form.elements.namedItem(n) as HTMLInputElement).find((el) => el && !el.value);
    blank?.focus();
  } finally {
    button.disabled = false;
  }
}

/** doiActions builds the handlers of the DOI fetch buttons in the drawer and the paper form. */
export const doiActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "lookup-doi": () => void fillFormFromDOI(app),
  // From the drawer: open the edit form and fetch at once, so a failure lands on fields ready to fill by hand.
  "fetch-doi": (_el, id) => {
    paperModal(app, id);
    void fillFormFromDOI(app);
  },
});
