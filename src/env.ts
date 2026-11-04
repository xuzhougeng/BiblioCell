// What the runtime allows, derived from Env. The desktop window, the web
// server's admin and the web server's viewer answer these differently, and
// every feature asks here instead of testing env.desktop itself.
import type { Env } from "./app.ts";
import type { Page } from "./model.ts";

/** hasBackend: a real database answers reads, in the desktop app or on the web. */
export const hasBackend = (env: Env): boolean => env.backend && env.real;
/** isAdmin: settings may be changed and the assistant run: the desktop app, or the web server after signing in. */
export const isAdmin = (env: Env): boolean => env.backend && !env.readOnly;
/** canWrite: the real database may be changed from here. */
export const canWrite = (env: Env): boolean => isAdmin(env) && env.real;

/** Pages only an admin sees on the web: database updates, and the assistant that writes through MCP. */
export const ADMIN_PAGES: ReadonlySet<Page> = new Set<Page>(["sources", "agent"]);
export const pageAllowed = (env: Env, page: Page): boolean => !env.readOnly || !ADMIN_PAGES.has(page);

/** VIEWER_ACTIONS are what a viewer may do: navigate, filter, read details, export. Everything else changes records or settings. */
export const VIEWER_ACTIONS: ReadonlySet<string> = new Set([
  "open-paper", "open-dataset", "close-drawer", "close-modal", "drawer-tab", "supplement-fulltext", "open-file",
  "paper-columns", "reset-paper-columns", "export-papers", "export-datasets", "export-report", "export-json", "bulk-export", "bulk-clear",
  "clear-filter", "clear-filters", "table-page", "datasets-view", "sort-column", "all-papers", "starred-papers", "review-papers",
  "all-datasets", "review-datasets", "task-filter", "shortcuts-help", "review-papers-nav", "stat-nav", "overview-filter",
  "calibration-open", "calibration-refresh", "calibration-clear", "web-login", "web-logout",
]);
/** VIEWER_FORMS are the forms a viewer may submit. */
export const VIEWER_FORMS: ReadonlySet<string> = new Set(["export-form", "web-login-form"]);
