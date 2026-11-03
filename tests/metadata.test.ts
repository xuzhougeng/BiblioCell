import assert from "node:assert/strict";
import { test } from "node:test";
import { demoState } from "../src/demo-data.ts";
import { metadataButton, metadataSelectedByDefault, metadataValue } from "../src/metadata.ts";
import type { MetadataClaim } from "../src/mygo.ts";
import type { App } from "../src/app.ts";

const claim = (values: Partial<MetadataClaim>) => ({ field: "platforms", values: ["Stereo-seq"], species: [], repository: "", accession: "", role: "", excerpt: "Evidence from full text.", ...values });

test("autofill defaults to missing fields and new dataset links", () => {
  const paper = demoState().papers[0]!;
  paper.platforms = [];
  assert.equal(metadataSelectedByDefault(paper, claim({}), []), true);
  paper.platforms = ["10x Genomics Chromium"];
  assert.equal(metadataSelectedByDefault(paper, claim({}), []), false);
  assert.equal(metadataSelectedByDefault(paper, claim({field:"source"}), []), false);
  const dataset = claim({field:"dataset", repository:"geo", accession:"GSE123"});
  assert.equal(metadataSelectedByDefault(paper, dataset, []), true);
  assert.equal(metadataSelectedByDefault(paper, dataset, [{repository:"GEO",accession:"GSE123"}]), false);
  assert.equal(metadataValue(claim({field:"species",species:[{name:"Oryza sativa",label:"水稻"}]})), "水稻 · Oryza sativa");
});

test("autofill requires a real desktop library, a configured model and stored full text", () => {
  const paper = demoState().papers[0]!;
  const app = { env: { real: true, desktop: true, settings: {models:[],tasks:{}} }, paperResources: () => [] } as unknown as App;
  assert.match(metadataButton(app, paper), /disabled/);
  app.env.settings.models = [{ id:"model",name:"Model",config:{protocol:"openai",model:"model",baseUrl:"http://localhost:1234"} }] as App["env"]["settings"]["models"];
  assert.match(metadataButton(app, paper), /disabled/);
  app.paperResources = () => [{type:"fulltext",localPath:"paper.pdf"}] as ReturnType<App["paperResources"]>;
  assert.doesNotMatch(metadataButton(app, paper), /disabled/);
  app.env.real = false;
  assert.equal(metadataButton(app, paper), "");
});
