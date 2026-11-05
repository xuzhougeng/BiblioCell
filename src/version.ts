import type { Env } from "./app.ts";
import { t } from "./i18n.ts";

/** Report the running binary/server, never the release waiting to be installed. */
export function versionLabel(env: Env): string {
  const version = (env.web?.version ?? env.appVersion ?? "").trim();
  if (version) return /^v?\d/.test(version) ? `v${version.replace(/^v/, "")}` : version;
  return env.desktop || env.web ? t("版本信息暂不可用") : t("开发预览");
}
