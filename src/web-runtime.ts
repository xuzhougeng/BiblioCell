// The web server's stand-in for the mygo runtime. In a browser the page has
// no window bridge, so this installs a `globalThis.mygo` whose calls go to
// /api/call and whose events arrive over /api/events; the generated client in
// mygo.ts then works unchanged. The server decides what each caller may do:
// viewers read, admins who signed in also write.
import { t } from "./i18n.ts";

/** WebSession is what /api/session says about this browser. */
export interface WebSession {
  admin: boolean;
  /** canLogin is false when the server runs without an admin password. */
  canLogin: boolean;
  version: string;
}

type Fetch = typeof fetch;

interface Reply {
  ok?: boolean;
  value?: unknown;
  error?: string;
}

async function readReply(res: Response): Promise<Reply> {
  try {
    return (await res.json()) as Reply;
  } catch {
    return {};
  }
}

/** replyError turns a failed reply into the error the page shows; the status says what kind of failure it was. */
export function replyError(status: number, message: string | undefined): Error {
  let text = message ?? "";
  if (status === 403) text = t("需要管理员登录后才能进行此操作。");
  else if (status === 404) text = t("此功能需要桌面应用。");
  else if (!text) text = t("服务器没有响应（HTTP {status}）。", { status });
  const error = new Error(text);
  error.name = "CallError";
  return error;
}

/** detectWeb asks the server whether this page is served by `BiblioCell web`; null means it is a static preview. */
export async function detectWeb(fetchFn: Fetch = fetch): Promise<WebSession | null> {
  try {
    const res = await fetchFn("/api/session", { headers: { Accept: "application/json" } });
    if (!res.ok || !(res.headers.get("content-type") ?? "").includes("application/json")) return null;
    const data = (await res.json()) as Partial<WebSession> & { web?: boolean };
    if (!data.web) return null;
    return { admin: Boolean(data.admin), canLogin: Boolean(data.canLogin), version: String(data.version ?? "") };
  } catch {
    return null;
  }
}

/** callWeb runs one Library method on the server and returns its value, or throws its error. */
export async function callWeb(method: string, args: unknown[], fetchFn: Fetch = fetch): Promise<unknown> {
  const res = await fetchFn("/api/call", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ method, args }),
  });
  const reply = await readReply(res);
  if (!res.ok || !reply.ok) throw replyError(res.status, reply.error);
  return reply.value;
}

export async function loginWeb(password: string, fetchFn: Fetch = fetch): Promise<WebSession> {
  const res = await fetchFn("/api/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) });
  const reply = await readReply(res);
  if (!res.ok) {
    if (res.status === 401) throw new Error(t("密码不正确。"));
    if (res.status === 429) throw new Error(t("尝试次数过多，请稍后再试。"));
    if (res.status === 404) throw new Error(t("服务器没有配置管理员密码。"));
    throw replyError(res.status, reply.error);
  }
  const data = reply as Partial<WebSession>;
  return { admin: Boolean(data.admin), canLogin: Boolean(data.canLogin), version: String(data.version ?? "") };
}

export async function logoutWeb(fetchFn: Fetch = fetch): Promise<void> {
  await fetchFn("/api/logout", { method: "POST" });
}

type Listener = (payload: unknown) => void;

/** installWebRuntime makes mygo-runtime's call() and events work over HTTP. Call it before anything touches Library. */
export function installWebRuntime(fetchFn: Fetch = fetch): void {
  const listeners = new Map<string, Set<Listener>>();
  let stream: EventSource | null = null;
  const subscribe = (name: string, listener: Listener): (() => void) => {
    let set = listeners.get(name);
    if (!set) {
      set = new Set();
      listeners.set(name, set);
      if (!stream && typeof EventSource !== "undefined") stream = new EventSource("/api/events");
      // Each event name is listened for once; the set fans it out.
      stream?.addEventListener(name, (e) => {
        let payload: unknown = null;
        try { payload = JSON.parse((e as MessageEvent<string>).data); } catch { return; }
        for (const fn of [...(listeners.get(name) ?? [])]) fn(payload);
      });
    }
    set.add(listener);
    return () => { listeners.get(name)?.delete(listener); };
  };
  const unsupported = () => { throw new Error("not available on the web"); };
  const runtime = {
    call: (method: string, ...args: unknown[]) => callWeb(method, args, fetchFn),
    channel: unsupported,
    on: subscribe,
    once: (name: string, listener: Listener) => {
      const off = subscribe(name, (payload) => { off(); listener(payload); });
      return off;
    },
    platform: /Windows/.test(navigator.userAgent) ? "win32" : /Mac/.test(navigator.userAgent) ? "darwin" : "linux",
    windowId: 0,
    locale: navigator.language,
    dir: "ltr",
    preferences: { accent: null, reduceMotion: false, reduceTransparency: false, highContrast: false, textScale: 1 },
    window: {
      minimize: async () => {}, maximize: async () => {}, unmaximize: async () => {}, toggleMaximize: async () => {},
      isMaximized: async () => false, toggleFullScreen: async () => {}, close: async () => {}, setTitle: async (title: string) => { document.title = title; },
    },
  };
  (globalThis as { mygo?: unknown }).mygo = runtime;
}
