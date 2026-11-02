// Interface language. Chinese is the source language: every string in the code
// is written in Chinese and doubles as its own key, so t() returns it unchanged
// for zh-CN and looks it up in the English dictionary otherwise. The locale is
// fixed at load; switching saves the choice and reloads, like the demo switch.
import { en } from "./locales/en.ts";

export type Locale = "zh-CN" | "en";
export const LOCALES: [Locale, string][] = [["zh-CN", "简体中文"], ["en", "English"]];
export const LOCALE_STORAGE_KEY = "bibliocell-locale";

const isLocale = (value: unknown): value is Locale => value === "zh-CN" || value === "en";

/** detectLocale prefers the saved choice, then the system language; Chinese systems stay Chinese. */
export function detectLocale(saved: string | null, languages: readonly string[]): Locale {
  if (isLocale(saved)) return saved;
  const first = languages[0]?.toLowerCase() ?? "zh";
  return first.startsWith("zh") ? "zh-CN" : "en";
}

function initialLocale(): Locale {
  let saved: string | null = null;
  try {
    saved = globalThis.localStorage?.getItem(LOCALE_STORAGE_KEY) ?? null;
  } catch {
    saved = null;
  }
  // Without a page (the node:test suite) the source language is used; Node has a navigator of its own.
  const languages = typeof document === "undefined" ? ["zh-CN"] : navigator.languages?.length ? navigator.languages : [navigator.language];
  return detectLocale(saved, languages);
}

export const locale: Locale = initialLocale();
const dictionary: Record<string, string> | null = locale === "en" ? en : null;

export type Params = Record<string, string | number>;

/** fill replaces {name} with params.name; braces without a matching param stay as written. */
export const fill = (text: string, params?: Params): string =>
  params ? text.replace(/\{(\w+)\}/g, (all, name: string) => (Object.hasOwn(params, name) ? String(params[name]) : all)) : text;

/** t translates a Chinese source string; a missing translation falls back to the Chinese. */
export const t = (source: string, params?: Params): string => fill(dictionary?.[source] ?? source, params);

/** tv translates a stored data value (article type, modality, source name) when it has a known label. */
export const tv = (value: string): string => dictionary?.[value] ?? value;

/** setLocale remembers the choice and reloads, so module-level labels pick it up. */
export function setLocale(next: string): void {
  if (!isLocale(next) || next === locale) return;
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, next);
  } catch {
    return;
  }
  location.reload();
}

/** listSeparator joins short lists: 、 in Chinese, a comma in English. */
export const listSeparator = locale === "en" ? ", " : "、";

/** applyStaticText translates the chrome written in index.html: text nodes and labelling attributes. */
export function applyStaticText(root: Document = document): void {
  root.documentElement.lang = locale;
  root.title = t(root.title);
  const description = root.querySelector<HTMLMetaElement>('meta[name="description"]');
  if (description) description.content = t(description.content);
  if (!dictionary) return;
  const walker = root.createTreeWalker(root.body, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const raw = node.nodeValue ?? "";
    const trimmed = raw.trim();
    if (trimmed && dictionary[trimmed]) node.nodeValue = raw.replace(trimmed, dictionary[trimmed]);
  }
  for (const attr of ["title", "aria-label", "placeholder"]) {
    root.body.querySelectorAll<HTMLElement>(`[${attr}]`).forEach((el) => el.setAttribute(attr, t(el.getAttribute(attr) ?? "")));
  }
}
