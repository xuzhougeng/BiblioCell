// Finding a quoted sentence in a PDF page's text layer. The quote comes from
// text a model read, which another extractor produced: word spacing, line
// breaks, hyphenation and ligatures differ from what PDF.js sees. So both
// sides are reduced to letters and digits before comparing, and a quote that
// is not found whole is retried by its opening words, like the Go side does
// for pages. Pure functions, so the node:test suite can cover them.

/** Where a match starts or ends: an item of the page's text content and a character offset in its string. */
export interface TextPosition {
  item: number;
  offset: number;
}

/** One item's share of a match: the character range of its string that is quoted. */
export interface MatchSpan {
  item: number;
  start: number;
  end: number;
}

/** Lengths (in letters and digits) of the openings a quote shrinks to when it is not found whole. */
const OPENINGS = [80, 40, 20];

/** fold reduces one character to the letters and digits it compares as: ligatures split, case ignored. */
function fold(char: string): string {
  return char.normalize("NFKC").toLowerCase().replace(/[^\p{L}\p{N}]/gu, "");
}

/** normalizeQuote reduces a quote to its letters and digits. */
export const normalizeQuote = (quote: string): string => [...quote].map(fold).join("");

/** quoteNeedles lists what to look for, longest first: the whole quote, then its openings. */
export function quoteNeedles(quote: string): string[] {
  const whole = normalizeQuote(quote);
  if (!whole) return [];
  const needles = [whole];
  for (const limit of OPENINGS) if (whole.length > limit) needles.push(whole.slice(0, limit));
  return needles;
}

/** PageIndex is a page's text folded for matching, with a way back to the text items. */
export interface PageIndex {
  text: string;
  /** For each character of text, where it came from. */
  origins: TextPosition[];
}

/** indexPage folds the strings of a page's text items into one searchable string. */
export function indexPage(items: readonly { str: string }[]): PageIndex {
  let text = "";
  const origins: TextPosition[] = [];
  items.forEach((item, index) => {
    [...item.str].forEach((char, offset) => {
      const folded = fold(char);
      for (let i = 0; i < folded.length; i++) origins.push({ item: index, offset });
      text += folded;
    });
  });
  return { text, origins };
}

/** findNeedle locates a folded needle in an indexed page and maps it back to item spans; null when absent. */
export function findNeedle(page: PageIndex, needle: string, items: readonly { str: string }[]): MatchSpan[] | null {
  if (!needle) return null;
  const at = page.text.indexOf(needle);
  if (at < 0) return null;
  const first = page.origins[at]!;
  const last = page.origins[at + needle.length - 1]!;
  const spans: MatchSpan[] = [];
  for (let item = first.item; item <= last.item; item++) {
    const length = [...(items[item]?.str ?? "")].length;
    spans.push({
      item,
      start: item === first.item ? first.offset : 0,
      end: item === last.item ? last.offset + 1 : length,
    });
  }
  return spans;
}

/** pageOrder lists page numbers to search: the hinted page, its neighbours, then the rest in order. */
export function pageOrder(hint: number, total: number): number[] {
  const order: number[] = [];
  const add = (n: number): void => { if (n >= 1 && n <= total && !order.includes(n)) order.push(n); };
  if (hint >= 1) for (const delta of [0, 1, -1, 2, -2]) add(hint + delta);
  for (let n = 1; n <= total; n++) add(n);
  return order;
}
