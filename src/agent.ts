// AI assistant page: a conversation with the built-in agent, which works on
// the real database through the same MCP tools external clients use.
import type { App } from "./app.ts";
import { Library, events, type Event as AgentEvent, type Message as AgentMessage } from "./mygo.ts";
import { renderMarkdown } from "./markdown.ts";
import { esc, shortTime } from "./model.ts";
import { $, $$, I, empty, errorMessage } from "./ui.ts";
import { formatElapsed, toolCardHTML, type ToolRun } from "./agent-tools.ts";
import { t } from "./i18n.ts";

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

export const AGENT_EXAMPLES = [
  t("库里现在有多少篇文献？哪些还没有判断收录？"),
  t("找出所有拟南芥根的 scRNA-seq 文献，为每篇提一个收录建议并说明理由。"),
  t("检查待判断的文献里哪些是综述，建议排除并记录依据。"),
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
  if (!env.desktop) return empty(t("AI 助手需要桌面应用"), t("浏览器预览没有后端，也没有可操作的数据库。请在 BiblioCell 桌面应用中使用。"));
  const ai = env.settings.ai;
  const protocol = ai.protocol || "anthropic";
  const model = ai.model || (protocol === "anthropic" ? "claude-opus-5-5" : "");
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
  const header = `<header class="agent-chat-header"><span class="agent-avatar assistant" aria-hidden="true">${I("sparkle", 18)}</span><div class="agent-chat-title"><strong>${t("AI 助手 · {model}", { model: esc(model || t("未配置模型")) })}</strong><span class="muted">${protocol === "anthropic" ? "Anthropic Messages" : "OpenAI Chat Completions"} · ${esc(endpoint)}${ai.effort ? ` · ${t("思考 {effort}", { effort: esc(ai.effort) })}` : ""}</span></div>`
    + `<span class="agent-chat-meta" title="${esc(env.mcp.tools.join("\n"))}">${I("code", 14)} ${t("{n} 个工具", { n: env.mcp.tools.length })}</span><a class="text-link" href="#settings" data-page="settings">${I("settings", 14)} ${t("设置")}</a><button type="button" class="btn btn-secondary" data-action="agent-history" ${state.busy ? "disabled" : ""}>${I("clock", 15)} ${t("历史对话")}</button><button type="button" class="btn btn-secondary" data-action="agent-clear" ${state.busy || !state.messages.length ? "disabled" : ""}>${t("新对话")}</button></header>`;
  // One chat window: a title bar, a scrolling conversation, and the composer pinned at the bottom.
  return `<section class="panel agent-chat">${header}${notice}${demoNote}`
    + `<div class="agent-transcript" id="agent-transcript">${body}<div id="agent-live">${liveHTML(state)}</div></div>`
    + `<form class="agent-composer" id="agent-form"><textarea class="textarea" id="agent-input" name="prompt" rows="3" placeholder="${state.busy ? t("运行中也可以继续输入，发送后排队") : t("描述要做的事，例如：把待判断的文献按物种分组列出来")}" maxlength="20000"></textarea>`
    + `<div class="agent-composer-row"><span class="muted">${t("⌘/Ctrl + Enter 发送 · 运行中发送的消息会排队 · 工具调用会直接写入正式数据库")}</span><button type="submit" class="btn btn-primary" ${!configured ? "disabled" : ""}>${I("arrow-right", 15)} ${state.busy ? t("排队") : t("发送")}</button></div></form></section>`;
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
  if (!app.env.desktop) return;
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
  if (!app.env.desktop) return;
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

/** watchAgent shows each tool call of the run in flight. */
export function watchAgent(app: App): void {
  events.agentProgress.on((event) => {
    if (!app.agent.busy) return;
    app.agent.live.push(event);
    refreshLive(app);
  });
}
