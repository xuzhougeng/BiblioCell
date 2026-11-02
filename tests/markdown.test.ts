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
