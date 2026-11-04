// The page's side of the embedded PDF viewer (pdf-viewer.html): where a
// paper's PDF frame points, and how to take it to a page and a quoted
// sentence. A frame that has loaded the viewer is sent a message; one that
// has not (or shows another file) is loaded with the target in its hash.

/** A page of a PDF and the sentence to highlight on it; an empty quote only turns to the page. */
export interface PdfTarget {
  page: number;
  quote: string;
}

/** Messages the viewer accepts from the page that embeds it. */
export const LOCATE_MESSAGE = "bibliocell-pdf-locate";
/** Messages the viewer sends back: it is ready, it located a target, or a key it does not use was pressed in it. */
export const READY_MESSAGE = "bibliocell-pdf-ready";
export const LOCATED_MESSAGE = "bibliocell-pdf-located";
export const KEY_MESSAGE = "bibliocell-pdf-key";

export interface LocatedMessage {
  type: typeof LOCATED_MESSAGE;
  page: number;
  /** Whether the quote was found and highlighted; false when only the page could be shown. */
  found: boolean;
  quote: string;
}

/** Keep the reader in its own document so PDF search is scoped to the PDF. */
export const paperFileURL = (fileId: string): string => `/pdf-viewer.html?file=${encodeURIComponent(fileId)}`;

/** targetHash writes a target into a viewer URL fragment; the viewer reads it when the document loads. */
export function targetHash(target: PdfTarget): string {
  const params = new URLSearchParams();
  if (target.page > 0) params.set("page", String(target.page));
  if (target.quote) params.set("quote", target.quote);
  const hash = params.toString();
  return hash ? `#${hash}` : "";
}

/** parseTargetHash reads a target back from a fragment; null when it carries none. */
export function parseTargetHash(hash: string): PdfTarget | null {
  const params = new URLSearchParams(hash.replace(/^#/, ""));
  const page = Math.trunc(Number(params.get("page") ?? 0));
  const quote = params.get("quote") ?? "";
  if (!page && !quote) return null;
  return { page: page > 0 ? page : 0, quote };
}

/** frameBase is a frame's document without the fragment, to tell a page jump from a different file. */
export const frameBase = (frame: HTMLIFrameElement | null | undefined): string => frame?.getAttribute("src")?.split("#")[0] ?? "";

const ready = new WeakSet<Window>();
let watching = false;

/** watchFrames remembers which viewer frames have loaded, so a target can be sent instead of reloading,
 * and lets Escape pressed inside a viewer act on the page around it, as if the page had focus. */
export function watchFrames(): void {
  if (watching || typeof window === "undefined") return;
  watching = true;
  window.addEventListener("message", (event) => {
    const data: unknown = event.data;
    if (!data || typeof data !== "object") return;
    const { type, key } = data as { type?: unknown; key?: unknown };
    // The source is the frame's WindowProxy; it belongs to another realm, so instanceof Window would say no.
    if (type === READY_MESSAGE && event.source && typeof event.source === "object") ready.add(event.source as Window);
    if (type === KEY_MESSAGE && key === "Escape" && [...document.querySelectorAll("iframe")].some((frame) => frame.contentWindow === event.source)) {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    }
  });
}

/** locateInFrame takes a viewer frame to a target: by message when the viewer is up, by reloading at the hash otherwise. */
export function locateInFrame(frame: HTMLIFrameElement, target: PdfTarget): void {
  const frameWindow = frame.contentWindow;
  if (frameWindow && ready.has(frameWindow)) {
    frameWindow.postMessage({ type: LOCATE_MESSAGE, page: target.page, quote: target.quote }, "*");
    return;
  }
  const base = frameBase(frame);
  // The viewer reads the fragment when the document loads, so the frame is reloaded at the target.
  frame.src = "about:blank";
  requestAnimationFrame(() => { frame.src = base + targetHash(target); });
}
