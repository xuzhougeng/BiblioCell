import assert from "node:assert/strict";
import { test } from "node:test";
import { demoState } from "../src/demo-data.ts";
import { matchingDatasets } from "../src/paper-fields.ts";
import { emptyRepository, normalizeDataset } from "../src/model.ts";

test("dataset picker searches metadata and secondary accessions, excluding already linked records", () => {
  const seed = demoState().datasets[0]!;
  const legacy = { ...seed, id: "legacy", paperLinks: [], repositories: [], repository: "geo", accession: "GSE123456" };
  const multiple = normalizeDataset({ ...seed, id: "multiple", paperLinks: [], repositories: [
    { ...emptyRepository(), repository: "geo", accession: "GSE765432" },
    { ...emptyRepository(), repository: "sra", accession: "SRP123456" },
  ] });
  const linked = { ...legacy, id: "linked", paperLinks: [{ paperId: "paper", role: "reused", evidenceUrl: "", evidenceText: "", verified: false }] };
  const datasets = [legacy, multiple, linked];
  assert.deepEqual(matchingDatasets(datasets, "paper", "  gse123456 ").map(d => d.id), ["legacy"]);
  assert.deepEqual(matchingDatasets(datasets, "paper", "srp123456").map(d => d.id), ["multiple"]);
  assert.equal(matchingDatasets(datasets, "paper", "Arabidopsis").length, 2);
  assert.equal(matchingDatasets(datasets, "paper", "拟南芥").length, 2);
  assert.equal(matchingDatasets(datasets, "paper", "missing").length, 0);
  assert.deepEqual(legacy.repositories, []); // Search must not modify stored records.
});
