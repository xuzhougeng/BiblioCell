// The top bar's Update pill, modelled on magpie's: update.go downloads a newer
// release in the background and the pill appears once it is ready, so a click
// only restarts into it. Its × hides it until a newer version is out; the
// staged version still goes in at the next launch.
import type { App } from "./app.ts";
import { Library, events, type UpdateState } from "./mygo.ts";
import { renderMarkdown } from "./markdown.ts";
import { esc } from "./model.ts";
import { I, errorMessage } from "./ui.ts";
import { t } from "./i18n.ts";

export const UPDATE_SKIP_KEY = "bibliocell-update-skip";
const RELEASE_PAGE = "https://github.com/xuzhougeng/BiblioCell/releases/tag/v";

let current: UpdateState | null = null;
// retrying: a click on a failed download, whose progress the pill shows.
let retrying = false;

const skipped = (): string => {
  try { return localStorage.getItem(UPDATE_SKIP_KEY) ?? ""; } catch { return ""; }
};

export interface PillView { label: string; title: string; busy: boolean; failed: boolean }

/** pillView is what the pill shows for u, or null while it stays away. */
export function pillView(u: UpdateState | null, skip: string, pulling: boolean): PillView | null {
  if (!u) return null;
  if (pulling && u.state === "downloading") {
    const p = u.total > 0 ? Math.min(100, Math.floor((u.done / u.total) * 100)) : NaN;
    return { label: Number.isFinite(p) ? t("下载中 {p}%", { p }) : t("下载中…"), title: t("正在下载 v{v}", { v: u.latest }), busy: true, failed: false };
  }
  if (u.latest && u.latest === skip) return null;
  if (u.state === "ready") return { label: t("更新"), title: t("重启以更新到 v{v}", { v: u.latest }), busy: false, failed: false };
  if (u.state === "error") return { label: t("更新失败"), title: t("无法下载 v{v}：{reason}。点击重试。", { v: u.latest, reason: u.error }), busy: false, failed: true };
  return null;
}

function render(): void {
  const wrap = document.querySelector<HTMLElement>(".update-wrap");
  const pill = document.querySelector<HTMLButtonElement>("#update-pill");
  if (!wrap || !pill) return;
  const view = pillView(current, skipped(), retrying);
  wrap.hidden = !view;
  if (!view) return;
  pill.querySelector("span")!.textContent = view.label;
  pill.title = view.title;
  pill.setAttribute("aria-label", view.title);
  pill.classList.toggle("busy", view.busy);
  pill.classList.toggle("failed", view.failed);
}

/** showUpdate takes the Go side's latest word on the update. */
export function showUpdate(u: UpdateState): void {
  current = u;
  if (u.state !== "downloading") retrying = false;
  render();
}

/** initUpdate puts the pill at the start of the top bar, in the desktop app only. */
export function initUpdate(app: App): void {
  const topbar = document.querySelector(".topbar-right");
  if (!app.env.desktop || !topbar || document.getElementById("update-pill")) return;
  const wrap = document.createElement("span");
  wrap.className = "update-wrap";
  wrap.hidden = true;
  wrap.innerHTML = `<button type="button" class="update-pill" id="update-pill" data-action="update-pill">${I("arrowUp", 12)}<span></span></button>`
    + `<button type="button" class="update-hide" data-action="update-skip" title="${t("隐藏，直到有更新的版本")}" aria-label="${t("隐藏，直到有更新的版本")}">${I("x", 9)}</button>`;
  topbar.prepend(wrap);
  events.updateChanged.on(showUpdate);
  void Library.updateStatus().then(showUpdate, () => {});
}

function confirmUpdate(app: App, u: UpdateState): void {
  const notes = u.notes.trim() ? `<div class="update-notes">${renderMarkdown(u.notes)}</div>` : "";
  const body = `<p>${t("新版本 v{v} 已下载好（当前 v{current}），重启后即可使用。", { v: esc(u.latest), current: esc(u.current) })}</p>${notes}`
    + `<p class="muted small"><a class="text-link" href="${RELEASE_PAGE}${encodeURIComponent(u.latest)}" target="_blank" rel="noopener noreferrer">${t("查看发布说明")}</a> · ${t("现在不重启也可以，下次打开时会自动换上新版本。")}</p>`;
  app.showModal(t("更新到 v{v}", { v: u.latest }), body,
    `<button class="btn btn-secondary" data-action="close-modal">${t("稍后")}</button><button class="btn btn-primary" data-action="update-restart">${I("refresh", 16)} ${t("重启并更新")}</button>`);
}

async function restart(app: App, button: HTMLElement): Promise<void> {
  if (button instanceof HTMLButtonElement) button.disabled = true;
  button.textContent = t("正在重启…");
  try {
    await Library.restartToUpdate();
  } catch (error) {
    app.closeModal();
    app.toast(errorMessage(error, t("重启失败，请退出后重新打开 BiblioCell。")), true);
  }
}

/** updateActions handles the pill, its ×, and the restart in the dialog it opens. */
export const updateActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "update-pill": () => {
    if (current?.state === "ready") return confirmUpdate(app, current);
    if (current?.state === "error") {
      retrying = true;
      current = { ...current, state: "downloading", done: 0, total: 0, error: "" };
      render();
      void Library.retryUpdate().catch((error) => {
        retrying = false;
        app.toast(errorMessage(error, t("无法重试更新，请稍后再试。")), true);
      });
    }
  },
  "update-skip": () => {
    if (!current?.latest) return;
    try { localStorage.setItem(UPDATE_SKIP_KEY, current.latest); } catch { /* the pill only stays away until the window reloads */ }
    render();
    app.toast(t("已隐藏 v{v} 的更新提示，下次打开时仍会换上新版本。", { v: current.latest }));
  },
  "update-restart": (el) => void restart(app, el),
});
