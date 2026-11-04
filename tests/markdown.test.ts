import assert from "node:assert/strict";
import { test } from "node:test";
import { renderMarkdown } from "../src/markdown.ts";

test("renderMarkdown handles inline emphasis, code and links", () => {
  assert.equal(renderMarkdown("共有 **886 篇文献**，另有 *456* 个数据集，用 `list_papers` 查。"),
    "<p>共有 <strong>886 篇文献</strong>，另有 <em>456</em> 个数据集，用 <code>list_papers</code> 查。</p>");
  assert.equal(renderMarkdown("见 [PubMed](https://pubmed.ncbi.nlm.nih.gov/?term=a&b=c)"),
    '<p>见 <a href="https://pubmed.ncbi.nlm.nih.gov/?term=a&amp;b=c" target="_blank" rel="noopener noreferrer">PubMed</a></p>');
  assert.equal(renderMarkdown("[x](javascript:alert(1))"), "<p>[x](javascript:alert(1))</p>");
});

test("renderMarkdown escapes HTML everywhere, including code", () => {
  assert.equal(renderMarkdown("<img src=x onerror=alert(1)> and `<b>`"), "<p>&lt;img src=x onerror=alert(1)&gt; and <code>&lt;b&gt;</code></p>");
  assert.equal(renderMarkdown("```html\n<script>1</script>\n```"), '<pre><code class="language-html">&lt;script&gt;1&lt;/script&gt;</code></pre>');
});

test("renderMarkdown builds blocks: headings, lists, quotes, tables, rules", () => {
  assert.equal(renderMarkdown("## 结果\n\n- a\n- **b**\n\n1. one\n2. two\n\n> note\n\n---"),
    "<h4>结果</h4><ul><li>a</li><li><strong>b</strong></li></ul><ol><li>one</li><li>two</li></ol><blockquote><p>note</p></blockquote><hr>");
  assert.equal(renderMarkdown("- outer\n  - inner\n- next"), "<ul><li>outer<ul><li>inner</li></ul></li><li>next</li></ul>");
  assert.equal(renderMarkdown("| 题目 | 年份 |\n|---|---|\n| A | 2024 |"),
    "<table><thead><tr><th>题目</th><th>年份</th></tr></thead><tbody><tr><td>A</td><td>2024</td></tr></tbody></table>");
  assert.equal(renderMarkdown("line one\nline two\n\npara two"), "<p>line one<br>line two</p><p>para two</p>");
});

test("renderMarkdown turns cite links into citation buttons that carry the verbatim sentence", () => {
  // The link text may paraphrase; the title holds the sentence to highlight.
  assert.equal(renderMarkdown('Zhang 在[方法里说明了建库平台](cite:P-1#page=3 "Libraries were prepared with 10x Genomics Chromium.")。'),
    '<p>Zhang 在<button type="button" class="agent-cite" data-action="agent-cite" data-paper="P-1" data-page="3" data-quote="Libraries were prepared with 10x Genomics Chromium." title="打开 PDF 第 3 页并高亮原文">方法里说明了建库平台<span class="agent-cite-page">第 3 页</span></button>。</p>');
  // Without a title the link text is the quote; without a page only the file opens.
  assert.equal(renderMarkdown("[Root tips were dissected.](cite:P-2)"),
    '<p><button type="button" class="agent-cite" data-action="agent-cite" data-paper="P-2" data-page="0" data-quote="Root tips were dissected." title="打开这篇文献的 PDF">Root tips were dissected.</button></p>');
  // A bare cite target in running text still becomes a button; the ID ends before punctuation.
  assert.equal(renderMarkdown("见 cite:P-3#page=2。"),
    '<p>见 <button type="button" class="agent-cite" data-action="agent-cite" data-paper="P-3" data-page="2" data-quote="" title="打开 PDF 第 2 页并高亮原文">P-3<span class="agent-cite-page">第 2 页</span></button>。</p>');
  // Model output stays escaped inside the button and its attributes; emphasis marks stay out of the quote.
  const html = renderMarkdown('[**bold** <b>x</b>](cite:P-4#page=1 "a "quoted" <i>claim</i>")');
  assert.match(html, /data-quote="a &quot;quoted&quot; &lt;i&gt;claim&lt;\/i&gt;"/);
  assert.match(html, /><strong>bold<\/strong> &lt;b&gt;x&lt;\/b&gt;<span/);
  // Ordinary links keep their title and other schemes are left alone.
  assert.equal(renderMarkdown('[a](https://x.y "note")'), '<p><a href="https://x.y" target="_blank" rel="noopener noreferrer" title="note">a</a></p>');
  assert.equal(renderMarkdown("[x](cite:)"), "<p>[x](cite:)</p>");
});
