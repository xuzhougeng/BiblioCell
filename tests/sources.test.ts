import assert from "node:assert/strict";
import { test } from "node:test";
import type { QueryChange } from "../src/mygo.ts";
import { renderQueryHistory } from "../src/sources.ts";

const change = (version: string, query: string, previousQuery: string, changedBy: string, reason = ""): QueryChange => ({
  id: `QH-${version}`, sourceId: "pubmed", version, query, provider: "", previousQuery, previousProvider: "", changedBy, reason, changedAt: "2026-11-01T10:00:00Z",
});

test("query history lists every version, ending with the one before the first change", () => {
  const html = renderQueryHistory([
    change("3", "plant[tiab] AND nucleus[tiab]", "plant[tiab]", "agent", "9/10 <included>"),
    change("2", "plant[tiab]", "default query", "human"),
  ], "plant[tiab] AND nucleus[tiab]");
  assert.deepEqual([...html.matchAll(/<strong>v(\d+)<\/strong>/g)].map((m) => m[1]), ["3", "2", "1"]);
  assert.match(html, /AI 助手/);
  assert.match(html, /9\/10 &lt;included&gt;/);
  // The query in use has no fill button; the others do.
  assert.equal([...html.matchAll(/data-action="use-source-query"/g)].length, 2);
  assert.match(html, /data-query="default query"/);
});

test("query history is empty without changes", () => {
  assert.equal(renderQueryHistory([], "anything"), "");
});
