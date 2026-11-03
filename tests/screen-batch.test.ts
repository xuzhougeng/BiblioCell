import assert from "node:assert/strict";
import { test } from "node:test";
import { setImmediate } from "node:timers/promises";
import { ScreeningBatch } from "../src/screen-batch-queue.ts";
import type { Judgement } from "../src/mygo.ts";

const answer = { verdict: "included", reason: "Plant cells", basis: "abstract", skipped: false } as Judgement;

test("batch bounds concurrency, handles out-of-order results and deduplicates papers", async () => {
  const batch = new ScreeningBatch(["a", "b", "c", "a"], "plants", "model-b", 2);
  const requests = new Map<string, ReturnType<typeof Promise.withResolvers<Judgement>>>();
  const work = batch.run(id => {
    const pending = Promise.withResolvers<Judgement>(); requests.set(id, pending); return pending.promise;
  }, () => {});
  assert.deepEqual([...requests.keys()], ["a", "b"]);
  await batch.run(() => assert.fail("duplicate run"), () => {});
  requests.get("b")!.resolve(answer);
  await setImmediate();
  assert.deepEqual([...requests.keys()], ["a", "b", "c"]);
  requests.get("c")!.resolve({ ...answer, skipped: true });
  requests.get("a")!.resolve(answer);
  await work;
  assert.equal(batch.results.size, 3);
  assert.equal(batch.running, false);
  assert.deepEqual(batch.results.get("c"), { state: "done", judgement: { ...answer, skipped: true } });
});

test("stop drains active calls; retry only requests errors and untouched papers", async () => {
  const batch = new ScreeningBatch(["a", "b", "c", "d"], "plants", "model-b", 2);
  const a = Promise.withResolvers<Judgement>();
  const b = Promise.withResolvers<Judgement>();
  const called: string[] = [];
  const work = batch.run(id => { called.push(id); return id === "a" ? a.promise : b.promise; }, () => {});
  batch.stop();
  b.reject(new Error("Rate limited"));
  a.resolve(answer);
  await work;
  assert.deepEqual(called, ["a", "b"]);
  assert.equal(batch.results.has("c"), false);
  assert.deepEqual(batch.results.get("b"), { state: "error", message: "Rate limited" });
  const retry: string[] = [];
  await batch.run(async id => { retry.push(id); return answer; }, () => {});
  assert.deepEqual(retry, ["b", "c", "d"]);
  assert.equal(batch.results.size, 4);
});

test("a synchronous failure does not prevent later papers from completing", async () => {
  const batch = new ScreeningBatch(["a", "b"], "plants", "m", 1);
  await batch.run(id => { if (id === "a") throw new Error("Disconnected"); return Promise.resolve(answer); }, () => {});
  assert.equal(batch.results.get("a")?.state, "error");
  assert.equal(batch.results.get("b")?.state, "done");
});

test("invalid concurrency cannot start an unbounded or stalled queue", () => {
  for (const concurrency of [0, -1, 9, 1.5, NaN]) assert.throws(() => new ScreeningBatch(["a"], "plants", "m", concurrency));
});
