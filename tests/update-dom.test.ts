// The top bar's Update pill, in a real DOM, against a mocked Go side.
import { $, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { Library, type UpdateState } from "../src/mygo.ts";
import { UPDATE_SKIP_KEY, pillView, showUpdate } from "../src/update.ts";

const env = (desktop: boolean): Env => ({
  desktop, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

const update = (over: Partial<UpdateState> = {}): UpdateState => ({ state: "ready", current: "0.2.0", latest: "0.3.0", notes: "", done: 0, total: 0, error: "", ...over });

async function startApp(t: TestContext, desktop: boolean, status: UpdateState): Promise<App> {
  resetDom("#overview");
  t.mock.method(Library, "updateStatus", async () => status);
  const state = demoState();
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: env(desktop) });
  app.start();
  await new Promise((resolve) => setTimeout(resolve, 0));
  return app;
}

const wrap = () => document.querySelector<HTMLElement>(".update-wrap");

test("the browser preview has no update pill", async (t) => {
  await startApp(t, false, update());
  assert.equal(wrap(), null);
});

test("a downloaded update shows the pill, whose dialog restarts into it", async (t) => {
  await startApp(t, true, update({ notes: "- 更快的筛选" }));
  assert.equal(wrap()!.hidden, false);
  assert.equal(wrap(), $(".topbar-right").firstElementChild, "the pill leads the top bar");
  assert.equal(text("#update-pill"), "更新");
  assert.equal($("#update-pill").title, "重启以更新到 v0.3.0");

  $("#update-pill").click();
  assert.equal(text("#modal-title"), "更新到 v0.3.0");
  assert.match(text(".update-notes"), /更快的筛选/);
  const restart = t.mock.method(Library, "restartToUpdate", async () => {});
  $('[data-action="update-restart"]').click();
  assert.equal(restart.mock.callCount(), 1);
});

test("an update still downloading stays out of sight", async (t) => {
  await startApp(t, true, update({ state: "downloading", done: 3, total: 10 }));
  assert.equal(wrap()!.hidden, true);
  showUpdate(update({ state: "idle", latest: "" }));
  assert.equal(wrap()!.hidden, true);
});

test("the × hides the pill until a newer version is out", async (t) => {
  await startApp(t, true, update());
  $('[data-action="update-skip"]').click();
  assert.equal(wrap()!.hidden, true);
  assert.equal(localStorage.getItem(UPDATE_SKIP_KEY), "0.3.0");
  showUpdate(update());
  assert.equal(wrap()!.hidden, true, "the same version stays hidden");
  showUpdate(update({ latest: "0.4.0" }));
  assert.equal(wrap()!.hidden, false);
});

test("a failed download says so and tries again on a click", async (t) => {
  await startApp(t, true, update({ state: "error", error: "signature does not match" }));
  assert.equal(text("#update-pill"), "更新失败");
  assert.match($("#update-pill").title, /signature does not match/);
  const retry = t.mock.method(Library, "retryUpdate", async () => {});
  $("#update-pill").click();
  assert.equal(retry.mock.callCount(), 1);
  assert.equal(text("#update-pill"), "下载中…");
  showUpdate(update({ state: "downloading", done: 42, total: 100 }));
  assert.equal(text("#update-pill"), "下载中 42%");
  showUpdate(update());
  assert.equal(text("#update-pill"), "更新");
});

test("pillView leaves background downloads and skipped versions out", () => {
  assert.equal(pillView(null, "", false), null);
  assert.equal(pillView(update({ state: "downloading" }), "", false), null);
  assert.equal(pillView(update(), "0.3.0", false), null);
  assert.equal(pillView(update({ state: "off", latest: "" }), "", false), null);
});
