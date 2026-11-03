// Screening page in a real DOM: the live PDF frame must survive every update
// of the paper beside it (AI suggestion, decisions, metadata edits, refresh),
// and edits made there must show in the list afterwards. Both regressions
// were fixed in d66573a and 9d307d5; these tests keep them fixed.
import { $, $$, keydown, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import { setImmediate } from "node:timers/promises";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { paperFieldModal, savePaperFields } from "../src/paper-fields.ts";
import type { LibraryState } from "../src/model.ts";
import { Library, type Judgement } from "../src/mygo.ts";

const desktopEnv = (): Env => ({
  desktop: true, real: true,
  settings: { storagePath: "/tmp/library", fontSize: 15, batchScreenConcurrency: 3, mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

const judgement = { verdict: "included", reason: "Root scRNA-seq with public data", basis: "fulltext", skipped: false } as Judgement;

/** screeningApp opens a real-mode library with a stored PDF on the first two papers and enters screening on them. */
function screeningApp() {
  resetDom("#papers");
  const state: LibraryState = demoState();
  state.papers[0]!.screening = "pending";
  const ids = state.papers.slice(0, 2).map((p) => p.id);
  state.resources = ids.map((id, i) => ({ id: `pdf-${i}`, paperId: id, datasetId: null, type: "fulltext", title: `paper-${i}.pdf`, localPath: `papers/paper-${i}.pdf`, status: "stored", url: "", isDemo: false, addedAt: "2026-01-01T00:00:00Z" }));
  const saved: LibraryState[] = [];
  const app = new App({ initial: state, store: { load: async () => state, save: async (next) => { saved.push(next); } }, env: desktopEnv() });
  app.start();
  app.screen = { ids, index: 0, ai: null };
  app.render();
  return { app, state, ids, saved };
}

/** reader marks the frame's browsing context, like a viewer's page and scroll position would be. */
function reader(frame: HTMLIFrameElement) {
  // jsdom gives the frame a window but no viewer document, so the reading
  // position lives on the window: a reloaded or replaced frame loses it.
  const context = frame.contentWindow as Window & { readerPage?: number };
  context.readerPage = 7;
  return (label: string) => {
    const current = $<HTMLIFrameElement>(".screen-pdf-frame");
    assert.equal(current, frame, `${label}: the iframe was replaced`);
    assert.equal(current.contentWindow, context, `${label}: the browsing context reloaded`);
    assert.equal((current.contentWindow as typeof context).readerPage, 7, `${label}: reading position lost`);
  };
}

test("an AI suggestion arriving, failing and being adopted never rebuilds the PDF frame", async (t: TestContext) => {
  const { app, ids, saved } = screeningApp();
  const frame = $<HTMLIFrameElement>(".screen-pdf-frame");
  assert.match(frame.getAttribute("src") ?? "", /pdf-0$/);
  const check = reader(frame);

  const pending = Promise.withResolvers<Judgement>();
  t.mock.method(Library, "screenPaper", () => pending.promise);
  // Turning suggestions on redraws the page around the frame and asks for the paper on screen.
  app.enableScreeningAI("plant root single-cell studies");
  assert.ok($("#screen-ai .ai-box").classList.contains("is-loading"));
  check("while loading");

  pending.resolve(judgement);
  await setImmediate();
  assert.match(text("#screen-ai"), /Root scRNA-seq with public data/);
  assert.ok($("#screen-ai [data-action=screen-ai-adopt]"));
  check("after the suggestion arrived");

  // A failed retry stays inside the suggestion box too.
  t.mock.method(Library, "screenPaper", () => Promise.reject(new Error("Model unavailable")));
  app.requestJudgement(ids[0]!, true);
  await setImmediate();
  assert.match(text("#screen-ai"), /Model unavailable/);
  check("after a failure");

  // Adopting records the decision, saves, and moves on to the next paper's own PDF.
  t.mock.method(Library, "screenPaper", () => pending.promise);
  app.requestJudgement(ids[0]!, true);
  await setImmediate();
  $("#screen-ai [data-action=screen-ai-adopt]").click();
  assert.equal(app.paperById(ids[0]!)?.screening, "included");
  assert.equal(app.paperById(ids[0]!)?.screeningReason, "Root scRNA-seq with public data");
  assert.equal(saved.at(-1)?.papers[0]?.screening, "included");
  assert.equal(app.screen?.index, 1);
  assert.match($(".screen-pdf-frame").getAttribute("src") ?? "", /pdf-1$/);
  assert.equal($(".screen-panel").dataset.paperId, ids[1]);
});

test("decisions, stars, a database refresh and attachment renames keep the reading position", async () => {
  const { app, ids, state } = screeningApp();
  const check = reader($(".screen-pdf-frame"));

  // Y/N/U keys and buttons redraw the status row while the PDF stays.
  app.screen!.ids = [ids[0]!]; // one paper, so a decision re-renders instead of moving on
  $("#app-content").focus();
  keydown("u");
  assert.equal(state.papers[0]!.screening, "uncertain");
  assert.ok($(".screen-decision[data-status=uncertain]").classList.contains("is-current"));
  check("after a decision");

  keydown("s");
  assert.equal(state.papers[0]!.starred, false);
  assert.match(text(".screen-extra"), /收藏/);
  check("after the star toggled");

  await app.reload();
  check("after a database refresh");

  state.resources[0]!.title = "Renamed.pdf";
  app.render();
  assert.equal($<HTMLIFrameElement>(".screen-pdf-frame").title, "Renamed.pdf");
  assert.match(text(".screen-pdf-head"), /Renamed\.pdf/);
  check("after the attachment title changed");

  // Hiding the PDF removes it; showing it again loads a new frame, which is expected.
  $("[data-action=screen-pdf-toggle]").click();
  assert.equal(document.querySelector(".screen-pdf-frame"), null);
  assert.ok($("[data-action=screen-pdf-toggle]"));
  $("[data-action=screen-pdf-toggle]").click();
  assert.match($(".screen-pdf-frame").getAttribute("src") ?? "", /pdf-0$/);
});

test("editing metadata in place keeps the PDF and the list shows the change afterwards", () => {
  const { app, ids, saved } = screeningApp();
  const check = reader($(".screen-pdf-frame"));

  for (const [field, input, value] of [["platforms", "platformsCustom", "Fixture platform"], ["modalities", "modalitiesCustom", "Fixture-seq"]] as const) {
    $(`.detail-edit[data-field="${field}"]`).click();
    const form = $<HTMLFormElement>("#paper-fields-form");
    assert.equal(form.dataset.field, field, "the modal opens on the field that was clicked");
    $<HTMLInputElement>(`[name="${input}"]`, form).value = value;
    savePaperFields(app, form);
    assert.equal(document.querySelector("#modal-root .modal"), null, "the modal closes after saving");
    assert.match(text(`.detail-edit[data-field="${field}"]`), new RegExp(value));
    check(`after saving ${field}`);
  }
  const paper = app.paperById(ids[0]!)!;
  assert.ok(paper.platforms.includes("Fixture platform"));
  assert.ok(paper.modalities.includes("Fixture-seq"));
  assert.ok(saved.at(-1)?.papers.find((p) => p.id === ids[0])?.modalities.includes("Fixture-seq"), "the edit was persisted");

  // Species through the modal too, with its label.
  paperFieldModal(app, ids[0]!, "species");
  const form = $<HTMLFormElement>("#paper-fields-form");
  $<HTMLInputElement>('[name="speciesName"]', form).value = "Hordeum vulgare";
  $<HTMLInputElement>('[name="speciesLabel"]', form).value = "Barley";
  savePaperFields(app, form);
  assert.match(text('.detail-edit[data-field="species"]'), /Barley/);
  check("after saving species");

  // Leaving screening lands on the same paper in the table, with the new values in its row.
  keydown("Escape");
  assert.equal(app.screen, null);
  const row = $(`tbody tr[data-id="${ids[0]}"]`);
  assert.equal(document.activeElement, row);
  assert.match(row.textContent ?? "", /Fixture-seq/);
  assert.match(row.textContent ?? "", /Barley/);
  // And the overview counts the new species as well.
  app.navigate("overview");
  assert.match(text(".species-bars"), /Barley/);
});

test("arrow keys move between papers and each paper shows its own PDF", () => {
  const { app, ids } = screeningApp();
  $("#app-content").focus();
  keydown("ArrowRight");
  assert.equal(app.screen?.index, 1);
  assert.equal($(".screen-panel").dataset.paperId, ids[1]);
  assert.match($(".screen-pdf-frame").getAttribute("src") ?? "", /pdf-1$/);
  assert.match(text(".screen-progress"), /第 2 \/ 2 篇/);
  const last = keydown("ArrowRight");
  assert.equal(app.screen?.index, 1);
  assert.equal(last.defaultPrevented, true, "screening swallows its own keys");
  keydown("ArrowLeft");
  assert.equal(app.screen?.index, 0);
  assert.match($(".screen-pdf-frame").getAttribute("src") ?? "", /pdf-0$/);
  // Without a stored PDF the pane offers an upload instead of a frame.
  app.screen!.ids.push(app.state.papers[5]!.id);
  app.screen!.index = 2;
  app.render();
  assert.equal(document.querySelector(".screen-pdf-frame"), null);
  assert.ok($(".screen-pdf-empty [data-action=import-file]"));
  assert.equal($$(".screen-pdf-frame").length, 0);
});
