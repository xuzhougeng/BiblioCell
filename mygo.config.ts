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
  // 自动更新：已安装的应用读最新 Release 里的 update-<平台>.json，只接受用这把公钥对应私钥签名的更新。
  // 私钥由 `mygo keygen` 生成，存在 CI 的 MYGO_UPDATER_PRIVATE_KEY secret 里，不进仓库；
  // 没有私钥时 `mygo build` 照常打包，只是不写更新文件。
  updates: {
    publicKey: "+xl0Y5UK0PG1v2kABam1m8iATF05OL/X8J3R07oZAYo=",
    github: "xuzhougeng/BiblioCell",
  },
});
