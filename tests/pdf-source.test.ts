import assert from "node:assert/strict";
import { test } from "node:test";
import { JSDOM } from "jsdom";
import { pdfSource } from "../src/pdf-source.ts";

test("Windows PDF iframe uses its parent's desktop transport and disables ranges", () => {
  const dom = new JSDOM('<iframe></iframe>', { url: "http://mygo.localhost/" });
  try {
    const parent = dom.window;
    Object.defineProperty(parent, "mygo", { value: { windowId: 1 } });
    const frame = parent.document.querySelector("iframe")!.contentWindow!;
    Object.defineProperty(frame.navigator, "userAgent", { value: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" });
    assert.equal((frame as Window & { mygo?: unknown }).mygo, undefined);
    assert.deepEqual(pdfSource("R-file /中", frame), {
      url: "http://paperfile.localhost/R-file%20%2F%E4%B8%AD", disableRange: true,
    });
  } finally { dom.window.close(); }
});

test("standalone desktop reader retains the native scheme on macOS", () => {
  const dom = new JSDOM();
  try {
    Object.defineProperty(dom.window, "mygo", { value: { windowId: 1 } });
    Object.defineProperty(dom.window.navigator, "userAgent", { value: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" });
    assert.deepEqual(pdfSource("R-file", dom.window as unknown as Window), {
      url: "paperfile://localhost/R-file", disableRange: true,
    });
  } finally { dom.window.close(); }
});

test("web reader keeps HTTP ranges and same-origin fixture URLs", () => {
  const dom = new JSDOM('<iframe></iframe>', { url: "https://library.example/" });
  try {
    const frame = dom.window.document.querySelector("iframe")!.contentWindow!;
    assert.deepEqual(pdfSource("R-file", frame), { url: "/paperfile/R-file", disableRange: false });
    Object.defineProperty(dom.window, "mygo", { value: { windowId: 0 } });
    for (const [file, url] of [
      ["R-file /中", "/paperfile/R-file%20%2F%E4%B8%AD"],
      ["/tests/fixture.pdf", "/tests/fixture.pdf"],
      ["blob:https://library.example/id", "blob:https://library.example/id"],
      ["//other.example/file", "/paperfile/%2F%2Fother.example%2Ffile"],
    ]) assert.deepEqual(pdfSource(file!, frame), { url, disableRange: false });
    Object.defineProperty(frame, "parent", { get() { throw new DOMException("Blocked", "SecurityError"); } });
    assert.deepEqual(pdfSource("R-file", frame), { url: "/paperfile/R-file", disableRange: false });
  } finally { dom.window.close(); }
});
