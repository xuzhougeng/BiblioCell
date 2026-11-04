// Network proxy options: the global choice on the settings page, its address
// check, and the per-model and per-source "follow / direct" choice.
import { $, change, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import type { SourceStatus } from "../src/mygo.ts";
import { modelModal, proxyURLError } from "../src/settings.ts";
import { sourceConfigModal } from "../src/sources.ts";

const pubmed: SourceStatus = {
  id: "pubmed", name: "PubMed", kind: "literature", description: "", homeUrl: "https://pubmed.ncbi.nlm.nih.gov/", supported: true, enabled: true,
  query: "plant", maxResults: 20, provider: "", proxy: "direct", recordCount: 0, queryVersion: "1", queryChange: null, lastSuccessAt: "", lastRun: null, recentRecords: [],
};

const adminEnv = (proxy: { mode: string; url: string }): Env => ({
  desktop: false, backend: true, readOnly: false, web: { admin: true, canLogin: true, version: "test" }, real: true,
  settings: {
    storagePath: "/srv/library", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "t".repeat(64), ncbiApiKey: "", contactEmail: "",
    models: [{ id: "m1", name: "Local", config: { protocol: "openai", baseUrl: "http://10.0.0.5:8000/v1", apiKey: "", model: "qwen", maxTokens: 0, maxSteps: 0, effort: "", proxy: "direct" } }],
    tasks: { chat: "", screen: "", translate: "" }, quitOnWindowClose: false, sourceCheck: { enabled: false, prompt: "" }, proxy,
  },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [pubmed],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function settingsApp(proxy = { mode: "", url: "" }): App {
  resetDom("#settings");
  const state = demoState();
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: adminEnv(proxy) });
  app.start();
  return app;
}

test("proxyURLError accepts http, https and socks5 proxies and explains the rest", () => {
  for (const ok of ["http://127.0.0.1:7890", "https://proxy.example", "socks5://user:secret@10.0.0.2:1080", " http://proxy.example:8080/ "]) {
    assert.equal(proxyURLError(ok), "", ok);
  }
  for (const bad of ["", "127.0.0.1:7890", "ftp://proxy.example", "socks4://p.example:1080", "http://p.example:8080/path", "http://p.example:8080?x=1", "http://"]) {
    assert.notEqual(proxyURLError(bad), "", bad);
  }
});

test("the settings page follows the system proxy by default and shows the address only for a custom proxy", () => {
  settingsApp();
  const mode = $<HTMLSelectElement>("#settings-proxy-mode");
  assert.equal(mode.value, "system");
  const field = $("#settings-proxy-url-field");
  assert.equal(field.hidden, true);
  change(mode, "custom");
  assert.equal(field.hidden, false);
  change(mode, "direct");
  assert.equal(field.hidden, true);
});

test("saving a custom proxy with an invalid address stops before the backend", () => {
  settingsApp({ mode: "custom", url: "" });
  assert.equal($("#settings-proxy-url-field").hidden, false);
  $<HTMLInputElement>("#settings-proxy-url").value = "127.0.0.1:7890";
  $<HTMLFormElement>("#settings-form").requestSubmit();
  assert.match(text("#settings-save-status"), /代理/);
  assert.equal(document.activeElement?.id, "settings-proxy-url");
});

test("models and sources choose between the global proxy and a direct connection", () => {
  const app = settingsApp();
  modelModal(app, "m1");
  const modelProxy = $<HTMLSelectElement>('#model-profile-form [name="proxy"]');
  assert.equal(modelProxy.value, "direct");
  assert.deepEqual([...modelProxy.options].map((o) => o.value), ["inherit", "direct"]);
  app.closeModal();
  modelModal(app);
  assert.equal($<HTMLSelectElement>('#model-profile-form [name="proxy"]').value, "inherit", "a new model follows the global setting");
  app.closeModal();

  sourceConfigModal(app, "pubmed");
  assert.equal($<HTMLSelectElement>('#source-config-form [name="proxy"]').value, "direct");
  app.closeModal();
  app.env.sources = [{ ...pubmed, proxy: "" }];
  sourceConfigModal(app, "pubmed");
  assert.equal($<HTMLSelectElement>('#source-config-form [name="proxy"]').value, "inherit", "an unset option follows the global setting");
});
