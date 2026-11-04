// The keyboard shortcut sheet, opened with ? or from the top bar. It lists what
// the keydown handler in app.ts and the screening keys in screen.ts respond to.
import type { App } from "./app.ts";
import { esc } from "./model.ts";
import { I } from "./ui.ts";
import { t } from "./i18n.ts";

type Group = [title: string, rows: [keys: string[], what: string][]];

const groups = (): Group[] => [
  [t("全局"), [
    [["/", "⌘ K"], t("搜索文献")],
    [["?"], t("打开快捷键面板")],
    [["Esc"], t("关闭弹窗或抽屉，退出筛选")],
  ]],
  [t("文献列表"), [
    [["↑", "↓"], t("上一行 / 下一行")],
    [["←", "→"], t("上一页 / 下一页")],
    [["Enter"], t("查看详情")],
  ]],
  [t("逐篇筛选"), [
    [["←", "→"], t("上一篇 / 下一篇")],
    [["Y"], t("已纳入")],
    [["N"], t("相关 / 排除")],
    [["U"], t("证据不足")],
    [["P"], t("待判断")],
    [["D"], t("从文献库删除")],
    [["A"], t("采纳 AI 建议")],
    [["S"], t("收藏")],
    [["T"], t("翻译摘要")],
    [["Enter"], t("完整详情")],
  ]],
  [t("AI 助手"), [
    [["⌘ Enter"], t("发送消息")],
  ]],
];

const kbd = (key: string): string => `<kbd>${esc(key)}</kbd>`;

export function shortcutsModal(app: App): void {
  const body = `<div class="shortcut-groups">${groups().map(([title, rows]) =>
    `<section class="shortcut-group"><h3>${esc(title)}</h3><dl>${rows.map(([keys, what]) => `<div class="shortcut-row"><dt>${keys.map(kbd).join(`<span class="shortcut-or">/</span>`)}</dt><dd>${esc(what)}</dd></div>`).join("")}</dl></section>`,
  ).join("")}</div><p class="muted small">${t("Windows 上 ⌘ 对应 Ctrl。")}</p>`;
  app.showModal(t("快捷键"), body, `<button class="btn btn-primary" data-action="close-modal">${I("check", 16)} ${t("完成")}</button>`);
}
