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
  desktop: true, backend: true, readOnly: false, web: null, real: true,
  settings: { storagePath: "/tmp/library", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" }, quitOnWindowClose: false, sourceCheck: { enabled: false, prompt: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

const judgement = { verdict: "included", reason: "Root scRNA-seq with public data", basis: "fulltext", skipped: false } as Judgement;

test("reopening a paper restores its saved translation without calling the model", async (t: TestContext) => {
  const lookup = t.mock.method(Library, "cachedTranslation", async () => "数据库中的译文");
  const translate = t.mock.method(Library, "translateText", async () => { throw new Error("model must not be called"); });
  const { app, state } = screeningApp();
  const checkReader = reader($<HTMLIFrameElement>(".screen-pdf-frame"));
  await setImmediate();
  assert.equal($("#screen-abstract .abstract-text").textContent, state.papers[0]!.abstract);
  $('[data-action="screen-abstract-toggle"]').click();
  assert.equal(text("#screen-abstract .abstract-text"), "数据库中的译文");
  app.render();
  await setImmediate();
  assert.equal(lookup.mock.callCount(), 1);
  assert.equal(translate.mock.callCount(), 0);
  checkReader("after restoring and displaying cached translation");
  screeningApp();
  await setImmediate();
  $('[data-action="screen-abstract-toggle"]').click();
  assert.equal(text("#screen-abstract .abstract-text"), "数据库中的译文");
  assert.equal(lookup.mock.callCount(), 2);
  assert.equal(translate.mock.callCount(), 0);
});

test("a late cache lookup cannot overwrite a newly requested translation", async (t: TestContext) => {
  const lookup = Promise.withResolvers<string>();
  t.mock.method(Library, "cachedTranslation", () => lookup.promise);
  t.mock.method(Library, "translateText", async () => "新译文");
  const { app, ids } = screeningApp();
  t.mock.method(app, "canTranslate", () => true);
  app.translateInScreening(ids[0]!);
  await setImmediate();
  lookup.resolve("旧译文");
  await setImmediate();
  assert.equal(text("#screen-abstract .abstract-text"), "新译文");
});

test("abstract translation replaces the displayed text and toggles without saving or another request", async (t: TestContext) => {
  const { app, state, ids, saved } = screeningApp();
  t.mock.method(app, "canTranslate", () => true);
  app.render();
  const original = state.papers[0]!.abstract;
  const pending = Promise.withResolvers<string>();
  const translate = t.mock.method(Library, "translateText", () => pending.promise);
  const checkReader = reader($<HTMLIFrameElement>(".screen-pdf-frame"));
  $('[data-action="screen-translate"]').click();
  assert.equal(text("#screen-abstract .abstract-text"), original.replace(/\s+/g, " ").trim());
  assert.equal(text("#screen-abstract [role=status]"), "正在翻译…");
  app.translateInScreening(ids[0]!);
  assert.equal(translate.mock.callCount(), 1);
  pending.resolve("译文 <em>保留为文本</em>\n第二段");
  await setImmediate();
  assert.equal($("#screen-abstract .abstract-text").textContent, "译文 <em>保留为文本</em>\n第二段");
  assert.equal(document.querySelector("#screen-abstract em"), null);
  assert.equal(document.querySelector("#translate-pop"), null);
  assert.equal(state.papers[0]!.abstract, original);
  assert.equal(saved.length, 0);
  assert.equal(text('[data-action="screen-abstract-toggle"]'), "显示原文");
  $('[data-action="screen-abstract-toggle"]').click();
  assert.equal($("#screen-abstract .abstract-text").textContent, original);
  assert.equal(text('[data-action="screen-abstract-toggle"]'), "显示译文");
  $('[data-action="screen-abstract-toggle"]').click();
  app.render();
  assert.match(text("#screen-abstract .abstract-text"), /译文/);
  assert.equal(translate.mock.callCount(), 1);
  checkReader("after translation, toggles and rerender");
});

test("late translations stay with their paper and changed prompts or abstracts invalidate cached text", async (t: TestContext) => {
  const { app, state, ids } = screeningApp();
  t.mock.method(app, "canTranslate", () => true);
  const first = Promise.withResolvers<string>();
  const second = Promise.withResolvers<string>();
  const translate = t.mock.method(Library, "translateText", () => first.promise);
  app.translateInScreening(ids[0]!);
  app.moveScreening(1);
  first.resolve("第一篇译文");
  await setImmediate();
  assert.equal($("#screen-abstract .abstract-text").textContent, state.papers[1]!.abstract);
  app.moveScreening(-1);
  assert.equal(text("#screen-abstract .abstract-text"), "第一篇译文");
  app.env.settings.translationPrompt = "Translate into English.";
  app.render();
  assert.equal(document.querySelector('[data-action="screen-abstract-toggle"]'), null);
  translate.mock.mockImplementation(() => second.promise);
  app.translateInScreening(ids[0]!);
  state.papers[0]!.abstract = "A revised abstract.";
  app.render();
  second.resolve("Stale translation");
  await setImmediate();
  assert.equal(text("#screen-abstract .abstract-text"), "A revised abstract.");
  translate.mock.mockImplementation(async () => "Revised translation");
  app.translateInScreening(ids[0]!);
  await setImmediate();
  assert.equal(text("#screen-abstract .abstract-text"), "Revised translation");
  assert.equal(translate.mock.calls[2]!.arguments[0], "A revised abstract.");
});

test("translation errors retain the original abstract and can be retried", async (t: TestContext) => {
  const { app, state, ids } = screeningApp();
  t.mock.method(app, "canTranslate", () => true);
  const translate = t.mock.method(Library, "translateText", async () => { throw new Error("Temporary failure"); });
  app.translateInScreening(ids[0]!);
  await setImmediate();
  assert.equal($("#screen-abstract .abstract-text").textContent, state.papers[0]!.abstract);
  assert.match(text("#screen-abstract [role=alert]"), /Temporary failure/);
  translate.mock.mockImplementation(async () => "重试成功");
  $('#screen-abstract [data-action="screen-translate"]').click();
  await setImmediate();
  assert.equal(text("#screen-abstract .abstract-text"), "重试成功");
});

test("settings saves the translation prompt and displays the normalized saved value", async (t: TestContext) => {
  const { app } = screeningApp();
  app.env.settings.mcpToken = "t".repeat(64);
  app.env.settings.translationPrompt = "Translate into Simplified Chinese.";
  const save = t.mock.method(Library, "saveSettings", async (config) => ({ settings: { ...config }, storageChanged: false, mcpTokenChanged: false }));
  app.navigate("settings");
  const input = $<HTMLTextAreaElement>("#settings-translation-prompt");
  assert.equal(input.value, "Translate into Simplified Chinese.");
  input.value = "Translate into French. Preserve gene names.";
  $("#settings-form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  await setImmediate();
  assert.equal(save.mock.calls[0]!.arguments[0].translationPrompt, input.value);
  assert.equal(app.env.settings.translationPrompt, input.value);
});

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
  assert.equal(frame.getAttribute("src"), "/pdf-viewer.html?file=pdf-0", "PDF search must use the isolated reader instead of the native plug-in");
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

test("the queue rail lists the run, jumps between papers and tracks decisions without reloading the PDF", () => {
  const { app, ids, state } = screeningApp();
  app.screenQueueOpen = true;
  app.render();
  const check = reader($(".screen-pdf-frame"));
  const items = $$(".screen-queue-item");
  assert.equal(items.length, 2);
  assert.equal(items[0]!.getAttribute("aria-current"), "true");
  assert.ok($(".screen-queue-item.is-current .screen-queue-dot").classList.length === 1, "a pending paper has a plain dot");

  // The AI region sits in the action bar, next to the decision buttons.
  assert.ok($(".screen-actionbar #screen-ai"));

  // Deciding colours the dot but keeps the frame (one paper left in the run, so no move).
  app.screen!.ids = [ids[0]!];
  $("#app-content").focus();
  keydown("y");
  assert.equal(state.papers[0]!.screening, "included");
  assert.ok($(".screen-queue-item.is-current .screen-queue-dot").classList.contains("green"));
  check("after a decision");

  // Jumping from the rail opens that paper with its own PDF; the current item follows.
  app.screen!.ids = ids;
  app.render();
  $$(".screen-queue-item")[1]!.click();
  assert.equal(app.screen?.index, 1);
  assert.equal($(".screen-queue-item[aria-current]").dataset.index, "1");
  assert.match($(".screen-pdf-frame").getAttribute("src") ?? "", /pdf-1$/);
  // jsdom's window is 1024px wide, where the rail is an overlay: picking a paper closes it.
  assert.equal(app.screenQueueOpen, false);
  assert.ok(!$(".screen-queue").classList.contains("is-open"));

  // The toolbar button shows and hides the rail.
  $(".screen-queue-toggle").click();
  assert.equal(app.screenQueueOpen, true);
  assert.ok($(".screen-queue").classList.contains("is-open"));
  $(".screen-queue-head [data-action=screen-queue-toggle]").click();
  assert.ok(!$(".screen-queue").classList.contains("is-open"));
});

test("D deletes the paper on screen from the library after one confirmation, and the run moves on", async (t: TestContext) => {
  const { app, state } = screeningApp();
  const ids = state.papers.slice(0, 3).map((p) => p.id);
  app.screen!.ids = [...ids];
  app.render();
  const removed: string[] = [];
  t.mock.method(Library, "deletePaper", async (id: string) => {
    removed.push(id);
    state.papers = state.papers.filter((p) => p.id !== id); // What the next reload reads back.
  });
  const settle = async () => { for (let i = 0; i < 5; i++) await setImmediate(); };
  const confirm = () => $<HTMLButtonElement>('#modal-root [data-action="confirm-delete-paper"]');

  // D asks first, with the confirm button focused so Enter deletes; Esc backs out untouched.
  $("#app-content").focus();
  keydown("d");
  assert.equal(document.activeElement, confirm());
  keydown("Escape");
  assert.equal(document.querySelector("#modal-root .modal"), null);
  assert.deepEqual(removed, []);

  keydown("d");
  confirm().click();
  await settle();
  assert.deepEqual(removed, [ids[0]]);
  assert.equal(app.paperById(ids[0]!), undefined);
  assert.equal($(".screen-panel").dataset.paperId, ids[1], "the paper after the deleted one is on screen");
  assert.match(text(".screen-progress"), /第 1 \/ 2 篇/);
  assert.match(text(".screen-progress"), /已删除 1 篇/);

  // The button does the same; deleting the last paper lands on the one before it.
  app.goToScreening(1);
  $('[data-action="screen-delete"]').click();
  confirm().click();
  await settle();
  assert.deepEqual(removed, [ids[0], ids[2]]);
  assert.equal($(".screen-panel").dataset.paperId, ids[1]);
  assert.match(text(".screen-progress"), /第 1 \/ 1 篇/);
  assert.match(text(".screen-progress"), /已删除 2 篇/);
});
