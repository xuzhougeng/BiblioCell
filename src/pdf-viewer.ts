import { getDocument, GlobalWorkerOptions, type PDFDocumentProxy } from "pdfjs-dist/legacy/build/pdf.mjs";
import workerURL from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import { DownloadManager, EventBus, PDFFindController, PDFLinkService, PDFViewer } from "pdfjs-dist/legacy/web/pdf_viewer.mjs";
import "pdfjs-dist/web/pdf_viewer.css";
import "./pdf-viewer.css";
import { t } from "./i18n.ts";

// This document owns its find controller. No native PDF plug-in or window.find
// is involved, so toolbar clicks and Ctrl/Cmd+F stay within this frame.
GlobalWorkerOptions.workerSrc = workerURL;
const file = new URLSearchParams(location.search).get("file") ?? "";
const title = window.frameElement?.getAttribute("title") || file;
const filename = /\.pdf$/i.test(title) ? title : `${title}.pdf`;
document.title = filename;
const source = `${/Windows/.test(navigator.userAgent) ? "http://paperfile.localhost" : "paperfile://localhost"}/${encodeURIComponent(file)}`;
const root = document.createElement("main");
root.innerHTML = `<div class="pdf-toolbar" role="toolbar"></div><div class="pdf-find" hidden></div><p class="pdf-status" role="status"></p><aside class="pdf-outline" hidden></aside><div class="pdf-container"><div class="pdfViewer"></div></div>`;
document.body.append(root);
const toolbar = root.querySelector<HTMLDivElement>(".pdf-toolbar")!;
const findbar = root.querySelector<HTMLDivElement>(".pdf-find")!;
const status = root.querySelector<HTMLParagraphElement>(".pdf-status")!;
const container = root.querySelector<HTMLDivElement>(".pdf-container")!;
const outline = root.querySelector<HTMLElement>(".pdf-outline")!;
const eventBus = new EventBus();
const linkService = new PDFLinkService({ eventBus, externalLinkTarget: 2, externalLinkRel: "noopener noreferrer" });
const findController = new PDFFindController({ eventBus, linkService });
const downloadManager = new DownloadManager();
const viewer = new PDFViewer({ container, eventBus, linkService, findController, downloadManager, imageResourcesPath: "/pdf-assets/web/images/" });
linkService.setViewer(viewer);
let pdf: PDFDocumentProxy | null = null;

function button(parent: HTMLElement, label: string, text: string, action: () => void): HTMLButtonElement {
  const el = document.createElement("button");
  el.type = "button";
  el.title = label;
  el.setAttribute("aria-label", label);
  el.textContent = text;
  el.addEventListener("click", action);
  parent.append(el);
  return el;
}

button(toolbar, t("PDF 目录"), "☰", () => {
  outline.hidden = !outline.hidden;
  root.classList.toggle("with-outline", !outline.hidden);
  viewer.update();
});
button(toolbar, t("上一页"), "‹", () => { if (viewer.currentPageNumber > 1) viewer.currentPageNumber--; });
const pageInput = document.createElement("input");
pageInput.type = "number";
pageInput.min = "1";
pageInput.value = "1";
pageInput.setAttribute("aria-label", t("PDF 页码"));
pageInput.addEventListener("change", () => {
  const page = Math.trunc(Number(pageInput.value));
  if (pdf) viewer.currentPageNumber = Math.max(1, Math.min(pdf.numPages, page || 1));
  pageInput.value = String(viewer.currentPageNumber);
});
toolbar.append(pageInput);
const pages = document.createElement("span");
toolbar.append(pages);
button(toolbar, t("下一页"), "›", () => { if (pdf && viewer.currentPageNumber < pdf.numPages) viewer.currentPageNumber++; });
button(toolbar, t("缩小 PDF"), "−", () => viewer.decreaseScale());
button(toolbar, t("放大 PDF"), "+", () => viewer.increaseScale());
button(toolbar, t("适应页面宽度"), "↔", () => { viewer.currentScaleValue = "page-width"; });
button(toolbar, t("旋转 PDF"), "↻", () => { viewer.pagesRotation = (viewer.pagesRotation + 90) % 360; });
const findButton = button(toolbar, t("搜索 PDF"), "⌕", () => toggleFind(Boolean(findbar.hidden)));
findButton.setAttribute("aria-expanded", "false");
button(toolbar, t("下载 PDF"), "↓", () => {
  if (pdf) void pdf.getData().then(data => downloadManager.downloadData(data, filename, "application/pdf")).catch(showError);
});
const printButton = button(toolbar, t("打印 PDF"), "⎙", () => { void printPDF(); });

const query = document.createElement("input");
query.type = "search";
query.placeholder = t("在 PDF 中查找");
query.setAttribute("aria-label", t("在 PDF 中查找"));
findbar.append(query);
const matches = document.createElement("span");
matches.setAttribute("role", "status");
button(findbar, t("上一个匹配"), "↑", () => search(true, true));
button(findbar, t("下一个匹配"), "↓", () => search(true));
findbar.append(matches);
button(findbar, t("关闭 PDF 搜索"), "×", () => toggleFind(false));

function toggleFind(open: boolean): void {
  findbar.hidden = !open;
  findButton.setAttribute("aria-expanded", String(open));
  root.classList.toggle("with-find", open);
  if (open) { query.focus(); query.select(); if (query.value) search(); }
  else { eventBus.dispatch("findbarclose", { source: findbar }); findButton.focus(); }
}

function search(again = false, backward = false): void {
  eventBus.dispatch("find", { source: query, type: again ? "again" : "", query: query.value, caseSensitive: false, entireWord: false, highlightAll: true, findPrevious: backward, matchDiacritics: false });
}
query.addEventListener("input", () => search());
query.addEventListener("keydown", event => {
  if (event.key === "Enter") { event.preventDefault(); search(true, event.shiftKey); }
});
document.addEventListener("keydown", event => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
    event.preventDefault(); event.stopPropagation(); toggleFind(true);
  } else if (event.key === "Escape" && !findbar.hidden) {
    event.preventDefault(); event.stopPropagation(); toggleFind(false);
  }
});
function updateMatches({ current, total }: { current: number; total: number }): void {
  matches.textContent = query.value ? t("{current} / {total} 个匹配", { current, total }) : "";
}
eventBus.on("updatefindmatchescount", (event: { matchesCount: { current: number; total: number } }) => updateMatches(event.matchesCount));
eventBus.on("updatefindcontrolstate", (event: { matchesCount: { current: number; total: number } }) => updateMatches(event.matchesCount));
eventBus.on("pagechanging", (event: { pageNumber: number }) => { pageInput.value = String(event.pageNumber); });
function followPageHash(): void {
  if (location.hash.length > 1) linkService.setHash(location.hash.slice(1));
}
eventBus.on("pagesinit", () => { viewer.currentScaleValue = "page-width"; followPageHash(); });
window.addEventListener("hashchange", followPageHash);
new ResizeObserver(() => { if (pdf && viewer.currentScaleValue === "page-width") viewer.currentScaleValue = "page-width"; }).observe(container);

function showError(error: unknown): void {
  status.textContent = `${t("无法显示 PDF，请在系统中打开。")} ${error instanceof Error ? error.message : String(error)}`;
}

async function printPDF(): Promise<void> {
  if (!pdf || printButton.disabled) return;
  printButton.disabled = true;
  const printPages = document.createElement("div");
  printPages.className = "pdf-print";
  document.body.append(printPages);
  try {
    for (let i = 1; i <= pdf.numPages; i++) {
      const page = await pdf.getPage(i);
      const viewport = page.getViewport({ scale: 1.5, rotation: viewer.pagesRotation });
      const canvas = document.createElement("canvas");
      canvas.width = Math.ceil(viewport.width);
      canvas.height = Math.ceil(viewport.height);
      printPages.append(canvas);
      await page.render({ canvas, viewport }).promise;
    }
    window.print();
  } catch (error) { showError(error); }
  finally { printPages.remove(); printButton.disabled = false; }
}

status.textContent = t("正在加载 PDF…");
const task = getDocument({ url: source, cMapUrl: "/pdf-assets/cmaps/", cMapPacked: true, standardFontDataUrl: "/pdf-assets/standard_fonts/", wasmUrl: "/pdf-assets/wasm/" });
task.onPassword = (update: (password: string) => void) => {
  const password = window.prompt(t("请输入 PDF 密码"));
  if (password === null) { void task.destroy(); status.textContent = t("PDF 已取消加载。"); }
  else update(password);
};
window.addEventListener("pagehide", () => { void task.destroy(); }, { once: true });
try {
  pdf = await task.promise;
  linkService.setDocument(pdf);
  viewer.setDocument(pdf);
  pages.textContent = `/ ${pdf.numPages}`;
  pageInput.max = String(pdf.numPages);
  status.textContent = "";
  const items = await pdf.getOutline();
  function addOutline(items: NonNullable<Awaited<ReturnType<PDFDocumentProxy["getOutline"]>>>, parent: HTMLElement): void {
    const list = document.createElement("ul");
    parent.append(list);
    for (const item of items) {
      const li = document.createElement("li");
      list.append(li);
      button(li, item.title, item.title, () => { if (item.dest) void linkService.goToDestination(item.dest); });
      if (item.items.length) addOutline(item.items, li);
    }
  }
  if (items?.length) addOutline(items, outline);
  else outline.textContent = t("此 PDF 没有目录。");
} catch (error) { showError(error); }
