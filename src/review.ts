// What only a person can do: judge agent evidence and verify dataset links.
// Each action writes straight to SQLite, so a concurrent agent edit of the same
// dataset never turns a verdict into a save conflict.
import type { App } from "./app.ts";
import { Library } from "./mygo.ts";
import { errorMessage } from "./ui.ts";

async function record(app: App, verdict: Promise<void>, done: string): Promise<void> {
  try {
    await verdict;
    await app.reload();
    app.toast(done);
  } catch (error) {
    app.toast(`保存失败：${errorMessage(error, "请重试。")}`, true);
  }
}

/** reviewActions builds the handlers of the data-action buttons on agent claims. */
export const reviewActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "review-evidence": (el, id) => {
    const status = el.dataset.status ?? "unreviewed";
    void record(app, Library.reviewEvidence(id, status), status === "verified" ? "证据已确认。" : status === "rejected" ? "证据已驳回。" : "已撤销判断。");
  },
  "verify-link": (el) => {
    const { paper = "", dataset = "", role = "" } = el.dataset;
    const verified = el.dataset.verified === "true";
    void record(app, Library.verifyDatasetLink(paper, dataset, role, verified), verified ? "关联已核实。" : "已撤销核实。");
  },
  "verify-repository": (el, id) => {
    const verified = el.dataset.verified === "true";
    void record(app, Library.verifyDatasetRepository(id, el.dataset.record ?? "", verified), verified ? "编号已核实。" : "已撤销核实。");
  },
});
