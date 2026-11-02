// Static chrome from index.html: icons, sidebar collapse, mobile navigation, font scale.
import { LOCALES, locale, setLocale, t } from "./i18n.ts";
import { icon } from "./icons.ts";

export function applyFontSize(value: number): void {
  const fontSize = Math.min(20, Math.max(12, Math.round(value) || 15));
  document.documentElement.style.setProperty("--font-scale", String(fontSize / 15));
}

export function initShell(): void {
  document.querySelectorAll<HTMLElement>("[data-icon]").forEach((el) => {
    el.innerHTML = icon(el.dataset.icon ?? "");
  });
  document.querySelectorAll<HTMLElement>(".sidebar .nav-item[data-page]").forEach((el) => {
    const label = el.querySelector("span:not([data-icon])");
    if (label?.textContent) el.title = label.textContent;
  });

  const collapse = document.getElementById("sidebar-collapse");
  let collapsed = false;
  try {
    collapsed = localStorage.getItem("bibliocell-sidebar-collapsed") === "true";
  } catch {
    collapsed = false;
  }
  const applySidebar = () => {
    document.body.classList.toggle("sidebar-collapsed", collapsed);
    if (!collapse) return;
    collapse.setAttribute("aria-expanded", String(!collapsed));
    collapse.setAttribute("aria-label", collapsed ? t("展开侧栏") : t("折叠侧栏"));
    collapse.title = collapsed ? t("展开侧栏") : t("折叠侧栏");
  };
  applySidebar();
  collapse?.addEventListener("click", () => {
    collapsed = !collapsed;
    try {
      localStorage.setItem("bibliocell-sidebar-collapsed", String(collapsed));
    } catch {
      // Collapse state is a convenience; losing it is fine.
    }
    applySidebar();
  });

  // The toggle names the other language in that language, so it reads right to the person who needs it.
  const toggle = document.getElementById("locale-toggle");
  const other = LOCALES.find(([code]) => code !== locale);
  if (toggle && other) {
    toggle.textContent = other[0] === "en" ? "EN" : "中"; // i18n-ignore: names the other language
    toggle.title = other[1];
    toggle.setAttribute("aria-label", other[1]);
    toggle.setAttribute("lang", other[0]);
    toggle.addEventListener("click", () => setLocale(other[0]));
  }
  const menu = document.getElementById("mobile-menu");
  menu?.addEventListener("click", () => {
    const open = document.body.classList.toggle("nav-open");
    menu.setAttribute("aria-expanded", String(open));
  });
  // On phones the open sidebar covers the page; a tap anywhere outside it closes it again.
  document.addEventListener("click", (e) => {
    if (!document.body.classList.contains("nav-open")) return;
    const target = e.target as Element | null;
    if (target?.closest(".sidebar, #mobile-menu")) return;
    document.body.classList.remove("nav-open");
    menu?.setAttribute("aria-expanded", "false");
  });
}
