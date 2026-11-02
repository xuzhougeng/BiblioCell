// Model profiles: the model services a person configured, and which one each
// AI feature uses. Mirrors settings.ModelFor on the Go side so the interface
// can tell before calling whether a feature has a model at all.
import type { AgentConfig, Config, ModelProfile } from "./mygo.ts";
import { t } from "./i18n.ts";

export type AITask = "chat" | "screen" | "translate";

/** TASKS lists the features a model can be bound to, with what each one does. */
export const TASKS: [AITask, string, string][] = [
  ["chat", t("AI 对话"), t("内置助手，通过 MCP 工具操作正式数据库")],
  ["screen", t("AI 判断"), t("逐篇筛选时按你的要求判断是否符合")],
  ["translate", t("AI 翻译"), t("把划选的摘要或段落翻译成中文")],
];

export const PROTOCOL_LABELS: Record<string, string> = { anthropic: "Anthropic Messages", openai: "OpenAI Chat Completions" };

export const AI_DEFAULTS: Record<string, { baseUrl: string; model: string; hint: string }> = {
  anthropic: { baseUrl: "https://api.anthropic.com", model: "claude-opus-5-5", hint: t("Anthropic Messages 协议：POST {地址}/v1/messages，x-api-key 认证。兼容网关填它的根地址。") },
  openai: { baseUrl: "https://api.openai.com/v1", model: t("例如 gpt-5、deepseek-chat、qwen3"), hint: t("OpenAI Chat Completions 协议：POST {地址}/chat/completions，Bearer 认证。DeepSeek、通义、Ollama、vLLM 等都走这个。") },
};

export const models = (settings: Config): ModelProfile[] => settings.models ?? [];

/** modelFor returns the profile bound to a task, or the first profile when the task has none of its own. */
export function modelFor(settings: Config, task: AITask): ModelProfile | undefined {
  const list = models(settings);
  const id = settings.tasks?.[task] ?? "";
  return (id && list.find((m) => m.id === id)) || list[0];
}

/** modelName is what a profile's model is called on the wire, filling the Anthropic default. */
export const modelName = (config: AgentConfig): string => config.model || ((config.protocol || "anthropic") === "anthropic" ? AI_DEFAULTS.anthropic!.model : "");

export const endpointOf = (config: AgentConfig): string => config.baseUrl || AI_DEFAULTS[config.protocol || "anthropic"]?.baseUrl || "";

/** configured: the profile names a model and either a key or a service address the backend can reach. */
export const configured = (profile: ModelProfile | undefined): boolean =>
  Boolean(profile && modelName(profile.config) && (profile.config.apiKey || profile.config.baseUrl));

/** profileStatus summarises a profile for a badge. */
export const profileStatus = (profile: ModelProfile): [string, string] =>
  !modelName(profile.config) ? [t("未配置"), "gray"] : profile.config.apiKey ? [t("已配置"), "green"] : [t("未填 Key"), "amber"];

/** newProfileId makes a short id that no existing profile uses. */
export function newProfileId(existing: ModelProfile[]): string {
  const taken = new Set(existing.map((m) => m.id));
  for (let i = 0; i < 1000; i++) {
    const id = `m${Date.now().toString(36)}${i ? i : ""}`;
    if (!taken.has(id)) return id;
  }
  return `m${Math.random().toString(36).slice(2)}`;
}
