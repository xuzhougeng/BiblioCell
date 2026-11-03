import assert from "node:assert/strict";
import { test } from "node:test";
import { demoState } from "../src/demo-data.ts";
import { repositoryLabel, repositoryURL } from "../src/model.ts";
import { COMMON_PLANTS, plantChoices, rememberChoices, repositoryChoices, researchChoices } from "../src/research-options.ts";
import { mergeChecked, readRepository, speciesRow, selectPlant } from "../src/ui.ts";

test("custom research choices are reusable and deduplicated across papers and datasets", () => {
  const state = demoState();
  state.papers[0]!.modalities = ["Custom-seq", "scrna-seq"];
  state.datasets[0]!.modalities = ["custom-seq"];
  rememberChoices("platforms", ["New platform", "NEW PLATFORM"]);
  assert.equal(researchChoices(state, "modalities").filter(v => v.toLowerCase() === "custom-seq").length, 1);
  assert.equal(researchChoices(state, "modalities").filter(v => v.toLowerCase() === "scrna-seq").length, 1);
  assert.equal(researchChoices(state, "platforms").filter(v => v.toLowerCase() === "new platform").length, 1);
  const data = new FormData();
  data.append("modalities", "scRNA-seq");
  data.set("modalitiesCustom", "SCRNA-seq， Custom-seq; custom-seq");
  assert.deepEqual(mergeChecked(data, "modalities", "modalitiesCustom"), ["scRNA-seq", "Custom-seq"]);
});

test("custom repositories remain selectable and pending input is saved even without clicking add", () => {
  const state = demoState();
  rememberChoices("repositories", ["Plant Archive"]);
  const choices = repositoryChoices(state, "Legacy Repository");
  assert.equal(choices["plant archive"], "Plant Archive");
  assert.equal(choices["legacy repository"], "Legacy Repository");
  assert.equal(choices.stomics, "STOmics DB");
  const data = new FormData();
  data.set("repository", "geo");
  data.set("repositoryCustom", " Plant Archive ");
  assert.equal(readRepository(data, "repository"), "plant archive");
  assert.equal(repositoryLabel("constructor"), "constructor");
  assert.equal(repositoryURL({ repository: "constructor", accession: "X1", url: "" }), "");
});

test("plant dropdown pairs common names with scientific names and retains uncommon saved species", () => {
  const state = demoState();
  state.papers[0]!.species = [{ name: "Uncommon plant", label: "Custom label" }, { name: "Oryza sativa", label: "Rice" }];
  const choices = plantChoices(state);
  assert.ok(COMMON_PLANTS.length >= 40);
  assert.equal(choices.filter(s => s.name === "Oryza sativa").length, 1);
  assert.ok(choices.some(s => s.name === "Uncommon plant"));
  const html = speciesRow({ name: "Oryza sativa", label: "Rice" }, choices);
  assert.match(html, /<select[^>]+data-species-picker/);
  assert.match(html, /value="Oryza sativa" data-label="水稻" selected/);
  assert.match(html, /name="speciesLabel"[^>]+value="Rice"/); // Rendering keeps the person's label.
  const name = { value: "" }, label = { value: "" };
  const select = {
    value: "Oryza sativa", selectedOptions: [{ dataset: { label: "水稻" } }],
    closest: () => ({ querySelector: (selector: string) => selector === "[name=speciesName]" ? name : label }),
  } as unknown as HTMLSelectElement;
  selectPlant(select);
  assert.equal(name.value, "Oryza sativa");
  assert.equal(label.value, "水稻");
});
