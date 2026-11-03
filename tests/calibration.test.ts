import assert from "node:assert/strict";
import { test } from "node:test";
import { confidenceText, percent, screeningOfMismatches, suggestionOf } from "../src/calibration.ts";
import type { CalibrationPair } from "../src/mygo.ts";

const pair = (suggestionId: string, paperId: string, verdict: string, decision: string): CalibrationPair => ({
  suggestionId, paperId, title: `Paper ${paperId}`, profileId: "fast", profileName: "Fast", model: "m-fast", runId: "run1", kind: "rerun",
  requirement: "plant roots", verdict, reason: "because", basis: "fulltext", excerpt: "Root cells were sequenced.", page: 3, confidence: 0.8,
  createdAt: "2026-03-01T00:00:00Z", decision, decisionReason: "", decidedAt: "2026-03-02T00:00:00Z",
});

test("mismatches open in screening at the clicked paper with their recorded suggestions", () => {
  const pairs = [pair("s3", "p3", "excluded", "included"), pair("s2", "p2", "uncertain", "excluded"), pair("s1", "p3", "included", "excluded")];
  const screen = screeningOfMismatches(pairs, "s2");
  assert.ok(screen);
  assert.deepEqual(screen.ids, ["p3", "p2"]); // one entry per paper, newest suggestion kept
  assert.equal(screen.index, 1);
  assert.equal(screen.returnTo, "calibration");
  assert.equal(screen.ai?.requirement, "plant roots");
  const shown = screen.ai?.results.get("p3");
  assert.equal(shown?.state, "done");
  if (shown?.state === "done") {
    assert.equal(shown.judgement.id, "s3");
    assert.equal(shown.judgement.verdict, "excluded");
    assert.equal(shown.judgement.page, 3);
    assert.equal(shown.judgement.profileName, "Fast");
    assert.equal(shown.judgement.skipped, false);
  }
  assert.equal(screeningOfMismatches(pairs, "missing"), null);
  assert.equal(suggestionOf(pairs[0]!).confidence, 0.8);
});

test("percent and confidence text", () => {
  assert.equal(percent(0.6), "60%");
  assert.equal(percent(1), "100%");
  assert.equal(confidenceText(null), "");
  assert.equal(confidenceText(0.754), "置信度 75%");
});
