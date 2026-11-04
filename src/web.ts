// Web mode's sign-in: a button in the top bar, a password modal, and the
// actions behind them. Signing in or out reloads the page, which then starts
// with the role the server reports.
import type { App } from "./app.ts";
import { esc } from "./model.ts";
import { $, I, errorMessage, setFormError, text } from "./ui.ts";
import { loginWeb, logoutWeb } from "./web-runtime.ts";
import { t } from "./i18n.ts";

/** initWebAuth puts the sign-in or sign-out button in the top bar of a page the web server serves. */
export function initWebAuth(app: App): void {
  const web = app.env.web;
  const topbar = document.querySelector(".topbar-right");
  if (!web || !topbar || document.getElementById("web-auth")) return;
  const button = document.createElement("button");
  button.type = "button";
  button.id = "web-auth";
  button.className = "topbar-link web-auth";
  if (app.env.readOnly) {
    if (!web.canLogin) return; // Nothing to sign in to.
    button.dataset.action = "web-login";
    button.innerHTML = `${I("lock", 16)}<span>${t("管理员登录")}</span>`;
    button.title = t("登录后可以更新数据库和配置 AI 模型");
  } else {
    button.dataset.action = "web-logout";
    button.innerHTML = `${I("unlock", 16)}<span>${t("退出登录")}</span>`;
    button.title = t("当前以管理员身份访问");
  }
  button.setAttribute("aria-label", button.title);
  topbar.prepend(button);
}

/** webLoginModal asks for the admin password. */
export function webLoginModal(app: App): void {
  const body = `<form id="web-login-form"><p class="muted">${t("输入服务器管理员密码。登录后可以运行来源更新、修改记录和配置 AI 模型；其他访问者只能浏览。")}</p>`
    + `<label class="form-field"><span>${t("管理员密码")}</span><input class="input" name="password" type="password" autocomplete="current-password" required maxlength="1000"></label>`
    + `<p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(t("管理员登录"), body, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-primary" type="submit" form="web-login-form">${I("lock", 16)} ${t("登录")}</button>`);
}

/** submitWebLogin sends the password; the page reloads as admin on success. */
export function submitWebLogin(app: App, form: HTMLFormElement): void {
  const password = text(new FormData(form), "password");
  if (!password) return setFormError(t("请输入密码。"));
  const button = $<HTMLButtonElement>('button[form="web-login-form"]');
  if (button) button.disabled = true;
  loginWeb(password).then(() => {
    app.toast(t("已登录，正在重新加载…"));
    location.reload();
  }).catch((error: unknown) => {
    setFormError(errorMessage(error, t("登录失败，请重试。")));
    if (button) button.disabled = false;
  });
}

/** webActions builds the handlers of the sign-in buttons. */
export const webActions = (app: App): Record<string, (el: HTMLElement) => void> => ({
  "web-login": () => webLoginModal(app),
  "web-logout": () => {
    logoutWeb().then(() => location.reload(), (error: unknown) => app.toast(errorMessage(error, t("退出失败，请重试。")), true));
  },
});

/** webViewerNotice is the settings page's word to a viewer: what is hidden, and how to sign in. */
export function webViewerNotice(app: App): string {
  const web = app.env.web;
  if (!web) return "";
  const action = web.canLogin
    ? `<button class="btn btn-primary" type="button" data-action="web-login">${I("lock", 16)} ${t("管理员登录")}</button>`
    : `<p class="muted">${esc(t("服务器没有配置管理员密码，所有访问者都只能浏览。"))}</p>`;
  return `<section class="settings-section"><div class="settings-section-heading"><span class="settings-section-icon">${I("lock", 21)}</span><div><h2>${t("管理员")}</h2><p>${t("数据库更新和 AI 模型 / API 配置只对管理员显示。")}</p></div></div>${action}</section>`;
}
