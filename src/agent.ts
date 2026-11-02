// AI assistant page: a conversation with the built-in agent, which works on
// the real database through the same MCP tools external clients use.
import type { App } from "./app.ts";
import { Library, events, type Event as AgentEvent, type Message as AgentMessage } from "./mygo.ts";
import { esc } from "./model.ts";
import { $, I, badge, empty, errorMessage } from "./ui.ts";

export interface AgentState {
  /** The whole transcript, as the Go side returned it last; it goes back whole on the next send. */
  messages: AgentMessage[];
  busy: boolean;
  /** Progress events of the run in flight. */
  live: AgentEvent[];
}

export const emptyAgentState = (): AgentState => ({ messages: [], busy: false, live: [] });

export const AGENT_EXAMPLES = [
  "库里现在有多少篇文献？哪些还没有判断收录？",
  "找出所有拟南芥根的 scRNA-seq 文献，为每篇提一个收录建议并说明理由。",
  "检查待判断的文献里哪些是综述，建议排除并记录依据。",
];

const DISPLAY_LIMIT = 4000;

const clip = (text: string): string => (text.length > DISPLAY_LIMIT ? text.slice(0, DISPLAY_LIMIT) + "\n…（已省略）" : text);

const paragraphs = (text: string): string =>
  text.trim().split(/\n{2,}/).map((p) => `<p>${esc(p)}</p>`).join("") || "<p class=\"muted\">（没有文字回复）</p>";

function toolBlock(name: string, args: string, result: AgentMessage | undefined, pending: boolean): string {
  const state = pending ? badge("运行中", "amber") : result?.isError ? badge("失败", "amber") : badge("完成", "green");
  const output = result ? `<pre>${esc(clip(result.content))}</pre>` : "";
  return `<details class="agent-tool"><summary>${I("code", 14)}<span>${esc(name)}</span>${state}</summary><pre>${esc(clip(args))}</pre>${output}</details>`;
}

function transcript(state: AgentState): string {
  const results = new Map<string, AgentMessage>();
  for (const m of state.messages) if (m.role === "tool" && m.toolCallId) results.set(m.toolCallId, m);
  const parts: string[] = [];
  for (const m of state.messages) {
    if (m.role === "user") parts.push(`<div class="agent-turn user">${paragraphs(m.content)}</div>`);
    else if (m.role === "assistant") {
      if (m.content.trim()) parts.push(`<div class="agent-turn assistant">${paragraphs(m.content)}</div>`);
      for (const call of m.toolCalls ?? []) parts.push(toolBlock(call.name, call.arguments, results.get(call.id), false));
    }
  }
  return parts.join("");
}

/** liveHTML shows the run in flight: each tool call as it starts and finishes. */
function liveHTML(state: AgentState): string {
  if (!state.busy) return "";
  const done = new Map<string, AgentEvent>();
  for (const e of state.live) if (e.type === "tool_result" && e.toolCall) done.set(e.toolCall.id, e);
  const calls = state.live.filter((e) => e.type === "tool_call" && e.toolCall);
  const blocks = calls.map((e) => {
    const call = e.toolCall!;
    const result = done.get(call.id);
    return toolBlock(call.name, call.arguments, result ? { role: "tool", content: result.output ?? "", isError: result.isError } : undefined, !result);
  });
  const step = state.live.reduce((n, e) => Math.max(n, e.step), 0);
  return blocks.join("") + `<div class="agent-thinking">${I("clock", 15)}<span>模型正在思考${step ? `（第 ${step} 步）` : ""}…</span></div>`;
}

export function renderAgent(app: App): string {
  const { env } = app;
  if (!env.desktop) return empty("AI 助手需要桌面应用", "浏览器预览没有后端，也没有可操作的数据库。请在 BiblioCell 桌面应用中使用。");
  const ai = env.settings.ai;
  const protocol = ai.protocol || "anthropic";
  const model = ai.model || (protocol === "anthropic" ? "claude-opus-5-5" : "");
  const configured = Boolean(model);
  const state = app.agent;
  const body = state.messages.length || state.busy
    ? transcript(state)
    : `<div class="agent-welcome"><h3>让助手整理你的文献库</h3><p>它通过 MCP 工具读写正式数据库：检索、导入、补充元数据、提出收录建议并记录依据。收录判断和核实仍由你在详情页决定。</p><ul>${AGENT_EXAMPLES.map((p) => `<li><button type="button" class="text-link" data-action="agent-example" data-prompt="${esc(p)}">${esc(p)}</button></li>`).join("")}</ul></div>`;
  const notice = !configured
    ? `<p class="agent-notice" role="alert">${I("alert", 15)}<span>还没有配置模型。请到<a href="#settings" data-page="settings">设置 · AI 模型</a>填写协议、地址、API Key 和模型名。</span></p>`
    : !ai.apiKey ? `<p class="agent-notice">${I("info", 15)}<span>未填写 API Key；本地模型服务通常不需要，云服务会拒绝请求。</span></p>` : "";
  const demoNote = env.real ? "" : `<p class="agent-notice">${I("info", 15)}<span>当前是 Demo 模式。助手只操作正式数据库，这里的虚构示例它看不到；要查看它的改动，请在设置中关闭 Demo。</span></p>`;
  return `<div class="agent-layout"><section class="panel agent-panel">${notice}${demoNote}`
    + `<div class="agent-transcript" id="agent-transcript">${body}<div id="agent-live">${liveHTML(state)}</div></div>`
    + `<form class="agent-composer" id="agent-form"><textarea class="textarea" id="agent-input" name="prompt" rows="3" placeholder="描述要做的事，例如：把待判断的文献按物种分组列出来" maxlength="20000" ${state.busy ? "disabled" : ""}></textarea>`
    + `<div class="agent-composer-row"><span class="muted">⌘/Ctrl + Enter 发送 · 工具调用会直接写入正式数据库</span><div class="agent-composer-actions"><button type="button" class="btn btn-secondary" data-action="agent-clear" ${state.busy || !state.messages.length ? "disabled" : ""}>新对话</button><button type="submit" class="btn btn-primary" ${state.busy || !configured ? "disabled" : ""}>${I("arrow-right", 15)} ${state.busy ? "运行中…" : "发送"}</button></div></div></form></section>`
    + `<aside class="agent-aside"><section class="panel settings-aside-card"><span class="eyebrow">MODEL</span><h2>${esc(model || "未配置")}</h2><p>${protocol === "anthropic" ? "Anthropic Messages 协议" : "OpenAI Chat Completions 协议"}<br>${esc(ai.baseUrl || (protocol === "anthropic" ? "https://api.anthropic.com" : "https://api.openai.com/v1"))}</p><div class="settings-aside-rule"></div><dl><div><dt>工具</dt><dd>${env.mcp.tools.length} 个 MCP 工具</dd></div><div><dt>单次上限</dt><dd>${ai.maxSteps || 20} 步</dd></div><div><dt>API Key</dt><dd>${ai.apiKey ? "已填写" : "未填写"}</dd></div></dl></section>`
    + `<div class="settings-restart-note">${I("code", 17)}<p><strong>和外部 AI 用同一套工具</strong><br>${esc(env.mcp.tools.join(" · "))}</p></div></aside></div>`;
}

/** refreshLive repaints only the in-flight block, so the textarea keeps its text. */
function refreshLive(app: App): void {
  const live = $("#agent-live");
  if (!live) return;
  live.innerHTML = liveHTML(app.agent);
  live.scrollIntoView({ block: "end" });
}

export function sendAgentMessage(app: App, form: HTMLFormElement): void {
  const state = app.agent;
  if (!app.env.desktop || state.busy) return;
  const input = $<HTMLTextAreaElement>("#agent-input", form);
  const prompt = input?.value.trim() ?? "";
  if (!prompt) return;
  const sent = [...state.messages, { role: "user", content: prompt }];
  state.messages = sent;
  state.busy = true;
  state.live = [];
  app.render();
  Library.agentChat(sent).then((result) => {
    state.messages = result.messages;
    if (result.stopReason === "max_tokens") app.toast("回复在输出上限处被截断，可以让它继续。", true);
    if (result.stopReason === "refusal") app.toast("模型拒绝了这次请求。", true);
  }).catch((error: unknown) => {
    // Keep the transcript as it was and hand the text back for a retry.
    state.messages = sent.slice(0, -1);
    app.toast(`助手运行失败：${errorMessage(error, "请检查模型设置和网络。")}`, true);
    state.busy = false;
    app.render();
    const retry = $<HTMLTextAreaElement>("#agent-input");
    if (retry) retry.value = prompt;
    return null;
  }).then((finished) => {
    if (finished === null) return;
    state.busy = false;
    state.live = [];
    app.render();
    $("#agent-live")?.scrollIntoView({ block: "end" });
    if (app.env.real) void app.reload(); // Tool calls wrote to the database.
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
    app.agent = emptyAgentState();
    app.render();
  },
  "agent-example": (el) => {
    const input = $<HTMLTextAreaElement>("#agent-input");
    if (!input) return;
    input.value = el.dataset.prompt ?? "";
    input.focus();
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
