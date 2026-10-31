// Static chrome from index.html: icons, sidebar collapse, mobile navigation, font scale.
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
    collapse.setAttribute("aria-label", collapsed ? "展开侧栏" : "折叠侧栏");
    collapse.title = collapsed ? "展开侧栏" : "折叠侧栏";
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

  const menu = document.getElementById("mobile-menu");
  menu?.addEventListener("click", () => {
    const open = document.body.classList.toggle("nav-open");
    menu.setAttribute("aria-expanded", String(open));
  });
}
