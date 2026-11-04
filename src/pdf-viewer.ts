import { isMyGo } from "mygo-runtime";
import { getDocument, GlobalWorkerOptions, type PageViewport, type PDFDocumentProxy } from "pdfjs-dist/legacy/build/pdf.mjs";
import workerURL from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import { DownloadManager, EventBus, PDFFindController, PDFLinkService, PDFViewer } from "pdfjs-dist/legacy/web/pdf_viewer.mjs";
import "pdfjs-dist/web/pdf_viewer.css";
import "./pdf-viewer.css";
import { t } from "./i18n.ts";
import { KEY_MESSAGE, LOCATED_MESSAGE, LOCATE_MESSAGE, READY_MESSAGE, parseTargetHash, type PdfTarget } from "./pdf-frame.ts";
import { findNeedle, indexPage, pageOrder, quoteNeedles, type MatchSpan, type PageIndex } from "./pdf-locate.ts";

// This document owns its find controller. No native PDF plug-in or window.find
// is involved, so toolbar clicks and Ctrl/Cmd+F stay within this frame.
GlobalWorkerOptions.workerSrc = workerURL;
const file = new URLSearchParams(location.search).get("file") ?? "";
const title = window.frameElement?.getAttribute("title") || file;
const filename = /\.pdf$/i.test(title) ? title : `${title}.pdf`;
document.title = filename;
// In the window the file comes through the paperfile scheme; from the web server it is a plain
// route. A browser test page (tests/) hands over a same-origin URL instead of a file ID.
const source = isMyGo()
  ? `${/Windows/.test(navigator.userAgent) ? "http://paperfile.localhost" : "paperfile://localhost"}/${encodeURIComponent(file)}`
  : /^(\/(?!\/)|blob:)/.test(file) ? file : `/paperfile/${encodeURIComponent(file)}`;
const root = document.createElement("main");
root.innerHTML = `<div class="pdf-toolbar" role="toolbar"></div><div class="pdf-find" hidden></div><p class="pdf-status" role="status"></p><aside class="pdf-outline" hidden></aside><div class="pdf-container"><div class="pdfViewer"></div></div><p class="pdf-note" role="status" hidden></p>`;
document.body.append(root);
const toolbar = root.querySelector<HTMLDivElement>(".pdf-toolbar")!;
const findbar = root.querySelector<HTMLDivElement>(".pdf-find")!;
const status = root.querySelector<HTMLParagraphElement>(".pdf-status")!;
const container = root.querySelector<HTMLDivElement>(".pdf-container")!;
const outline = root.querySelector<HTMLElement>(".pdf-outline")!;
const noteBox = root.querySelector<HTMLParagraphElement>(".pdf-note")!;
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
  } else if (event.key === "Escape" && window.parent !== window) {
    // Nothing here to close: the page around the viewer may have something (a pane, screening).
    event.preventDefault(); window.parent.postMessage({ type: KEY_MESSAGE, key: "Escape" }, "*");
  }
});
function updateMatches({ current, total }: { current: number; total: number }): void {
  matches.textContent = query.value ? t("{current} / {total} 个匹配", { current, total }) : "";
}
eventBus.on("updatefindmatchescount", (event: { matchesCount: { current: number; total: number } }) => updateMatches(event.matchesCount));
eventBus.on("updatefindcontrolstate", (event: { matchesCount: { current: number; total: number } }) => updateMatches(event.matchesCount));
eventBus.on("pagechanging", (event: { pageNumber: number }) => { pageInput.value = String(event.pageNumber); });
function followHash(): void {
  const target = parseTargetHash(location.hash);
  if (target) void locate(target);
  else if (location.hash.length > 1) linkService.setHash(location.hash.slice(1));
}
eventBus.on("pagesinit", () => {
  viewer.currentScaleValue = "page-width";
  if (pending) { const target = pending; pending = null; void locate(target); }
  else followHash();
});
window.addEventListener("hashchange", followHash);
new ResizeObserver(() => { if (pdf && viewer.currentScaleValue === "page-width") viewer.currentScaleValue = "page-width"; }).observe(container);

function showError(error: unknown): void {
  status.textContent = `${t("无法显示 PDF，请在系统中打开。")} ${error instanceof Error ? error.message : String(error)}`;
}

let noteTimer: ReturnType<typeof setTimeout> | undefined;
/** note shows a passing remark at the bottom of the viewer, such as a quote that was not found. */
function note(text: string): void {
  clearTimeout(noteTimer);
  noteBox.textContent = text;
  noteBox.hidden = !text;
  if (text) noteTimer = setTimeout(() => { noteBox.hidden = true; }, 7000);
}

// ---- locating a quoted sentence ----
// The embedding page asks for a page and a sentence, by URL fragment when the
// frame loads and by message afterwards. The sentence is searched in the text
// layer (hinted page first), drawn as a highlight over the page, and scrolled
// to; when it is not found, the page alone is shown and the parent is told.

/** TextPiece is what the match needs from a text item: its string and where it sits on the page. */
interface TextPiece {
  str: string;
  transform: number[];
  width: number;
}
/** PageText is a page's text items with the folded index built over them. */
interface PageText {
  items: TextPiece[];
  index: PageIndex;
}
/** A highlight: rectangles in PDF user space on one page, redrawn whenever the page renders. */
interface Highlight {
  page: number;
  rects: number[][];
}
interface RenderedPage {
  div: HTMLDivElement;
  viewport: PageViewport;
}

/** A target that arrived before the document did; applied at pagesinit. */
let pending: PdfTarget | null = null;
let highlight: Highlight | null = null;
let locateTicket = 0;
const pageTexts = new Map<number, Promise<PageText>>();
/** Pages searched beyond the hinted one, for a very long document. */
const MAX_SCANNED_PAGES = 400;

function pageText(pageNumber: number): Promise<PageText> {
  let cached = pageTexts.get(pageNumber);
  if (!cached) {
    cached = pdf!.getPage(pageNumber).then(async (page) => {
      const content = await page.getTextContent();
      // Marked-content markers carry no text; only the text items take part.
      const items: TextPiece[] = content.items.flatMap((item) => "str" in item ? [{ str: item.str, transform: item.transform, width: item.width }] : []);
      return { items, index: indexPage(items) };
    });
    pageTexts.set(pageNumber, cached);
  }
  return cached;
}

/** quoteRects turns matched spans into rectangles in PDF user space, one per text item. The
 * item transform maps text space (1 unit = the font size) to the page; the item's width is
 * already on the page, so it is taken back to text space before the quoted share is cut. */
function quoteRects(items: TextPiece[], spans: MatchSpan[]): number[][] {
  const rects: number[][] = [];
  for (const span of spans) {
    const item = items[span.item];
    if (!item) continue;
    const length = [...item.str].length || 1;
    const [a = 1, b = 0, c = 0, d = 1, e = 0, f = 0] = item.transform;
    const scale = Math.hypot(a, b) || 1;
    const textWidth = item.width / scale;
    const x0 = textWidth * (span.start / length);
    const x1 = textWidth * (span.end / length);
    if (x1 <= x0) continue;
    // Glyphs sit from about a fifth of the font size below the baseline to four fifths above it.
    const corners = [[x0, -0.2], [x1, -0.2], [x1, 0.8], [x0, 0.8]].map(([x, y]) => [a * x! + c * y! + e, b * x! + d * y! + f]);
    const xs = corners.map((p) => p[0]!);
    const ys = corners.map((p) => p[1]!);
    rects.push([Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys)]);
  }
  return rects;
}

function renderedPage(pageNumber: number): RenderedPage | null {
  const view = viewer.getPageView(pageNumber - 1) as RenderedPage | undefined;
  return view?.div && view.viewport ? view : null;
}

function clearHighlight(): void {
  highlight = null;
  for (const layer of container.querySelectorAll(".pdf-quote-layer")) layer.remove();
}

/** drawHighlight lays the marks over the page as percentages of its size, so CSS zoom keeps them in place. */
function drawHighlight(): void {
  if (!highlight) return;
  const view = renderedPage(highlight.page);
  if (!view) return;
  view.div.querySelector(".pdf-quote-layer")?.remove();
  const layer = document.createElement("div");
  layer.className = "pdf-quote-layer";
  const { viewport } = view;
  for (const rect of highlight.rects) {
    const [x0, y0] = viewport.convertToViewportPoint(rect[0]!, rect[1]!) as [number, number];
    const [x1, y1] = viewport.convertToViewportPoint(rect[2]!, rect[3]!) as [number, number];
    const mark = document.createElement("span");
    mark.style.left = `${(Math.min(x0, x1) / viewport.width) * 100}%`;
    mark.style.top = `${(Math.min(y0, y1) / viewport.height) * 100}%`;
    mark.style.width = `${(Math.abs(x1 - x0) / viewport.width) * 100}%`;
    mark.style.height = `${(Math.abs(y1 - y0) / viewport.height) * 100}%`;
    layer.append(mark);
  }
  view.div.append(layer);
}
eventBus.on("pagerendered", (event: { pageNumber: number }) => { if (highlight?.page === event.pageNumber) drawHighlight(); });

/** showHighlight marks the quote on its page and scrolls the page so the quote sits a little below the top. */
function showHighlight(pageNumber: number, rects: number[][]): void {
  clearHighlight();
  highlight = { page: pageNumber, rects };
  const top = Math.max(...rects.map((r) => r[3]!));
  viewer.scrollPageIntoView({ pageNumber, destArray: [null, { name: "XYZ" }, null, top + 96, null], allowNegativeOffset: true });
  drawHighlight();
}

/** report tells the embedding page where the viewer landed. */
function report(page: number, found: boolean, quote: string): void {
  if (window.parent !== window) window.parent.postMessage({ type: LOCATED_MESSAGE, page, found, quote }, "*");
}

/** locate goes to a page and highlights a sentence on it: the whole sentence on the hinted page or
 * its neighbours first, then anywhere; failing that, its opening words; failing that, the page alone. */
async function locate(target: PdfTarget): Promise<void> {
  if (!pdf) { pending = target; return; }
  const ticket = ++locateTicket;
  const total = pdf.numPages;
  const hint = Math.min(Math.max(Math.trunc(target.page), 0), total);
  const quote = target.quote.trim();
  clearHighlight();
  note("");
  if (!quote) {
    if (hint) viewer.currentPageNumber = hint;
    report(hint, false, quote);
    return;
  }
  const order = pageOrder(hint, total).slice(0, MAX_SCANNED_PAGES);
  try {
    for (const needle of quoteNeedles(quote)) {
      for (const pageNumber of order) {
        const text = await pageText(pageNumber);
        if (ticket !== locateTicket) return; // A newer target replaced this one.
        const spans = findNeedle(text.index, needle, text.items);
        if (!spans) continue;
        const rects = quoteRects(text.items, spans);
        if (!rects.length) continue;
        showHighlight(pageNumber, rects);
        report(pageNumber, true, quote);
        return;
      }
    }
  } catch (error) {
    showError(error);
  }
  if (hint) {
    viewer.currentPageNumber = hint;
    note(t("未在 PDF 文本中找到这段原文，已跳到第 {n} 页。", { n: hint }));
  } else {
    note(t("未在 PDF 文本中找到这段原文。"));
  }
  report(hint, false, quote);
}

window.addEventListener("message", (event) => {
  if (event.source !== window.parent) return;
  const data: unknown = event.data;
  if (!data || typeof data !== "object" || (data as { type?: unknown }).type !== LOCATE_MESSAGE) return;
  const { page, quote } = data as { page?: unknown; quote?: unknown };
  void locate({ page: Number(page) || 0, quote: typeof quote === "string" ? quote : "" });
});
if (window.parent !== window) window.parent.postMessage({ type: READY_MESSAGE }, "*");

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
// In the window the file arrives whole from the app, so range requests gain nothing; on
// Windows, where the scheme is an http URL, they would even break the document (see
// servePaperFile). Over a real web server ranges work and spare reading the whole file.
const task = getDocument({ url: source, disableRange: isMyGo(), cMapUrl: "/pdf-assets/cmaps/", cMapPacked: true, standardFontDataUrl: "/pdf-assets/standard_fonts/", wasmUrl: "/pdf-assets/wasm/" });
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
