import assert from "node:assert/strict";
import { test } from "node:test";
import { argsHint, breakdown, formatElapsed, toolCardHTML, type ToolRun } from "../src/agent-tools.ts";

const call = (id: string, name: string, args: object, result?: { content: string; isError?: boolean }): ToolRun =>
  ({ call: { id, name, arguments: JSON.stringify(args) }, result });

test("argsHint folds JSON arguments into one short line", () => {
  assert.equal(argsHint('{"status":"pending","limit":50,"ids":["a","b"],"meta":{"x":1},"flag":true}'),
    'status: "pending", limit: 50, ids: [2 项], meta: {…}, flag: true');
  assert.equal(argsHint('{"note":"' + "字".repeat(60) + '"}'), 'note: "' + "字".repeat(39) + '…"');
  assert.equal(argsHint("not json\n  at all"), "not json at all");
  assert.equal(argsHint("{" + '"k":"v",'.repeat(40) + "}").length, 90);
});

test("a run of calls becomes one card with a breakdown and one row per call", () => {
  const runs = [
    call("c1", "list_papers", { status: "pending" }, { content: "[]" }),
    call("c2", "list_papers", { status: "pending", page: 2 }, { content: "[]" }),
    call("c3", "update_paper", { id: "p1" }, { content: "boom", isError: true }),
  ];
  assert.equal(breakdown(runs), "list_papers ×2 · update_paper ×1");
  const html = toolCardHTML(runs);
  assert.match(html, /调用了 3 次工具，1 次失败/);
  assert.match(html, /agent-tools-icon failed/);
  assert.match(html, /list_papers ×2 · update_paper ×1/);
  assert.equal((html.match(/<li>/g) ?? []).length, 3);
  assert.match(html, /<code>update_paper<\/code><span class="agent-tool-hint">id: &quot;p1&quot;<\/span><span class="agent-tool-state failed">失败<\/span>/);
  assert.doesNotMatch(html, /<details class="agent-tools" data-key="c1" open>/); // Finished cards start folded.
  assert.match(html, /<span class="agent-tool-caption">结果<\/span><pre>boom<\/pre>/);
});

test("one finished call shows its arguments in the head and its details right below", () => {
  const html = toolCardHTML([call("c1", "describe_library", {}, { content: "ok" })]);
  assert.match(html, /调用了 <code>describe_library<\/code>/);
  assert.doesNotMatch(html, /agent-tools-list/);
  assert.match(html, /<\/summary><div class="agent-tool-body">/);
});

test("a running card stays open, names the call in flight and escapes everything", () => {
  const runs = [
    call("c1", "list_papers", { q: "<b>" }, { content: "<script>" }),
    call("c2", "update_paper", { id: "<i>" }),
  ];
  const html = toolCardHTML(runs, { running: true, step: 4, startedAt: Date.now() - 65_000 });
  assert.match(html, /<details class="agent-tools" data-key="c1" open>/);
  assert.match(html, /正在调用 <code>update_paper<\/code>/);
  assert.match(html, /已发起 2 次 · list_papers ×1 · update_paper ×1/);
  assert.match(html, /第 4 步 · <span class="agent-tools-elapsed" data-started="\d+">1m 5s<\/span>/);
  assert.match(html, /agent-tool-state running/);
  assert.doesNotMatch(html, /<script>|<b>|<i>/);
});

test("formatElapsed", () => {
  assert.equal(formatElapsed(900), "1s");
  assert.equal(formatElapsed(59_400), "59s");
  assert.equal(formatElapsed(125_000), "2m 5s");
});
