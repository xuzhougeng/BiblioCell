// English dictionary, keyed by the Chinese source string. Split by area so
// each file sits next to the code it translates; tests/i18n.test.ts checks
// that every t("…") literal in src has an entry here.
import { agent } from "./en/agent.ts";
import { core } from "./en/core.ts";
import { forms } from "./en/forms.ts";
import { settings } from "./en/settings.ts";
import { views } from "./en/views.ts";

export const en: Record<string, string> = { ...core, ...views, ...forms, ...settings, ...agent };
