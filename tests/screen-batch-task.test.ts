import assert from "node:assert/strict";
import { test } from "node:test";
import { adoptable, draftTask, targetText, verdictLabel, type BatchDraft } from "../src/screen-batch.ts";
import { ScreeningBatch } from "../src/screen-batch-queue.ts";
import type { JudgeTask, ScreeningSuggestion } from "../src/mygo.ts";

const review: JudgeTask = { id: "review", name: "是否综述", question: "是否综述？", basis: "abstract", target: { kind: "articleType", value: "review" } };
const draft = (over: Partial<BatchDraft>): BatchDraft => ({
  current: [], selected: new Set(), scope: "current", search: "", requirement: "plants", modelId: "m", concurrency: 2, kind: "batch",
  taskId: "screening", question: "", tag: "", presets: [review], ...over,
});

test("draftTask is null for screening and reruns, a preset by id, or the custom question with its tag", () => {
  assert.equal(draftTask(draft({})), null);
  assert.equal(draftTask(draft({ taskId: "review", kind: "rerun" })), null);
  assert.deepEqual(draftTask(draft({ taskId: "review" })), review);
  assert.equal(draftTask(draft({ taskId: "missing" })), null);
  assert.deepEqual(draftTask(draft({ taskId: "custom", question: " 是否用了 10x？ ", tag: " 10x " })), { id: "custom", name: "", question: "是否用了 10x？", basis: "abstract", target: { kind: "tag", value: "10x" } });
});

test("verdicts read as decisions for screening and as yes/no for a question", () => {
  assert.equal(verdictLabel("included", null), "已纳入");
  assert.equal(verdictLabel("included", review), "是");
  assert.equal(verdictLabel("excluded", review), "否");
  assert.equal(verdictLabel("odd", review), "odd");
  assert.match(targetText(review), /综述/);
  assert.match(targetText({ ...review, target: { kind: "tag", value: "10x" } }), /10x/);
});

test("adoptable lists recorded yes answers once each, skipping no, uncertain, skipped and unrecorded ones", async () => {
  const batch = new ScreeningBatch(["a", "b", "c", "d", "e"], review.question, "m", 2, "run", "batch", review);
  const answers: Record<string, Partial<ScreeningSuggestion>> = {
    a: { id: "SG1", verdict: "included" }, b: { id: "SG2", verdict: "excluded" }, c: { id: "SG3", verdict: "uncertain" },
    d: { id: "", verdict: "included" }, e: { id: "SG5", verdict: "included", skipped: true },
  };
  await batch.run(async id => ({ skipped: false, ...answers[id] } as ScreeningSuggestion), () => {});
  assert.deepEqual(adoptable(batch), ["SG1"]);
  assert.equal(batch.task, review);
});
