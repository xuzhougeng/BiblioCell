// Overview dashboard and the keyboard shortcut sheet, in a real DOM.
import { $, $$, keydown, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";

const previewEnv = (): Env => ({
  desktop: false, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function overviewApp() {
  resetDom("#overview");
  const state = demoState();
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: previewEnv() });
  app.start();
  return { app, state };
}

test("the overview dashboard sums up screening and resumes it on the first undecided paper", () => {
  const { app, state } = overviewApp();
  const undecided = state.papers.filter((p) => p.screening === "pending").length;
  assert.match(text(".dashboard-panel .panel-header p"), new RegExp(`已判断 ${state.papers.length - undecided} / ${state.papers.length} 篇`));
  assert.equal($$(".screen-bar-seg").length, new Set(state.papers.map((p) => p.screening)).size);
  assert.match(text(".dashboard-metric:nth-child(3)"), new RegExp(`待判断\\s*${undecided}`));
  // The assistant card shrank to a single line without the example prompts.
  assert.ok($(".agent-entry.compact"));
  assert.equal(document.querySelector(".agent-entry-prompt"), null);

  $('[data-action="resume-screening"]').click();
  assert.ok(app.screen, "screening opened");
  const first = state.papers.find((p) => p.screening === "pending")!;
  assert.equal($(".screen-panel").dataset.paperId, first.id);
  assert.equal(app.screen!.ids.length, state.papers.length);
});

test("? opens the shortcut sheet unless typing, and the top bar has the same button", () => {
  overviewApp();
  assert.ok($("#shortcuts-help"));
  $("#app-content").focus();
  keydown("?");
  assert.match(text("#modal-root #modal-title"), /快捷键/);
  assert.match(text("#modal-root .shortcut-groups"), /逐篇筛选/);
  keydown("Escape");
  assert.equal(document.querySelector("#modal-root .modal"), null);
  const search = $<HTMLInputElement>("#global-search");
  search.focus();
  const typed = keydown("?", search);
  assert.equal(typed.defaultPrevented, false, "a ? typed into a field is text");
  assert.equal(document.querySelector("#modal-root .modal"), null);
  $("#shortcuts-help").click();
  assert.match(text("#modal-root #modal-title"), /快捷键/);
});
