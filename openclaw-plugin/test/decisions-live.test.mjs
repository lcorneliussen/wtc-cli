// OpenAI's Decisions API on the question the card's project suggestion asks,
// for real. Runs only with WTC_DECISIONS_OPENAI_API_KEY set (CI passes its
// OpenAI secret); a few decisions cost a fraction of a cent.
import { test } from "node:test";
import assert from "node:assert/strict";
import { decideProjects, openaiDecisions } from "../index.js";

const apiKey = process.env.WTC_DECISIONS_OPENAI_API_KEY;
const PROJECTS = [
  { name: "wtc", repos: [
    { name: "wtc-site", role: "The official wtc site; what it says about wtc comes from wtc-cli." },
    { name: "wtc-cli", role: "The wtc CLI and OpenClaw plugin, what the site describes; its docs are the source of truth." },
  ] },
  { name: "shop", repos: [
    { name: "storefront", role: "The web shop's frontend: product pages, cart and checkout." },
    { name: "billing", role: "Invoices, payments and refunds." },
  ] },
];

test("OpenAI decides where clear messages ask to work", { skip: !apiKey && "no WTC_DECISIONS_OPENAI_API_KEY" }, async () => {
  const decisions = openaiDecisions({ apiKey, model: process.env.WTC_DECISIONS_MODEL || undefined });
  for (const [text, want] of [
    ["lets work on the site", "wtc/wtc-site"],
    ["wtc add-repo crashes with a nil pointer", "wtc/wtc-cli"],
    ["the invoices are rounded wrong", "shop/billing"],
    ["checkout page is slow on mobile", "shop/storefront"],
    ["what's the weather tomorrow?", "none"],
  ]) {
    const got = await decideProjects(decisions, "main", text, PROJECTS);
    const where = got?.none ? "none" : got ? `${got.project}/${got.repos[0]}` : "unavailable";
    assert.equal(where, want, text);
  }
});
