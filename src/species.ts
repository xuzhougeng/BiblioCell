import { locale, type Locale } from "./i18n.ts";
import type { SpeciesEntry } from "./mygo.ts";

// Canonical names and stored labels stay separate from localized display names.
export const COMMON_PLANTS: (SpeciesEntry & { englishLabel: string })[] = [
  { name: "Arabidopsis thaliana", label: "拟南芥", englishLabel: "Thale cress" }, // i18n-ignore
  { name: "Oryza sativa", label: "水稻", englishLabel: "Rice" }, // i18n-ignore
  { name: "Zea mays", label: "玉米", englishLabel: "Maize" }, // i18n-ignore
  { name: "Triticum aestivum", label: "普通小麦", englishLabel: "Bread wheat" }, // i18n-ignore
  { name: "Hordeum vulgare", label: "大麦", englishLabel: "Barley" }, // i18n-ignore
  { name: "Sorghum bicolor", label: "高粱", englishLabel: "Sorghum" }, // i18n-ignore
  { name: "Setaria italica", label: "谷子", englishLabel: "Foxtail millet" }, // i18n-ignore
  { name: "Setaria viridis", label: "狗尾草", englishLabel: "Green foxtail" }, // i18n-ignore
  { name: "Brachypodium distachyon", label: "二穗短柄草", englishLabel: "Purple false brome" }, // i18n-ignore
  { name: "Glycine max", label: "大豆", englishLabel: "Soybean" }, // i18n-ignore
  { name: "Medicago truncatula", label: "蒺藜苜蓿", englishLabel: "Barrel medic" }, // i18n-ignore
  { name: "Medicago sativa", label: "紫花苜蓿", englishLabel: "Alfalfa" }, // i18n-ignore
  { name: "Lotus japonicus", label: "百脉根", englishLabel: "Japanese lotus" }, // i18n-ignore
  { name: "Arachis hypogaea", label: "花生", englishLabel: "Peanut" }, // i18n-ignore
  { name: "Phaseolus vulgaris", label: "菜豆", englishLabel: "Common bean" }, // i18n-ignore
  { name: "Solanum lycopersicum", label: "番茄", englishLabel: "Tomato" }, // i18n-ignore
  { name: "Solanum tuberosum", label: "马铃薯", englishLabel: "Potato" }, // i18n-ignore
  { name: "Capsicum annuum", label: "辣椒", englishLabel: "Pepper" }, // i18n-ignore
  { name: "Nicotiana tabacum", label: "烟草", englishLabel: "Tobacco" }, // i18n-ignore
  { name: "Nicotiana benthamiana", label: "本氏烟草", englishLabel: "Benthami tobacco" }, // i18n-ignore
  { name: "Brassica napus", label: "甘蓝型油菜", englishLabel: "Rapeseed" }, // i18n-ignore
  { name: "Brassica rapa", label: "白菜", englishLabel: "Field mustard" }, // i18n-ignore
  { name: "Brassica oleracea", label: "甘蓝", englishLabel: "Cabbage" }, // i18n-ignore
  { name: "Gossypium hirsutum", label: "陆地棉", englishLabel: "Upland cotton" }, // i18n-ignore
  { name: "Gossypium barbadense", label: "海岛棉", englishLabel: "Sea island cotton" }, // i18n-ignore
  { name: "Cucumis sativus", label: "黄瓜", englishLabel: "Cucumber" }, // i18n-ignore
  { name: "Citrullus lanatus", label: "西瓜", englishLabel: "Watermelon" }, // i18n-ignore
  { name: "Daucus carota", label: "胡萝卜", englishLabel: "Carrot" }, // i18n-ignore
  { name: "Lactuca sativa", label: "莴苣", englishLabel: "Lettuce" }, // i18n-ignore
  { name: "Populus trichocarpa", label: "毛果杨", englishLabel: "Black cottonwood" }, // i18n-ignore
  { name: "Populus tremula", label: "欧洲山杨", englishLabel: "European aspen" }, // i18n-ignore
  { name: "Eucalyptus grandis", label: "巨桉", englishLabel: "Flooded gum" }, // i18n-ignore
  { name: "Camellia sinensis", label: "茶树", englishLabel: "Tea plant" }, // i18n-ignore
  { name: "Vitis vinifera", label: "葡萄", englishLabel: "Grape" }, // i18n-ignore
  { name: "Malus domestica", label: "苹果", englishLabel: "Apple" }, // i18n-ignore
  { name: "Prunus persica", label: "桃", englishLabel: "Peach" }, // i18n-ignore
  { name: "Fragaria vesca", label: "森林草莓", englishLabel: "Woodland strawberry" }, // i18n-ignore
  { name: "Citrus sinensis", label: "甜橙", englishLabel: "Sweet orange" }, // i18n-ignore
  { name: "Marchantia polymorpha", label: "地钱", englishLabel: "Common liverwort" }, // i18n-ignore
  { name: "Physcomitrium patens", label: "小立碗藓", englishLabel: "Spreading earthmoss" }, // i18n-ignore
];


const plantsByName = new Map(COMMON_PLANTS.map(plant => [plant.name.toLowerCase(), plant]));

/** Resolve translations by scientific name, including records with older common labels. */
export function speciesCommonName(entry: Partial<SpeciesEntry>, language: Locale = locale): string {
  const plant = plantsByName.get(entry.name?.trim().toLowerCase() ?? "");
  if (language === "en" && plant) return plant.englishLabel;
  return entry.label || entry.name || "";
}

/** Pair common and scientific names in English views and species pickers. */
export function speciesText(entry: Partial<SpeciesEntry>, includeScientific = locale === "en"): string {
  const common = speciesCommonName(entry);
  const name = entry.name?.trim() ?? "";
  return includeScientific && name && common !== name ? common + " · " + name : common;
}
