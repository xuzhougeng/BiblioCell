// The web server's viewer, in a real DOM: browsing works, every change is
// blocked before it reaches the store, and the admin-only pages stay hidden.
import { $, $$, change, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import type { LibraryState } from "../src/model.ts";

const webEnv = (admin: boolean): Env => ({
  desktop: false, backend: true, readOnly: !admin, web: { admin, canLogin: true, version: "test" }, real: true,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" }, quitOnWindowClose: false, sourceCheck: { enabled: false, prompt: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function viewerApp(hash = "#papers") {
  resetDom(hash);
  const state = demoState();
  const saves: LibraryState[] = [];
  const app = new App({ initial: state, store: { load: async () => state, save: async (next) => { saves.push(next); } }, env: webEnv(false) });
  app.start();
  return { app, state, saves };
}

test("a viewer sees the library and the sign-in button, with the admin pages out of reach", () => {
  const { app } = viewerApp("#sources");
  assert.ok(document.body.classList.contains("read-only"));
  assert.equal(app.page, "overview", "the sources page is not a viewer's");
  const auth = $("#web-auth");
  assert.equal(auth.dataset.action, "web-login");
  app.navigate("agent");
  assert.equal(app.page, "overview");
  app.navigate("papers");
  assert.equal(app.page, "papers");
  assert.ok($$(".paper-table tbody tr").length > 0, "papers are listed");
});

test("a viewer's clicks on write controls are refused before the store hears of them", () => {
  const { app, state, saves } = viewerApp();
  const paper = state.papers[0]!;
  const starred = paper.starred;
  $(`[data-action="toggle-star"][data-id="${paper.id}"]`).click();
  assert.equal(paper.starred, starred, "the star did not change");
  assert.equal(saves.length, 0);
  assert.match(text("#toast-root"), /只读访问/);

  app.openPaper(paper.id);
  const screening = $<HTMLSelectElement>('[data-paper-setting="screening"]');
  assert.ok(screening.disabled, "the screening select is disabled");
  change(screening, "excluded");
  assert.equal(paper.screening, state.papers[0]!.screening);
  assert.equal(saves.length, 0);
  $('[data-action="drawer-tab"][data-tab="notes"]').click();
  assert.ok($<HTMLTextAreaElement>("#paper-notes").readOnly, "notes cannot be typed into");
  assert.ok($<HTMLInputElement>("#paper-tags").readOnly, "tags cannot be typed into");

  // Reading and exporting stay open.
  $('[data-action="close-drawer"]').click();
  $('[data-action="export-papers"]').click();
  assert.ok($("#export-form"), "the export modal opened");
  $('[data-action="close-modal"]').click();
  $('[data-action="start-screening"]').click();
  assert.equal(app.screen, null, "screening, which decides, is not a viewer's");
});

test("a viewer's settings page offers to sign in instead of the AI configuration", () => {
  const { app } = viewerApp("#settings");
  assert.equal(app.page, "settings");
  assert.equal(document.querySelector("#settings-ai-section"), null);
  assert.equal(document.querySelector("#settings-storage-path"), null, "the server's folder is not shown");
  assert.equal(document.querySelector("#settings-mcp-token"), null);
  assert.ok($('#app-content [data-action="web-login"]'), "the sign-in button is on the page");
  $('#app-content [data-action="web-login"]').click();
  assert.ok($("#web-login-form input[name=password]"), "the password modal opened");
});

test("an admin on the web edits, sees the AI configuration, and keeps the native-only controls out", () => {
  resetDom("#settings");
  const state = demoState();
  const saves: LibraryState[] = [];
  const app = new App({ initial: state, store: { load: async () => state, save: async (next) => { saves.push(next); } }, env: webEnv(true) });
  app.start();
  assert.ok(!document.body.classList.contains("read-only"));
  assert.equal($("#web-auth").dataset.action, "web-logout");
  assert.ok($("#settings-ai-section"), "AI models can be configured");
  assert.equal(document.querySelector('[data-action="choose-storage"]'), null, "no folder dialog on the web");
  assert.equal(document.querySelector("#demo-mode"), null, "the web is always the real database");
  assert.equal(document.querySelector("#settings-mcp-token"), null, "the web server runs no MCP endpoint");

  app.navigate("sources");
  assert.equal(app.page, "sources");
  app.navigate("papers");
  const paper = state.papers[0]!;
  const starred = paper.starred;
  $(`[data-action="toggle-star"][data-id="${paper.id}"]`).click();
  assert.equal(paper.starred, !starred);
  assert.equal(saves.length, 1);
});
