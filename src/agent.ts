// AI assistant page: a conversation with the built-in agent, which works on
// the real database through the same MCP tools external clients use.
import type { App } from "./app.ts";
import { modelFor, modelName } from "./models.ts";
import { Library, events, type Event as AgentEvent, type Message as AgentMessage } from "./mygo.ts";
import { renderMarkdown } from "./markdown.ts";
import { esc, shortTime } from "./model.ts";
import { $, $$, I, empty, errorMessage } from "./ui.ts";
import { formatElapsed, toolCardHTML, type ToolRun } from "./agent-tools.ts";
import { LOCATED_MESSAGE, frameBase, locateInFrame, paperFileURL, targetHash, type LocatedMessage } from "./pdf-frame.ts";
import { t } from "./i18n.ts";
import { hasBackend, isAdmin } from "./env.ts";

export interface AgentState {
  /** The saved conversation this transcript belongs to; empty until the first completed run saves it. */
  id: string;
  title: string;
  /** The whole transcript, as the Go side returned it last; it goes back whole on the next send. */
  messages: AgentMessage[];
  busy: boolean;
  /** Progress events of the run in flight. */
  live: AgentEvent[];
  /** Messages typed while a run was in flight; each goes out when the previous run ends. */
  queue: string[];
  /** Epoch ms the run in flight started; 0 when idle. */
  startedAt: number;
}

export const emptyAgentState = (): AgentState => ({ id: "", title: "", messages: [], busy: false, live: [], queue: [], startedAt: 0 });

/** AgentPdfState is the PDF open beside the conversation: which file, and the citation it was opened for. */
export interface AgentPdfState {
  paperId: string;
  fileId: string;
  fileTitle: string;
  /** The cited page and sentence; page 0 and an empty quote are allowed. */
  page: number;
  quote: string;
  /** Where the viewer stands: looking for the sentence, found it, or showing the page alone. */
  status: "locating" | "found" | "page";
  /** The page the viewer landed on, once it reported back. */
  shownPage: number;
}

export const AGENT_EXAMPLES = [
  t("库里现在有多少篇文献？哪些还没有判断收录？"),
  t("找出所有拟南芥根的 scRNA-seq 文献，为每篇提一个收录建议并说明理由。"),
  t("检查待判断的文献里哪些是综述，建议不纳入并记录依据。"),
];

const paragraphs = (text: string): string =>
  text.trim().split(/\n{2,}/).map((p) => `<p>${esc(p)}</p>`).join("") || `<p class="muted">${t("（没有文字回复）")}</p>`;

const userRow = (html: string): string => `<div class="agent-row user"><span class="agent-avatar user" aria-hidden="true">${t("我")}</span><div class="agent-turn user">${html}</div></div>`;
const assistantRow = (html: string): string => `<div class="agent-row assistant"><span class="agent-avatar assistant" aria-hidden="true">${I("sparkle", 18)}</span><div class="agent-turn assistant">${html}</div></div>`;

/** transcript renders the saved conversation. Tool calls that follow one another with nothing said in between share one card. */
function transcript(state: AgentState): string {
  const results = new Map<string, AgentMessage>();
  for (const m of state.messages) if (m.role === "tool" && m.toolCallId) results.set(m.toolCallId, m);
  const parts: string[] = [];
  let runs: ToolRun[] = [];
  const flush = (): void => {
    if (runs.length) parts.push(toolCardHTML(runs));
    runs = [];
  };
  for (const m of state.messages) {
    if (m.role === "user") {
      flush();
      parts.push(userRow(paragraphs(m.content)));
    } else if (m.role === "assistant") {
      if (m.thinking?.trim()) {
        flush();
        parts.push(`<div class="agent-row block"><details class="agent-thought"><summary>${I("info", 14)}<span>${t("思考摘要")}</span></summary><div>${renderMarkdown(m.thinking)}</div></details></div>`);
      }
      if (m.content.trim()) {
        flush();
        parts.push(assistantRow(renderMarkdown(m.content)));
      }
      for (const call of m.toolCalls ?? []) {
        const result = results.get(call.id);
        runs.push({ call, result: result ? { content: result.content, isError: result.isError } : undefined });
      }
    }
  }
  flush();
  return parts.join("");
}

/** queueHTML lists what waits to be sent. Each item stays editable until it goes out; without a run in flight it can go out now. */
function queueHTML(state: AgentState): string {
  if (!state.queue.length) return "";
  const items = state.queue.map((prompt, i) =>
    `<li><textarea class="agent-queue-edit" data-queue-index="${i}" rows="2" maxlength="20000" aria-label="${t("编辑第 {n} 条排队消息", { n: i + 1 })}">${esc(prompt)}</textarea>${state.busy ? "" : `<button type="button" class="text-link" data-action="agent-send-queued" data-index="${i}">${t("发送")}</button>`}<button type="button" class="btn-icon" data-action="agent-unqueue" data-index="${i}" aria-label="${t("移出队列")}">${I("x", 14)}</button></li>`,
  ).join("");
  return `<div class="agent-row block"><div class="agent-queue"><span class="agent-queue-label">${I("clock", 14)} ${t("排队中 {n} · 发送前可以继续修改", { n: state.queue.length })}</span><ol>${items}</ol></div></div>`;
}

/** liveHTML shows the run in flight: one card that grows with each tool call, then the queue. */
function liveHTML(state: AgentState): string {
  if (!state.busy) return queueHTML(state);
  const done = new Map<string, AgentEvent>();
  for (const e of state.live) if (e.type === "tool_result" && e.toolCall) done.set(e.toolCall.id, e);
  const runs: ToolRun[] = state.live.filter((e) => e.type === "tool_call" && e.toolCall).map((e) => {
    const call = e.toolCall!;
    const result = done.get(call.id);
    return { call, result: result ? { content: result.output ?? "", isError: result.isError } : undefined };
  });
  const step = state.live.reduce((n, e) => Math.max(n, e.step), 0);
  const card = toolCardHTML(runs, { running: true, step, startedAt: state.startedAt });
  return card + `<div class="agent-row block"><div class="agent-thinking">${I("clock", 15)}<span>${step ? t("模型正在思考（第 {n} 步）…", { n: step }) : t("模型正在思考…")}</span></div></div>` + queueHTML(state);
}

/** tickElapsed keeps the running card's timer moving between events. */
function tickElapsed(): void {
  for (const el of $$("#agent-live .agent-tools-elapsed")) el.textContent = formatElapsed(Date.now() - Number(el.dataset.started));
}

export function renderAgent(app: App): string {
  const { env } = app;
  if (!env.backend) return empty(t("AI 助手需要桌面应用"), t("浏览器预览没有后端，也没有可操作的数据库。请在 BiblioCell 桌面应用中使用。"));
  if (env.readOnly) return empty(t("AI 助手仅限管理员"), t("助手通过 MCP 工具读写正式数据库，并使用服务器上配置的模型；请先登录管理员。"));
  const profile = modelFor(env.settings, "chat");
  const ai = profile?.config ?? { protocol: "anthropic", baseUrl: "", apiKey: "", model: "", maxTokens: 0, maxSteps: 0, effort: "", proxy: "" };
  const protocol = ai.protocol || "anthropic";
  const model = profile ? modelName(ai) : "";
  const configured = Boolean(model);
  const state = app.agent;
  const body = state.messages.length || state.busy
    ? transcript(state)
    : `<div class="agent-welcome"><h3>${t("让助手整理你的文献库")}</h3><p>${t("它通过 MCP 工具读写正式数据库：检索、导入、补充元数据、提出收录建议并记录依据。收录判断和核实仍由你在详情页决定。")}</p><ul>${AGENT_EXAMPLES.map((p) => `<li><button type="button" class="text-link" data-action="agent-example" data-prompt="${esc(p)}">${esc(p)}</button></li>`).join("")}</ul></div>`;
  const notice = !configured
    ? `<p class="agent-notice" role="alert">${I("alert", 15)}<span>${t("还没有配置模型。请到{link}填写协议、地址、API Key 和模型名。", { link: `<a href="#settings" data-page="settings">${t("设置 · AI 模型")}</a>` })}</span></p>`
    : !ai.apiKey ? `<p class="agent-notice">${I("info", 15)}<span>${t("未填写 API Key；本地模型服务通常不需要，云服务会拒绝请求。")}</span></p>` : "";
  const demoNote = env.real ? "" : `<p class="agent-notice">${I("info", 15)}<span>${t("当前是 Demo 模式。助手只操作正式数据库，这里的虚构示例它看不到；要查看它的改动，请在设置中关闭 Demo。")}</span></p>`;
  const endpoint = ai.baseUrl || (protocol === "anthropic" ? "https://api.anthropic.com" : "https://api.openai.com/v1");
  const header = `<header class="agent-chat-header"><span class="agent-avatar assistant" aria-hidden="true">${I("sparkle", 18)}</span><div class="agent-chat-title"><strong>${t("AI 助手 · {model}", { model: esc(profile ? `${profile.name} · ${model}` : t("未配置模型")) })}</strong><span class="muted">${protocol === "anthropic" ? "Anthropic Messages" : "OpenAI Chat Completions"} · ${esc(endpoint)}${ai.effort ? ` · ${t("思考 {effort}", { effort: esc(ai.effort) })}` : ""}</span></div>`
    + `<span class="agent-chat-meta" title="${esc(env.mcp.tools.join("\n"))}">${I("code", 14)} ${t("{n} 个工具", { n: env.mcp.tools.length })}</span><a class="text-link" href="#settings" data-page="settings">${I("settings", 14)} ${t("设置")}</a><button type="button" class="btn btn-secondary" data-action="agent-history" ${state.busy ? "disabled" : ""}>${I("clock", 15)} ${t("历史对话")}</button><button type="button" class="btn btn-secondary" data-action="agent-clear" ${state.busy || !state.messages.length ? "disabled" : ""}>${t("新对话")}</button></header>`;
  // One chat window: a title bar, a scrolling conversation, and the composer pinned at the bottom.
  // A cited PDF opens beside it, in the same page.
  return `<div class="agent-split${app.agentPdf ? " has-pdf" : ""}"><section class="panel agent-chat">${header}${notice}${demoNote}`
    + `<div class="agent-transcript" id="agent-transcript">${body}<div id="agent-live">${liveHTML(state)}</div></div>`
    + `<form class="agent-composer" id="agent-form"><textarea class="textarea" id="agent-input" name="prompt" rows="3" placeholder="${state.busy ? t("运行中也可以继续输入，发送后排队") : t("描述要做的事，例如：把待判断的文献按物种分组列出来")}" maxlength="20000"></textarea>`
    + `<div class="agent-composer-row"><span class="muted">${t("⌘/Ctrl + Enter 发送 · 运行中发送的消息会排队 · 工具调用会直接写入正式数据库")}</span><button type="submit" class="btn btn-primary" ${!configured ? "disabled" : ""}>${I("arrow-right", 15)} ${state.busy ? t("排队") : t("发送")}</button></div></form></section>`
    + agentPdfHTML(app) + `</div>`;
}

// ---- the PDF beside the conversation ----
// A citation opens the paper's PDF in a reading pane to the right of the chat:
// a column of the same page, never a layer over it. The pane is inserted next
// to the chat without repainting it, keeps its viewer across repaints, can be
// dragged wider or narrower, and steps through the reply's citations.

const PDF_WIDTH_KEY = "bibliocell-agent-pdf-width";

/** pdfStatusText says what the viewer is doing with the citation. */
function pdfStatusText(s: AgentPdfState): string {
  if (s.status === "locating") return s.page > 0 ? t("正在第 {n} 页定位原文…", { n: s.page }) : t("正在定位原文…");
  if (s.status === "found") return t("第 {n} 页 · 已高亮原文", { n: s.shownPage });
  if (s.quote) return s.shownPage > 0 ? t("已跳到第 {n} 页，未找到这段原文", { n: s.shownPage }) : t("未找到这段原文");
  return s.shownPage > 0 ? t("第 {n} 页", { n: s.shownPage }) : "";
}

/** citationButtons lists the reply citations in reading order; the pane steps through them. */
const citationButtons = (): HTMLElement[] => $$<HTMLElement>("#agent-transcript .agent-cite");

const sameCitation = (button: HTMLElement, s: AgentPdfState): boolean =>
  button.dataset.paper === s.paperId && Number(button.dataset.page ?? 0) === s.page && (button.dataset.quote ?? "") === s.quote;

/** agentPdfTopHTML is the pane's head: the paper and its actions, then the citation strip. */
function agentPdfTopHTML(app: App, s: AgentPdfState): string {
  const title = app.paperById(s.paperId)?.title ?? s.fileTitle;
  const buttons = citationButtons();
  const index = buttons.findIndex((b) => sameCitation(b, s));
  const position = buttons.length > 1 && index >= 0 ? `<span class="agent-pdf-cite-nav"><button type="button" class="btn-icon" data-action="agent-cite-step" data-step="-1" aria-label="${t("上一处引用")}" title="${t("上一处引用")}" ${index <= 0 ? "disabled" : ""}>${I("chevron-left", 15)}</button><span>${t("引用 {n} / {total}", { n: index + 1, total: buttons.length })}</span><button type="button" class="btn-icon" data-action="agent-cite-step" data-step="1" aria-label="${t("下一处引用")}" title="${t("下一处引用")}" ${index >= buttons.length - 1 ? "disabled" : ""}>${I("chevron-right", 15)}</button></span>` : "";
  const status = pdfStatusText(s);
  const quote = s.quote ? `<q class="agent-pdf-quote" title="${esc(s.quote)}">${esc(s.quote)}</q>` : "";
  return `<div class="agent-pdf-top"><div class="agent-pdf-head"><span class="agent-pdf-title" title="${esc(title)}">${I("file", 15)}<strong>${esc(title)}</strong></span>`
    + `<span class="section-actions"><button type="button" class="text-link" data-action="open-paper" data-id="${esc(s.paperId)}">${t("文献详情")}</button><button type="button" class="text-link" data-action="open-file" data-id="${esc(s.fileId)}">${app.env.desktop ? t("在系统中打开") : t("新标签页打开")}</button><button type="button" class="btn-icon" data-action="agent-pdf-close" aria-label="${t("关闭 PDF")}" title="${t("关闭 PDF")}">${I("x", 16)}</button></span></div>`
    + `<div class="agent-pdf-cite">${position}<span class="agent-pdf-status ${esc(s.status)}" role="status">${esc(status)}</span>${quote}</div></div>`;
}

/** agentPdfHTML renders the divider and the pane, with the viewer loading at the cited page and sentence. */
function agentPdfHTML(app: App): string {
  const s = app.agentPdf;
  if (!s) return "";
  const src = paperFileURL(s.fileId) + targetHash({ page: s.page, quote: s.quote });
  return `<div class="agent-split-handle" role="separator" aria-orientation="vertical" aria-label="${t("调整 PDF 宽度")}"></div>`
    + `<aside class="agent-pdf" id="agent-pdf" aria-label="${t("全文")}">${agentPdfTopHTML(app, s)}<iframe class="agent-pdf-frame" src="${esc(src)}" title="${esc(s.fileTitle)}"></iframe></aside>`;
}

/** updateAgentPdfHead repaints the pane's head without touching the viewer. */
function updateAgentPdfHead(app: App): void {
  const top = $("#agent-pdf .agent-pdf-top");
  if (top && app.agentPdf) top.outerHTML = agentPdfTopHTML(app, app.agentPdf);
}

/** mountAgentPdf puts the pane beside the chat, or replaces it, leaving the conversation untouched. */
function mountAgentPdf(app: App): void {
  const split = $(".agent-split");
  if (!split) return;
  for (const el of $$(".agent-split-handle, #agent-pdf", split)) el.remove();
  split.insertAdjacentHTML("beforeend", agentPdfHTML(app));
  split.classList.toggle("has-pdf", Boolean(app.agentPdf));
  applyPdfWidth(split);
}

/** fulltextPdf is the paper's stored PDF, the file the pane can show. */
const fulltextPdf = (app: App, paperId: string) => app.paperResources(paperId).find((r) => r.type === "fulltext" && r.localPath && /\.pdf$/i.test(r.localPath));

/** openCitation shows a cited passage: the paper's PDF opens beside the conversation on that page
 * with the sentence highlighted. Without a stored PDF the paper's files are shown instead. */
export function openCitation(app: App, paperId: string, page: number, quote: string): void {
  const paper = app.paperById(paperId);
  if (!paper) {
    app.toast(t("库里没有 ID 为 {id} 的文献。", { id: paperId }), true);
    return;
  }
  if (!hasBackend(app.env)) {
    app.openPaper(paperId); // The static preview has no files to show.
    return;
  }
  const file = fulltextPdf(app, paperId);
  if (!file) {
    app.toast(t("《{title}》还没有 PDF 全文，可在详情的“数据与资料”中导入。", { title: paper.title }), true);
    app.openPaperResources(paperId);
    return;
  }
  const previous = app.agentPdf;
  app.agentPdf = { paperId, fileId: file.id, fileTitle: file.title, page, quote, status: quote ? "locating" : "page", shownPage: page };
  const frame = $<HTMLIFrameElement>("#agent-pdf .agent-pdf-frame");
  if (frame && previous?.fileId === file.id && frameBase(frame) === paperFileURL(file.id)) {
    // The same file is open: take the viewer there instead of loading it again.
    updateAgentPdfHead(app);
    locateInFrame(frame, { page, quote });
  } else {
    mountAgentPdf(app);
  }
  markActiveCitation(app);
}

/** stepCitation opens the previous or next citation of the reply. */
function stepCitation(app: App, step: number): void {
  const s = app.agentPdf;
  const buttons = citationButtons();
  if (!s || !buttons.length) return;
  const index = buttons.findIndex((b) => sameCitation(b, s));
  const target = buttons[Math.min(Math.max(index + step, 0), buttons.length - 1)];
  if (!target || target === buttons[index]) return;
  openCitation(app, target.dataset.paper ?? "", Number(target.dataset.page ?? 0), target.dataset.quote ?? "");
  target.scrollIntoView({ block: "nearest" });
}

/** closeAgentPdf puts the conversation back on its own. */
export function closeAgentPdf(app: App): void {
  if (!app.agentPdf) return;
  app.agentPdf = null;
  const split = $(".agent-split");
  if (!split) return;
  for (const el of $$(".agent-split-handle, #agent-pdf", split)) el.remove();
  split.classList.remove("has-pdf");
  markActiveCitation(app);
}

/** markActiveCitation shows which citation the pane is open for. */
function markActiveCitation(app: App): void {
  const s = app.agentPdf;
  for (const button of citationButtons()) button.classList.toggle("is-active", Boolean(s) && sameCitation(button, s!));
}

/** applyPdfWidth gives the pane the width the person last dragged it to. */
function applyPdfWidth(split: HTMLElement): void {
  const width = Number(localStorage.getItem(PDF_WIDTH_KEY));
  if (width > 0) split.style.setProperty("--agent-pdf-width", `${width}px`);
}

/** startPdfResize follows a drag on the divider: the pane takes the width between the pointer and the right edge. */
function startPdfResize(split: HTMLElement, event: PointerEvent): void {
  event.preventDefault();
  split.classList.add("is-resizing");
  const move = (e: PointerEvent): void => {
    const rect = split.getBoundingClientRect();
    const width = Math.round(Math.min(Math.max(rect.right - e.clientX, 320), rect.width - 420));
    split.style.setProperty("--agent-pdf-width", `${width}px`);
  };
  const stop = (): void => {
    split.classList.remove("is-resizing");
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", stop);
    window.removeEventListener("pointercancel", stop);
    const width = parseInt(split.style.getPropertyValue("--agent-pdf-width"), 10);
    if (width > 0) localStorage.setItem(PDF_WIDTH_KEY, String(width));
  };
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", stop);
  window.addEventListener("pointercancel", stop);
}

/** afterAgentRender does what the page needs once its HTML is in place: open on the newest message, mark the open citation. */
export function afterAgentRender(app: App): void {
  const transcript = $("#agent-transcript");
  if (transcript) transcript.scrollTop = transcript.scrollHeight; // Open on the newest message, like a chat window.
  const split = $(".agent-split");
  if (split) applyPdfWidth(split);
  markActiveCitation(app);
  updateAgentPdfHead(app); // The reply may have gained citations; the strip counts them.
}

/** updateAgentContent swaps in the page's new HTML but keeps the PDF viewer when it shows the same
 * file: re-inserting an iframe reloads it, and every finished run repaints the page. */
export function updateAgentContent(content: HTMLElement, html: string): void {
  const frame = content.querySelector<HTMLIFrameElement>(".agent-split .agent-pdf-frame");
  if (frame) {
    const template = document.createElement("template");
    template.innerHTML = html;
    const nextSplit = template.content.querySelector<HTMLElement>(".agent-split");
    const nextFrame = nextSplit?.querySelector<HTMLIFrameElement>(".agent-pdf-frame");
    const split = frame.closest<HTMLElement>(".agent-split");
    const chat = split?.querySelector(".agent-chat");
    const nextChat = nextSplit?.querySelector(".agent-chat");
    const top = split?.querySelector(".agent-pdf-top");
    const nextTop = nextSplit?.querySelector(".agent-pdf-top");
    if (split && nextSplit && nextFrame && frameBase(frame) === frameBase(nextFrame) && chat && nextChat && top && nextTop) {
      chat.replaceWith(nextChat);
      top.replaceWith(nextTop);
      split.className = nextSplit.className;
      frame.title = nextFrame.title;
      return;
    }
  }
  content.innerHTML = html;
}

/** scrollToEnd keeps the newest message in view, the way a chat window does. */
function scrollToEnd(): void {
  const transcript = $("#agent-transcript");
  if (transcript) transcript.scrollTop = transcript.scrollHeight;
}

/** refreshLive repaints only the in-flight block, so the prompt box keeps its text, a queued message being edited keeps its cursor, and cards the user opened or closed stay that way. */
function refreshLive(app: App): void {
  const live = $("#agent-live");
  if (!live) return;
  const active = document.activeElement;
  const editing = active instanceof HTMLTextAreaElement && active.dataset.queueIndex !== undefined
    ? { index: active.dataset.queueIndex, start: active.selectionStart, end: active.selectionEnd }
    : null;
  const folds = new Map<string, boolean>();
  for (const d of $$<HTMLDetailsElement>("details[data-key]", live)) folds.set(d.dataset.key ?? "", d.open);
  live.innerHTML = liveHTML(app.agent);
  for (const d of $$<HTMLDetailsElement>("details[data-key]", live)) {
    const open = folds.get(d.dataset.key ?? "");
    if (open !== undefined) d.open = open;
  }
  for (const list of $$(".agent-tools-list", live)) list.scrollTop = list.scrollHeight; // The newest call stays in view.
  if (editing) {
    const again = $<HTMLTextAreaElement>(`.agent-queue-edit[data-queue-index="${editing.index}"]`, live);
    if (again) {
      again.focus();
      again.setSelectionRange(editing.start, editing.end);
    }
  } else {
    scrollToEnd();
  }
}

/** rerender repaints the page but keeps the draft and the cursor in the prompt box. */
function rerender(app: App, draft?: string, focus?: boolean): void {
  const before = $<HTMLTextAreaElement>("#agent-input");
  const text = draft ?? before?.value ?? "";
  const keepFocus = focus ?? (!before || document.activeElement === before || document.activeElement === document.body);
  app.render();
  const input = $<HTMLTextAreaElement>("#agent-input");
  if (!input) return;
  input.value = text;
  if (keepFocus) input.focus();
}

export function sendAgentMessage(app: App, form: HTMLFormElement): void {
  const state = app.agent;
  if (!isAdmin(app.env)) return;
  const input = $<HTMLTextAreaElement>("#agent-input", form);
  const prompt = input?.value.trim() ?? "";
  if (!prompt) return;
  if (input) input.value = "";
  if (state.busy) {
    state.queue.push(prompt);
    refreshLive(app);
    input?.focus();
    return;
  }
  runAgent(app, prompt);
}

/** runAgent sends one user turn and, when it ends, the next queued one. */
function runAgent(app: App, prompt: string): void {
  const state = app.agent;
  const sent = [...state.messages, { role: "user", content: prompt }];
  state.messages = sent;
  state.busy = true;
  state.live = [];
  state.startedAt = Date.now();
  rerender(app, "", true); // Sending keeps the cursor in the box for the next message.
  scrollToEnd();
  const timer = setInterval(tickElapsed, 1000);
  Library.agentChat(sent).then(async (result) => {
    state.messages = result.messages;
    if (result.stopReason === "max_tokens") app.toast(t("回复在输出上限处被截断，可以让它继续。"), true);
    if (result.stopReason === "refusal") app.toast(t("模型拒绝了这次请求。"), true);
    await saveConversation(app);
    return true;
  }).catch((error: unknown) => {
    // Keep the transcript as it was and hand the text back for a retry; queued messages wait.
    state.messages = sent.slice(0, -1);
    app.toast(t("助手运行失败：{error}", { error: errorMessage(error, t("请检查模型设置和网络。")) }), true);
    state.busy = false;
    state.live = [];
    state.startedAt = 0;
    rerender(app, prompt, true);
    return false;
  }).then(async (finished) => {
    clearInterval(timer);
    if (!finished) return;
    state.busy = false;
    state.live = [];
    state.startedAt = 0;
    if (app.env.real) await app.reload(); // Tool calls wrote to the database; reload renders too.
    rerender(app);
    scrollToEnd();
    // Blank edits are dropped rather than sent.
    let next = state.queue.shift();
    while (next !== undefined && !next.trim()) next = state.queue.shift();
    if (next !== undefined) runAgent(app, next.trim());
  });
}

/** saveConversation keeps the transcript in the database after each completed run, so it shows up in 历史对话. */
async function saveConversation(app: App): Promise<void> {
  const state = app.agent;
  if (!state.messages.length) return;
  try {
    const saved = await Library.saveAgentConversation(state.id, state.title, state.messages);
    state.id = saved.id;
    state.title = saved.title;
  } catch (error: unknown) {
    app.toast(t("对话未能保存到历史：{error}", { error: errorMessage(error, t("请重试。")) }), true);
  }
}

/** agentHistoryModal lists saved conversations to reopen or delete. */
export function agentHistoryModal(app: App): void {
  if (!isAdmin(app.env)) return;
  Library.listAgentConversations().then((list) => {
    const items = list.map((c) => {
      const current = c.id === app.agent.id;
      return `<li class="agent-history-item${current ? " current" : ""}"><button type="button" class="agent-history-open" data-action="agent-load" data-id="${esc(c.id)}"><strong>${esc(c.title)}</strong><span class="muted">${esc(shortTime(c.updatedAt))} · ${t("{n} 条消息", { n: c.messageCount })}${current ? ` · ${t("当前对话")}` : ""}</span></button><button type="button" class="btn-icon" data-action="agent-delete-conversation" data-id="${esc(c.id)}" aria-label="${t("删除这段对话")}" title="${t("删除")}">${I("trash", 16)}</button></li>`;
    }).join("");
    const body = list.length
      ? `<p class="muted">${t("对话在每轮回复结束后自动保存，随存放文件夹迁移。")}</p><ul class="agent-history">${items}</ul>`
      : `<p class="muted">${t("还没有保存的对话。发送第一条消息并收到回复后，对话会出现在这里。")}</p>`;
    app.showModal(t("历史对话"), body, `<button class="btn btn-secondary" data-action="close-modal">${t("关闭")}</button>`, true);
  }).catch((error: unknown) => {
    app.toast(t("读取历史对话失败：{error}", { error: errorMessage(error, t("请重试。")) }), true);
  });
}

/** openAgent goes to the assistant page, with a prompt ready to send when given. */
export function openAgent(app: App, prompt = ""): void {
  app.navigate("agent");
  const input = $<HTMLTextAreaElement>("#agent-input");
  if (!input) return;
  if (prompt) input.value = prompt;
  input.focus();
}

export const agentActions = (app: App): Record<string, (el: HTMLElement) => void> => ({
  "agent-open": (el) => openAgent(app, el.dataset.prompt ?? ""),
  "agent-clear": () => {
    if (app.agent.busy) return;
    app.agent = emptyAgentState(); // The previous conversation is already saved in history.
    app.render();
    $<HTMLTextAreaElement>("#agent-input")?.focus();
  },
  "agent-history": () => agentHistoryModal(app),
  "agent-load": (el) => {
    if (app.agent.busy) return;
    const id = el.dataset.id ?? "";
    Library.getAgentConversation(id).then((c) => {
      app.agent = { ...emptyAgentState(), id: c.id, title: c.title, messages: c.messages };
      app.closeModal();
      app.navigate("agent");
      $<HTMLTextAreaElement>("#agent-input")?.focus();
    }).catch((error: unknown) => {
      app.toast(t("打开对话失败：{error}", { error: errorMessage(error, t("它可能已被删除。")) }), true);
    });
  },
  "agent-delete-conversation": (el) => {
    const id = el.dataset.id ?? "";
    Library.deleteAgentConversation(id).then(() => {
      if (app.agent.id === id) app.agent.id = ""; // The open transcript stays; its next save creates a new record.
      app.toast(t("已删除这段对话。"));
      agentHistoryModal(app);
    }).catch((error: unknown) => {
      app.toast(t("删除失败：{error}", { error: errorMessage(error, t("请重试。")) }), true);
    });
  },
  "agent-cite": (el) => openCitation(app, el.dataset.paper ?? "", Number(el.dataset.page ?? 0), el.dataset.quote ?? ""),
  "agent-cite-step": (el) => stepCitation(app, Number(el.dataset.step) || 1),
  "agent-pdf-close": () => closeAgentPdf(app),
  "agent-example": (el) => {
    const input = $<HTMLTextAreaElement>("#agent-input");
    if (!input) return;
    input.value = el.dataset.prompt ?? "";
    input.focus();
  },
  "agent-unqueue": (el) => {
    app.agent.queue.splice(Number(el.dataset.index), 1);
    refreshLive(app);
    $<HTMLTextAreaElement>("#agent-input")?.focus();
  },
  "agent-send-queued": (el) => {
    if (app.agent.busy) return;
    const [prompt] = app.agent.queue.splice(Number(el.dataset.index), 1);
    if (prompt?.trim()) runAgent(app, prompt.trim());
    else refreshLive(app);
  },
});

/** watchAgent shows each tool call of the run in flight, and where the PDF viewer landed for a citation. */
export function watchAgent(app: App): void {
  events.agentProgress.on((event) => {
    if (!app.agent.busy) return;
    app.agent.live.push(event);
    refreshLive(app);
  });
  window.addEventListener("message", (event) => {
    const data: unknown = event.data;
    if (!data || typeof data !== "object" || (data as { type?: unknown }).type !== LOCATED_MESSAGE) return;
    const frame = $<HTMLIFrameElement>("#agent-pdf .agent-pdf-frame");
    const s = app.agentPdf;
    if (!frame || !s || event.source !== frame.contentWindow) return;
    const { page, found } = data as LocatedMessage;
    s.status = found ? "found" : "page";
    s.shownPage = Number(page) || 0;
    updateAgentPdfHead(app);
  });
  document.addEventListener("pointerdown", (event) => {
    const handle = event.target instanceof Element ? event.target.closest<HTMLElement>(".agent-split-handle") : null;
    const split = handle?.closest<HTMLElement>(".agent-split");
    if (handle && split && event.button === 0) startPdfResize(split, event);
  });
}
