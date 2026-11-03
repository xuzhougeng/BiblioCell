// Saved records supply shared vocabulary; explicitly added choices also stay on this device.
import { arr, MODALITY_CHOICES, PLATFORM_CHOICES, REPOSITORIES, type LibraryState } from "./model.ts";
import type { SpeciesEntry } from "./mygo.ts";
import { COMMON_PLANTS } from "./species.ts";
export { COMMON_PLANTS } from "./species.ts";

export type ChoiceKind = "repositories" | "modalities" | "platforms";
const memory = new Map<ChoiceKind, string[]>();
const storageKey = (kind: ChoiceKind) => `bibliocell-choices-${kind}`;

export function distinctChoices(values: string[]): string[] {
  const seen = new Set<string>();
  return values.map(v => v.trim()).filter(v => {
    const key = v.toLowerCase();
    if (!key || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function savedChoices(kind: ChoiceKind): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(storageKey(kind)) ?? "[]");
    if (Array.isArray(value)) return distinctChoices([...value.filter((v): v is string => typeof v === "string"), ...(memory.get(kind) ?? [])]);
  } catch { /* Session choices remain usable when storage is unavailable. */ }
  return memory.get(kind) ?? [];
}

export function rememberChoices(kind: ChoiceKind, values: string[]): void {
  const merged = distinctChoices([...savedChoices(kind), ...values]);
  memory.set(kind, merged);
  try { localStorage.setItem(storageKey(kind), JSON.stringify(merged)); } catch { /* Keep session choices. */ }
}

export function researchChoices(state: LibraryState, kind: "modalities" | "platforms"): string[] {
  return distinctChoices([
    ...(kind === "modalities" ? MODALITY_CHOICES : PLATFORM_CHOICES),
    ...savedChoices(kind), ...[...state.papers, ...state.datasets].flatMap(item => arr(item[kind])),
  ]);
}

export function repositoryChoices(state: LibraryState, selected = ""): Record<string, string> {
  const choices = { ...REPOSITORIES };
  for (const name of [...savedChoices("repositories"), ...state.datasets.flatMap(d => [d.repository, ...arr(d.repositories).map(r => r.repository)]), selected]) {
    const key = (name ?? "").trim().toLowerCase();
    if (key && !Object.hasOwn(choices, key)) Object.defineProperty(choices, key, { value: name.trim(), enumerable: true, configurable: true });
  }
  return choices;
}
export function plantChoices(state?: LibraryState): SpeciesEntry[] {
  const choices = new Map(COMMON_PLANTS.map(({ name, label }) => [name.toLowerCase(), { name, label }]));
  for (const item of state ? [...state.papers, ...state.datasets] : []) {
    for (const entry of arr(item.species)) {
      const name = entry.name.trim();
      if (name && !choices.has(name.toLowerCase())) choices.set(name.toLowerCase(), { name, label: entry.label });
    }
  }
  return [...choices.values()];
}
