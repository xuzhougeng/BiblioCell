// The HTTP stand-in for the mygo runtime: how calls are encoded, how replies
// and failures come back, and how the web server is told apart from a static preview.
import "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { callWeb, detectWeb, installWebRuntime, loginWeb } from "../src/web-runtime.ts";
import { Library } from "../src/mygo.ts";

type Handler = (url: string, init?: RequestInit) => Response;
const fakeFetch = (handler: Handler): typeof fetch => (async (input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init)) as typeof fetch;
const json = (status: number, body: unknown): Response => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

test("callWeb posts the method and its arguments as JSON and unwraps the value", async () => {
  const seen: { url: string; init?: RequestInit }[] = [];
  const fetchFn = fakeFetch((url, init) => {
    seen.push({ url, init });
    return json(200, { ok: true, value: [{ id: "P-1" }] });
  });
  const value = await callWeb("Library.ListPapers", [], fetchFn);
  assert.deepEqual(value, [{ id: "P-1" }]);
  assert.equal(seen[0]!.url, "/api/call");
  assert.equal(seen[0]!.init?.method, "POST");
  assert.deepEqual(JSON.parse(String(seen[0]!.init?.body)), { method: "Library.ListPapers", args: [] });

  await callWeb("Library.SavePaperEdits", [[{ before: null, after: { id: "P-1" } }]], fetchFn);
  assert.deepEqual(JSON.parse(String(seen[1]!.init?.body)).args, [[{ before: null, after: { id: "P-1" } }]]);
});

test("callWeb turns a method error and the server's refusals into errors the page can show", async () => {
  await assert.rejects(callWeb("Library.DeletePaper", ["x"], fakeFetch(() => json(200, { ok: false, error: "no such paper" }))), { message: "no such paper" });
  await assert.rejects(callWeb("Library.DeletePaper", ["x"], fakeFetch(() => json(403, { ok: false, error: "sign in" }))), { message: /管理员/ });
  await assert.rejects(callWeb("Library.Restart", [], fakeFetch(() => json(404, { ok: false, error: "desktop" }))), { message: /桌面应用/ });
  await assert.rejects(callWeb("Library.Status", [], fakeFetch(() => new Response("gateway down", { status: 502 }))), { message: /502/ });
});

test("detectWeb tells the web server from a static preview", async () => {
  assert.deepEqual(await detectWeb(fakeFetch(() => json(200, { web: true, admin: false, canLogin: true, version: "0.4.0" }))), { admin: false, canLogin: true, version: "0.4.0" });
  assert.equal(await detectWeb(fakeFetch(() => new Response("<!doctype html>", { status: 404, headers: { "Content-Type": "text/html" } }))), null, "GitHub Pages answers with its own 404 page");
  assert.equal(await detectWeb(fakeFetch(() => json(200, { hello: "world" }))), null, "a JSON answer from something else is not the server");
  assert.equal(await detectWeb(fakeFetch(() => { throw new Error("offline"); })), null);
});

test("loginWeb explains a wrong password and a missing configuration", async () => {
  await assert.rejects(loginWeb("x", fakeFetch(() => json(401, { ok: false, error: "wrong password" }))), { message: /密码不正确/ });
  await assert.rejects(loginWeb("x", fakeFetch(() => json(404, { ok: false, error: "no password" }))), { message: /没有配置管理员密码/ });
  await assert.rejects(loginWeb("x", fakeFetch(() => json(429, { ok: false }))), { message: /稍后/ });
  assert.deepEqual(await loginWeb("x", fakeFetch(() => json(200, { web: true, admin: true, canLogin: true, version: "" }))), { admin: true, canLogin: true, version: "" });
});

test("installWebRuntime makes the generated client call the server", async () => {
  const seen: string[] = [];
  installWebRuntime(fakeFetch((url, init) => {
    seen.push(String(init?.body));
    return json(200, { ok: true, value: { paperCount: 3 } });
  }));
  const status = await Library.status();
  assert.equal(status.paperCount, 3);
  assert.deepEqual(JSON.parse(seen[0]!), { method: "Library.Status", args: [] });
  const runtime = (globalThis as { mygo?: { channel: () => unknown; on: (name: string, fn: () => void) => () => void } }).mygo!;
  assert.throws(() => runtime.channel(), /web/);
  assert.equal(typeof runtime.on("sourcesChanged", () => {}), "function", "subscribing without EventSource (as in node) still returns an unsubscribe");
});
