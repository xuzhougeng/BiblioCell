// Translation coverage: every Chinese string the interface shows goes through t()
// and has an English entry, with the same {placeholders}.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { detectLocale, fill, t } from "../src/i18n.ts";
import { en } from "../src/locales/en.ts";
import { agent } from "../src/locales/en/agent.ts";
import { core } from "../src/locales/en/core.ts";
import { forms } from "../src/locales/en/forms.ts";
import { settings } from "../src/locales/en/settings.ts";
import { views } from "../src/locales/en/views.ts";

const HAN = /[一-鿿]/;
const SKIP = new Set(["demo-data.ts", "i18n.ts"]); // Demo records are sample data, not interface text.
const sources = readdirSync(new URL("../src/", import.meta.url)).filter((f) => f.endsWith(".ts") && !SKIP.has(f));
const read = (file: string) => readFileSync(new URL(`../src/${file}`, import.meta.url), "utf8");
const CALL = /\bt\(\s*("(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`)/g;
const literal = (code: string): string => new Function(`return ${code}`)() as string;
const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test("t passes Chinese through and fills placeholders", () => {
  assert.equal(t("文献库"), "文献库");
  assert.equal(fill("共 {n} 篇，{地址} 保留", { n: 3 }), "共 3 篇，{地址} 保留");
});

test("detectLocale prefers the saved choice, then the system language", () => {
  assert.equal(detectLocale("en", ["zh-CN"]), "en");
  assert.equal(detectLocale(null, ["zh-TW"]), "zh-CN");
  assert.equal(detectLocale(null, ["en-US"]), "en");
  assert.equal(detectLocale("fr", []), "zh-CN");
});

test("every t() literal has an English entry with the same placeholders", () => {
  const missing: string[] = [];
  for (const file of sources) {
    for (const match of read(file).matchAll(CALL)) {
      const code = match[1]!;
      assert.ok(!code.includes("${"), `${file}: t() takes params, not a template: ${code}`);
      const key = literal(code);
      const value = en[key];
      if (value === undefined) missing.push(`${file}: ${key}`);
      else assert.deepEqual(placeholders(value), placeholders(key), `${file}: placeholders differ for ${key}`);
    }
  }
  assert.deepEqual(missing, []);
});

test("no Chinese interface text bypasses t()", () => {
  const leaks: string[] = [];
  for (const file of sources) {
    read(file).split("\n").forEach((line, i) => {
      if (!HAN.test(line) || line.includes("i18n-ignore")) return;
      const code = line
        .replace(/^\s*(\/\/|\*|\/\*\*).*$/, "")
        .replace(/\s\/\/ .*$/, "")
        .replace(CALL, "t(…")
        .replace(/\btv\([^)]*\)/g, "tv(…)");
      if (HAN.test(code)) leaks.push(`${file}:${i + 1}: ${line.trim().slice(0, 120)}`);
    });
  }
  assert.deepEqual(leaks, []);
});

test("index.html chrome has English entries", () => {
  const html = readFileSync(new URL("../index.html", import.meta.url), "utf8").replace(/<script[\s\S]*?<\/script>/g, "");
  const texts = [...html.matchAll(/>([^<>]+)</g)].map((m) => m[1]!.trim());
  const attrs = [...html.matchAll(/(?:title|aria-label|placeholder|content)="([^"]+)"/g)].map((m) => m[1]!);
  const missing = [...texts, ...attrs].filter((s) => HAN.test(s) && en[s] === undefined);
  assert.deepEqual([...new Set(missing)], []);
});

test("a key shared by several dictionaries has one English meaning", () => {
  const seen = new Map<string, string>();
  const conflicts: string[] = [];
  for (const part of [core, views, forms, settings, agent]) {
    for (const [key, value] of Object.entries(part)) {
      const before = seen.get(key);
      if (before !== undefined && before !== value) conflicts.push(`${key}: ${before} / ${value}`);
      seen.set(key, value);
    }
  }
  assert.deepEqual(conflicts, []);
});
