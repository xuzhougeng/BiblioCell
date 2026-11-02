// A small Markdown renderer for assistant replies. Every piece of text passes
// through esc() before any tag is added, so model output can never inject
// HTML. Covers what chat replies use: headings, paragraphs, bold, italic,
// inline code, fenced code, links, lists, quotes, tables, and rules.
import { esc } from "./model.ts";

const SAFE_URL = /^(https?:|mailto:|#)/i;

function inline(text: string): string {
  // Protect code spans first so their contents stay verbatim.
  const codes: string[] = [];
  let out = text.replace(/`([^`\n]+)`/g, (_m, code: string) => {
    codes.push(`<code>${esc(code)}</code>`);
    return `\u0000${codes.length - 1}\u0000`;
  });
  out = esc(out);
  out = out.replace(/\[([^\]\n]+)\]\(([^)\s]+)\)/g, (m, label: string, href: string) => {
    const url = href.replace(/&amp;/g, "&");
    if (!SAFE_URL.test(url)) return m;
    return `<a href="${esc(url)}" target="_blank" rel="noopener noreferrer">${label}</a>`;
  });
  out = out.replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>");
  out = out.replace(/(^|[^*\w])\*([^*\n]+)\*(?!\w)/g, "$1<em>$2</em>");
  out = out.replace(/(^|[^_\w])_([^_\n]+)_(?!\w)/g, "$1<em>$2</em>");
  out = out.replace(/\u0000(\d+)\u0000/g, (_m, i: string) => codes[Number(i)] ?? "");
  return out;
}

const isRule = (line: string): boolean => /^ {0,3}([-*_])( *\1){2,} *$/.test(line);
const listItem = (line: string): RegExpMatchArray | null => line.match(/^( {0,3})([-*+]|\d+[.)])\s+(.*)$/);
const tableRow = (line: string): string[] => line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
const isTableDivider = (line: string): boolean => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(line);

/** renderMarkdown turns a reply into HTML. Unknown constructs fall through as text. */
export function renderMarkdown(source: string): string {
  const lines = source.replace(/\r\n?/g, "\n").split("\n");
  const html: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i]!;
    if (!line.trim()) {
      i++;
      continue;
    }
    const fence = line.match(/^ {0,3}(`{3,}|~{3,})\s*(\S*)/);
    if (fence) {
      const close = fence[1]!;
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i]!.trim().startsWith(close)) body.push(lines[i++]!);
      i++; // closing fence, or end of text
      const lang = fence[2] ? ` class="language-${esc(fence[2])}"` : "";
      html.push(`<pre><code${lang}>${esc(body.join("\n"))}</code></pre>`);
      continue;
    }
    const heading = line.match(/^ {0,3}(#{1,6})\s+(.+?)\s*#*\s*$/);
    if (heading) {
      const level = Math.min(6, heading[1]!.length + 2); // keep page headings bigger than reply headings
      html.push(`<h${level}>${inline(heading[2]!)}</h${level}>`);
      i++;
      continue;
    }
    if (isRule(line)) {
      html.push("<hr>");
      i++;
      continue;
    }
    if (line.trimStart().startsWith(">")) {
      const quoted: string[] = [];
      while (i < lines.length && lines[i]!.trimStart().startsWith(">")) quoted.push(lines[i++]!.replace(/^\s*> ?/, ""));
      html.push(`<blockquote>${renderMarkdown(quoted.join("\n"))}</blockquote>`);
      continue;
    }
    if (line.includes("|") && i + 1 < lines.length && isTableDivider(lines[i + 1]!)) {
      const head = tableRow(line);
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i]!.includes("|") && lines[i]!.trim()) rows.push(tableRow(lines[i++]!));
      const cells = (row: string[], tag: string) => row.map((c) => `<${tag}>${inline(c)}</${tag}>`).join("");
      html.push(`<table><thead><tr>${cells(head, "th")}</tr></thead><tbody>${rows.map((r) => `<tr>${cells(r, "td")}</tr>`).join("")}</tbody></table>`);
      continue;
    }
    const item = listItem(line);
    if (item) {
      const ordered = /\d/.test(item[2]!);
      const indent = item[1]!.length;
      const items: string[] = [];
      while (i < lines.length) {
        const m = listItem(lines[i]!);
        if (!m || m[1]!.length !== indent || /\d/.test(m[2]!) !== ordered) break;
        const text: string[] = [m[3]!];
        i++;
        // Continuation and nested lines belong to the item until a blank line or a sibling.
        while (i < lines.length && lines[i]!.trim() && !(listItem(lines[i]!)?.[1]!.length === indent)) text.push(lines[i++]!.replace(new RegExp(`^ {0,${indent + 2}}`), ""));
        const nested = text.slice(1).join("\n");
        items.push(`<li>${inline(text[0]!)}${nested.trim() ? renderMarkdown(nested) : ""}</li>`);
      }
      const tag = ordered ? "ol" : "ul";
      html.push(`<${tag}>${items.join("")}</${tag}>`);
      continue;
    }
    const paragraph: string[] = [];
    while (i < lines.length && lines[i]!.trim() && !listItem(lines[i]!) && !lines[i]!.match(/^ {0,3}(#{1,6}\s|`{3}|~{3}|>)/) && !isRule(lines[i]!)) paragraph.push(lines[i++]!.trim());
    html.push(`<p>${paragraph.map(inline).join("<br>")}</p>`);
  }
  return html.join("");
}
