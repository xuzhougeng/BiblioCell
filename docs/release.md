# 发布

版本号只写在 `package.json` 的 `version` 里，`mygo.config.ts` 从那里读取。发布一个版本：

```bash
npm version 0.1.0 --no-git-tag-version
```

```bash
git commit -am "release: v0.1.0" && git tag v0.1.0 && git push origin main v0.1.0
```

推送 `v*` 标签会触发 [`.github/workflows/release.yml`](../.github/workflows/release.yml)：先核对标签与 `package.json` 版本一致、签名 secrets 齐全，然后在 macOS runner 上 `mygo build -platform darwin/universal`（Developer ID 签名 + Apple 公证），在 Windows runner 上 `mygo build -platform windows/amd64`（SSL.com eSigner 签 exe、卸载程序与安装包），两边都通过后创建 GitHub Release，上传 DMG、安装包、自动更新文件和 `SHA256SUMS`。自动更新文件由 mygo 写出：每个平台一个签名的更新包 `bibliocell-<版本>-<平台>.tar.gz`、从已发布版本出发的增量包 `.delta`，以及清单 `update-<平台>.json`。已安装的应用读最新 Release 里的清单，只接受用 `mygo.config.ts` 里 `updates.publicKey` 对应私钥签名的更新。`.github/release-notes/<tag>.md` 存在时作为发布说明，后面附上自动生成的变更列表。

签名只在设置了环境变量时启用（见 `mygo.config.ts` 顶部注释），本机 `npm run build` 仍是 ad hoc 签名、不公证，可以直接运行但 Gatekeeper 会拦截其他 Mac。仓库需要这些 secrets，与 citebox / wisp-science 同名，可复用同一套证书：

| Secret | 用途 |
|---|---|
| `MAC_CERT_P12_BASE64`、`MAC_CERT_PASSWORD` | Developer ID Application 证书（含私钥）的 base64 与密码 |
| `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD` | 公证用的 Apple ID、Team ID 与 app 专用密码 |
| `ESIGNER_CREDENTIAL_ID`、`ESIGNER_USERNAME`、`ESIGNER_PASSWORD`、`ESIGNER_TOTP_SECRET`、`ESIGNER_PIN` | SSL.com eSigner |
| `MYGO_UPDATER_PRIVATE_KEY` | 自动更新的签名私钥，即 `mygo keygen` 生成的 `mygo-update.key`，BiblioCell 专用。丢失后已安装的应用无法再自动更新，泄露则任何人都能向用户推送代码，请另存一份到密码管理器 |

手动运行该 workflow（workflow_dispatch）可以对已有标签重新构建；不勾选 publish 时只把签好名的安装包留作 artifact，不碰 Release。

---

[← 返回文档目录](README.md)
