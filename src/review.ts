// What only a person can do: judge agent evidence, verify dataset links, and
// finish reviewing what a source update added. Each action writes straight to
// SQLite, so a concurrent agent edit of the same record never turns a verdict
// into a save conflict.
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
  "finish-review": (el, id) => {
    const dataset = el.dataset.kind === "dataset";
    const done = "已核查，移出需核对。";
    if (!app.env.real) {
      const item = dataset ? app.state.datasets.find((d) => d.id === id) : app.paperById(id);
      if (!item) return;
      item.awaitingReview = false;
      app.persist();
      app.render();
      app.renderDrawer();
      app.toast(done);
      return;
    }
    void record(app, dataset ? Library.markDatasetReviewed(id) : Library.markPaperReviewed(id), done);
  },
  "verify-repository": (el, id) => {
    const verified = el.dataset.verified === "true";
    void record(app, Library.verifyDatasetRepository(id, el.dataset.record ?? "", verified), verified ? "编号已核实。" : "已撤销核实。");
  },
});
