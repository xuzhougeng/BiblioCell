import assert from "node:assert/strict";
import { test } from "node:test";
import { findNeedle, indexPage, normalizeQuote, pageOrder, quoteNeedles } from "../src/pdf-locate.ts";

const items = [
  { str: "Libraries were pre-" },
  { str: "pared with 10x Genomics " },
  { str: "Chromium and sequenced on NovaSeq." },
  { str: "Data under GSE123456." },
];

test("normalizeQuote keeps only letters and digits, folding case and ligatures", () => {
  assert.equal(normalizeQuote("Pre-pared  with\n10x Genomics!"), "preparedwith10xgenomics");
  assert.equal(normalizeQuote("ﬁne Zellen – 根组织"), "finezellen根组织");
  assert.deepEqual(quoteNeedles(""), []);
  const long = "a".repeat(100);
  assert.deepEqual(quoteNeedles(long).map((n) => n.length), [100, 80, 40, 20]);
  assert.deepEqual(quoteNeedles("short quote").map((n) => n.length), [10]);
});

test("findNeedle maps a folded match back to item character ranges", () => {
  const page = indexPage(items);
  const spans = findNeedle(page, normalizeQuote("prepared with 10x Genomics Chromium"), items);
  assert.deepEqual(spans, [
    { item: 0, start: 15, end: 19 }, // "pre-"
    { item: 1, start: 0, end: 24 }, // the whole middle item, trailing space included
    { item: 2, start: 0, end: 8 }, // "Chromium"
  ]);
  assert.deepEqual(findNeedle(page, normalizeQuote("GSE123456"), items), [{ item: 3, start: 11, end: 20 }]);
  assert.equal(findNeedle(page, normalizeQuote("nothing like it"), items), null);
  assert.equal(findNeedle(page, "", items), null);
});

test("quoteNeedles lets a long quote the model shortened still land by its opening", () => {
  const page = indexPage(items);
  const quote = "Libraries were prepared with 10x Genomics Chromium and sequenced on NovaSeq, then something invented at the end that is nowhere on the page at all";
  const [whole, ...openings] = quoteNeedles(quote);
  assert.equal(findNeedle(page, whole!, items), null);
  const found = openings.map((needle) => findNeedle(page, needle, items)).find(Boolean);
  assert.ok(found && found[0]!.item === 0 && found[0]!.start === 0);
});

test("pageOrder tries the hinted page and its neighbours before the rest", () => {
  assert.deepEqual(pageOrder(5, 8), [5, 6, 4, 7, 3, 1, 2, 8]);
  assert.deepEqual(pageOrder(1, 3), [1, 2, 3]);
  assert.deepEqual(pageOrder(0, 3), [1, 2, 3]);
  assert.deepEqual(pageOrder(9, 3), [1, 2, 3]);
});
