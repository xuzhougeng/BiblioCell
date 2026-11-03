// DOM for interaction tests: a jsdom window installed as the global document,
// so App renders into real elements and events reach its listeners. Import this
// module before any src module (import order is evaluation order), because
// i18n.ts reads the saved locale and `document` when it loads.
//
// Each test calls resetDom() first: every App registers document listeners, so
// a fresh window keeps earlier tests' apps from answering later tests' events.
import { JSDOM } from "jsdom";

const SKELETON = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>BiblioCell</title></head><body>
<header><input id="global-search" type="search"><h1 id="page-title"></h1><p id="page-subtitle"></p><div class="heading-actions"></div></header>
<a data-page="papers" href="#papers">papers</a>
<div id="app-content" tabindex="-1"></div>
<div id="drawer-root"></div><div id="modal-root"></div><div id="toast-root"></div>
</body></html>`;

const COPIED = ["window", "document", "navigator", "location", "history", "localStorage", "sessionStorage", "getComputedStyle", "requestAnimationFrame", "cancelAnimationFrame",
  "Element", "Node", "NodeFilter", "Text", "DocumentFragment", "Event", "CustomEvent", "KeyboardEvent", "MouseEvent", "FocusEvent", "InputEvent", "SubmitEvent", "FormData", "DOMParser", "getSelection", "innerWidth", "innerHeight"];

export let dom: JSDOM;

function define(name: string, value: unknown): void {
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
}

/** resetDom replaces the global window with a fresh one at #papers, in Chinese, without a backend. */
export function resetDom(hash = "#papers"): JSDOM {
  dom?.window.close();
  dom = new JSDOM(SKELETON, { url: `http://localhost/${hash}`, pretendToBeVisual: true });
  const w = dom.window as unknown as Record<string, unknown> & Window;
  for (const name of COPIED) define(name, typeof w[name] === "function" && !/^[A-Z]/.test(name) ? (w[name] as Function).bind(w) : w[name]);
  for (const name of Object.getOwnPropertyNames(w)) if (/^(HTML|SVG)\w*Element$/.test(name)) define(name, w[name]);
  // Layout never happens in jsdom; the app only uses these to keep things in view.
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.localStorage.setItem("bibliocell-locale", "zh-CN"); // Tests read the source strings, like the other suites.
  // The desktop runtime is absent: calls fail unless a test mocks the Library method; events have no sender.
  define("mygo", { call: (method: string) => Promise.reject(new Error(`no backend in tests: ${method}`)), on: () => () => {}, once: () => () => {} });
  return dom;
}

resetDom();

/** keydown sends a key the way the keyboard does: on the focused element, bubbling up to the document. */
export function keydown(key: string, target: EventTarget = document.activeElement ?? document.body, init: KeyboardEventInit = {}): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

/** change sets a form control's value and announces it, as a person picking an option does. */
export function change(el: HTMLInputElement | HTMLSelectElement, value: string): void {
  el.value = value;
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

export const $ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T => {
  const el = root.querySelector<T>(selector);
  if (!el) throw new Error(`not on the page: ${selector}`);
  return el;
};
export const $$ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T[] => [...root.querySelectorAll<T>(selector)];
export const text = (selector: string, root: ParentNode = document): string => $(selector, root).textContent?.replace(/\s+/g, " ").trim() ?? "";
