import { defineConfig } from "vite";
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";

// PDF.js loads fonts, CMaps and image decoders by name, including offline builds.
const pdfAssets = ["cmaps", "standard_fonts", "wasm", "web/images"];

export default defineConfig({
  build: { rolldownOptions: { input: { main: "index.html", pdf: "pdf-viewer.html" } } },
  plugins: [{
    name: "pdf-assets",
    configureServer(server) {
      server.middlewares.use("/pdf-assets", (req, res, next) => {
        try {
          const name = decodeURIComponent((req.url ?? "").split("?")[0]!).replace(/^\//, "");
          if (!pdfAssets.some(folder => name.startsWith(`${folder}/`)) || name.includes("\\") || name.split("/").includes("..")) return next();
          const data = readFileSync(resolve("node_modules/pdfjs-dist", name));
          if (name.endsWith(".wasm")) res.setHeader("Content-Type", "application/wasm");
          if (name.endsWith(".svg")) res.setHeader("Content-Type", "image/svg+xml");
          res.end(data);
        } catch { next(); }
      });
    },
    generateBundle() {
      this.emitFile({ type: "asset", fileName: "pdf-assets/LICENSE", source: readFileSync(resolve("node_modules/pdfjs-dist/LICENSE")) });
      for (const folder of pdfAssets) {
        for (const file of readdirSync(resolve("node_modules/pdfjs-dist", folder), { withFileTypes: true })) {
          if (file.isFile()) {
            this.emitFile({
              type: "asset",
              fileName: `pdf-assets/${folder}/${file.name}`,
              source: readFileSync(resolve("node_modules/pdfjs-dist", folder, file.name)),
            });
          }
        }
      }
    },
  }],
  server: {
    // The app loads this address during `mygo dev` (devUrl in mygo.json).
    port: 5173,
    strictPort: true,
    // The development app and the packaged builds are not the page's.
    watch: { ignored: ["**/.mygo/**", "**/build/**"] },
  },
});
