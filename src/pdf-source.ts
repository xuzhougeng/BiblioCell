/** Resolve the file transport in both the desktop window and its PDF iframe. */
export function pdfSource(file: string, view: Window = window): { url: string; disableRange: boolean } {
  type RuntimeWindow = Window & { mygo?: { windowId?: number } };
  let runtime = (view as RuntimeWindow).mygo;
  // MyGo injects its bridge only into the top-level window. The same-origin
  // reader inherits the transport from its parent, without installing a bridge.
  try {
    runtime ??= (view.parent as RuntimeWindow).mygo;
  } catch {
    // A web reader embedded by another origin cannot inspect its parent.
  }
  // installWebRuntime also supplies mygo, using windowId 0. Native windows
  // have positive IDs, so the web adapter must keep the HTTP file route.
  const desktop = (runtime?.windowId ?? 0) > 0;
  const url = desktop
    ? `${/Windows/.test(view.navigator.userAgent) ? "http://paperfile.localhost" : "paperfile://localhost"}/${encodeURIComponent(file)}`
    : /^(\/(?!\/)|blob:)/.test(file) ? file : `/paperfile/${encodeURIComponent(file)}`;
  return { url, disableRange: desktop };
}
