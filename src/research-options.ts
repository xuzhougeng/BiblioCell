// Saved records supply shared vocabulary; explicitly added choices also stay on this device.
import { arr, MODALITY_CHOICES, PLATFORM_CHOICES, REPOSITORIES, type LibraryState } from "./model.ts";
import type { SpeciesEntry } from "./mygo.ts";

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

// Species names are research data, independent of interface language.
export const COMMON_PLANTS: SpeciesEntry[] = [
  { name: "Arabidopsis thaliana", label: "拟南芥" }, // i18n-ignore
  { name: "Oryza sativa", label: "水稻" }, // i18n-ignore
  { name: "Zea mays", label: "玉米" }, // i18n-ignore
  { name: "Triticum aestivum", label: "普通小麦" }, // i18n-ignore
  { name: "Hordeum vulgare", label: "大麦" }, // i18n-ignore
  { name: "Sorghum bicolor", label: "高粱" }, // i18n-ignore
  { name: "Setaria italica", label: "谷子" }, // i18n-ignore
  { name: "Setaria viridis", label: "狗尾草" }, // i18n-ignore
  { name: "Brachypodium distachyon", label: "二穗短柄草" }, // i18n-ignore
  { name: "Glycine max", label: "大豆" }, // i18n-ignore
  { name: "Medicago truncatula", label: "蒺藜苜蓿" }, // i18n-ignore
  { name: "Medicago sativa", label: "紫花苜蓿" }, // i18n-ignore
  { name: "Lotus japonicus", label: "百脉根" }, // i18n-ignore
  { name: "Arachis hypogaea", label: "花生" }, // i18n-ignore
  { name: "Phaseolus vulgaris", label: "菜豆" }, // i18n-ignore
  { name: "Solanum lycopersicum", label: "番茄" }, // i18n-ignore
  { name: "Solanum tuberosum", label: "马铃薯" }, // i18n-ignore
  { name: "Capsicum annuum", label: "辣椒" }, // i18n-ignore
  { name: "Nicotiana tabacum", label: "烟草" }, // i18n-ignore
  { name: "Nicotiana benthamiana", label: "本氏烟草" }, // i18n-ignore
  { name: "Brassica napus", label: "甘蓝型油菜" }, // i18n-ignore
  { name: "Brassica rapa", label: "白菜" }, // i18n-ignore
  { name: "Brassica oleracea", label: "甘蓝" }, // i18n-ignore
  { name: "Gossypium hirsutum", label: "陆地棉" }, // i18n-ignore
  { name: "Gossypium barbadense", label: "海岛棉" }, // i18n-ignore
  { name: "Cucumis sativus", label: "黄瓜" }, // i18n-ignore
  { name: "Citrullus lanatus", label: "西瓜" }, // i18n-ignore
  { name: "Daucus carota", label: "胡萝卜" }, // i18n-ignore
  { name: "Lactuca sativa", label: "莴苣" }, // i18n-ignore
  { name: "Populus trichocarpa", label: "毛果杨" }, // i18n-ignore
  { name: "Populus tremula", label: "欧洲山杨" }, // i18n-ignore
  { name: "Eucalyptus grandis", label: "巨桉" }, // i18n-ignore
  { name: "Camellia sinensis", label: "茶树" }, // i18n-ignore
  { name: "Vitis vinifera", label: "葡萄" }, // i18n-ignore
  { name: "Malus domestica", label: "苹果" }, // i18n-ignore
  { name: "Prunus persica", label: "桃" }, // i18n-ignore
  { name: "Fragaria vesca", label: "森林草莓" }, // i18n-ignore
  { name: "Citrus sinensis", label: "甜橙" }, // i18n-ignore
  { name: "Marchantia polymorpha", label: "地钱" }, // i18n-ignore
  { name: "Physcomitrium patens", label: "小立碗藓" }, // i18n-ignore
];

export function plantChoices(state?: LibraryState): SpeciesEntry[] {
  const choices = new Map(COMMON_PLANTS.map(s => [s.name.toLowerCase(), s]));
  for (const item of state ? [...state.papers, ...state.datasets] : []) {
    for (const entry of arr(item.species)) {
      const name = entry.name.trim();
      if (name && !choices.has(name.toLowerCase())) choices.set(name.toLowerCase(), { name, label: entry.label });
    }
  }
  return [...choices.values()];
}
