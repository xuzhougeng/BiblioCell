import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import { setImmediate } from "node:timers/promises";
import { App } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { Library, type Judgement } from "../src/mygo.ts";

const judgement = { verdict: "included", reason: "Evidence <confirmed>", basis: "fulltext", skipped: false } as Judgement;

function setup(t: TestContext) {
  const app = Object.create(App.prototype) as App;
  app.state = demoState();
  app.state.papers[0]!.screening = "pending";
  const id = app.state.papers[0]!.id;
  app.screen = { ids: app.state.papers.map(p => p.id), index: 0, ai: { requirement: "plant cells", results: new Map() } };
  // Any whole-page render would destroy the embedded PDF's browsing context.
  app.render = () => assert.fail("AI updates must not render the page containing the PDF");
  const region = { dataset: { paperId: id }, innerHTML: "initial suggestion" };
  const previous = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", { configurable: true, value: {
    querySelector(selector: string) {
      assert.equal(selector, "#screen-ai", "only the suggestion region may be accessed");
      return region;
    },
  } });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "document", previous);
    else Reflect.deleteProperty(globalThis, "document");
  });
  const pending = Promise.withResolvers<Judgement>();
  const call = t.mock.method(Library, "screenPaper", () => pending.promise);
  return { app, id, region, pending, call };
}

test("AI success updates the suggestion without rebuilding the PDF page", async t => {
  const { app, id, region, pending, call } = setup(t);
  app.requestJudgement(id);
  app.requestJudgement(id);
  assert.equal(call.mock.callCount(), 1);
  assert.equal(region.innerHTML, "initial suggestion");
  pending.resolve(judgement);
  await setImmediate();
  assert.match(region.innerHTML, /Evidence &lt;confirmed&gt;/);
  assert.match(region.innerHTML, /data-action="screen-ai-adopt"/);
});

test("AI failure, retry and skipped results stay within the suggestion region", async t => {
  const { app, id, region, pending } = setup(t);
  app.requestJudgement(id);
  pending.reject(new Error("Model unavailable"));
  await setImmediate();
  assert.match(region.innerHTML, /Model unavailable/);
  assert.match(region.innerHTML, /data-action="screen-ai-retry"/);
  const retry = Promise.withResolvers<Judgement>();
  t.mock.method(Library, "screenPaper", () => retry.promise);
  app.requestJudgement(id, true);
  assert.match(region.innerHTML, /is-loading/);
  retry.resolve({ ...judgement, skipped: true });
  await setImmediate();
  assert.match(region.innerHTML, /is-skipped/);
  assert.doesNotMatch(region.innerHTML, /screen-ai-adopt/);
});

for (const change of ["next paper", "disable AI", "new AI run", "exit screening"] as const) {
  test(`late AI results do not change the page after ${change}`, async t => {
    const { app, id, region, pending } = setup(t);
    app.requestJudgement(id);
    if (change === "next paper") app.screen!.index = 1;
    if (change === "disable AI") app.screen!.ai = null;
    if (change === "new AI run") app.screen!.ai = { requirement: "new requirement", results: new Map() };
    if (change === "exit screening") app.screen = null;
    pending.resolve(judgement);
    await setImmediate();
    assert.equal(region.innerHTML, "initial suggestion");
  });
}
