// Paper files in the storage folder: import, open and remove.
import type { App } from "./app.ts";
import { cancelButton } from "./forms.ts";
import { esc, fileResource } from "./model.ts";
import { Library } from "./mygo.ts";
import { enqueue } from "./store.ts";
import { errorMessage } from "./ui.ts";

/** refresh reloads papers as well, because the fulltext status follows the stored files. */
async function refresh(app: App): Promise<void> {
  try {
    const [papers, files] = await enqueue(() => Promise.all([Library.listPapers(), Library.listPaperFiles()]));
    app.state.papers = papers;
    app.state.resources = files.map(fileResource);
    app.render();
    app.renderDrawer();
  } catch (error) {
    app.toast(`刷新失败：${errorMessage(error, "请重新打开应用。")}`, true);
  }
}

/** fileActions builds the handlers of the data-action buttons on stored files. */
export const fileActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "import-file": async (el, id) => {
    const button = el as HTMLButtonElement;
    button.disabled = true;
    try {
      const files = await Library.importPaperFiles(id, button.dataset.type ?? "fulltext");
      if (!files.length) return;
      await refresh(app);
      app.toast(`已导入 ${files.length} 个文件。`);
    } catch (error) {
      app.toast(`导入失败：${errorMessage(error, "请检查文件后重试。")}`, true);
      // Files before the failed one are already stored.
      await refresh(app);
    } finally {
      button.disabled = false;
    }
  },
  "open-file": (_el, id) => {
    Library.openPaperFile(id).catch((error: unknown) => {
      app.toast(`无法打开文件：${errorMessage(error, "请重试。")}`, true);
    });
  },
  "remove-file": (_el, id) => {
    const file = app.state.resources.find((r) => r.id === id);
    if (!file) return;
    app.showModal("移除文件", `<p>从这篇文献移除“${esc(file.title)}”？</p><p class="muted">存放文件夹里的副本会移到系统废纸篓，你原来的文件不受影响。</p>`, `${cancelButton}<button class="btn btn-danger" data-action="confirm-remove-file" data-id="${esc(id)}">移除</button>`);
  },
  "confirm-remove-file": async (el, id) => {
    (el as HTMLButtonElement).disabled = true;
    try {
      await Library.removePaperFile(id);
      app.closeModal();
      await refresh(app);
      app.toast("文件已移除。");
    } catch (error) {
      app.closeModal();
      app.toast(`移除失败：${errorMessage(error, "请重试。")}`, true);
    }
  },
});
