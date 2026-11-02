import { defineConfig } from "mygo-cli";
import pkg from "./package.json" with { type: "json" };

// 发布版本号只写在 package.json 里；release 工作流会核对 git tag 与它一致。
//
// 签名只在设置了对应环境变量时启用，本机 `npm run build` 仍是 ad hoc 签名、不公证：
// - BIBLIOCELL_MACOS_SIGNING_IDENTITY：Developer ID Application 身份，CI 从导入的证书里读出
// - BIBLIOCELL_NOTARY_PROFILE：`xcrun notarytool store-credentials` 存下的 profile 名，设置后公证 DMG
// - BIBLIOCELL_WINDOWS_SIGN_COMMAND：给一个文件签名的命令，%1 是文件路径；CI 用 scripts/sign-windows-esigner.ps1
//   接 SSL.com eSigner，mygo 会依次签 exe、卸载程序和安装包
const macosSigningIdentity = process.env.BIBLIOCELL_MACOS_SIGNING_IDENTITY?.trim();
const notaryProfile = process.env.BIBLIOCELL_NOTARY_PROFILE?.trim();
const windowsSignCommand = process.env.BIBLIOCELL_WINDOWS_SIGN_COMMAND?.trim();

export default defineConfig({
  name: "BiblioCell",
  slug: "bibliocell",
  identifier: "com.bibliocell.library",
  version: pkg.version,
  copyright: "© 2026 xuzhougeng",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "npm run dev:web",
  buildCommand: "npm run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
  macos: {
    minimumSystemVersion: "12.0",
    signingIdentity: macosSigningIdentity || undefined,
    notarize: notaryProfile ? { keychainProfile: notaryProfile } : undefined,
  },
  windows: {
    signCommand: windowsSignCommand || undefined,
  },
});
