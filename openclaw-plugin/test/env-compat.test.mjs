import { test } from "node:test";
import assert from "node:assert/strict";
import { productEnv } from "../index.js";

test("plugin settings accept both prefixes, preserving unset and empty values", () => {
  assert.equal(productEnv("WTC_OPTION", {}), undefined);
  assert.equal(productEnv("TABA_OPTION", { WTC_OPTION: "old" }), "old");
  assert.equal(productEnv("WTC_OPTION", { TABA_OPTION: "new" }), "new");
  const warnings = [];
  const warn = (message) => warnings.push(message);
  const env = { TABA_API_KEY: "new-secret", WTC_API_KEY: "old-secret" };
  assert.equal(productEnv("WTC_API_KEY", env, warn), "new-secret");
  assert.equal(productEnv("TABA_API_KEY", env, warn), "new-secret");
  assert.equal(productEnv("WTC_API_KEY", { ...env, TABA_API_KEY: "" }, warn), "");
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /using TABA_API_KEY/);
  assert.ok(!warnings[0].includes("secret"));
  assert.equal(productEnv("WTC_EQUAL", { TABA_EQUAL: "same", WTC_EQUAL: "same" }, () => assert.fail("equal values warned")), "same");
});

test("plugin test, development and unbranded controls are not aliased", () => {
  for (const name of ["PATH", "WTC_TEST_OPTION", "WTC_DEV_OPTION"]) {
    assert.equal(productEnv(name, { [name]: "original", [name.replace(/^WTC_/, "TABA_")]: "original" }), "original");
  }
  assert.equal(productEnv("WTC_TEST_OPTION", { TABA_TEST_OPTION: "new" }), undefined);
  assert.equal(productEnv("WTC_DEV_OPTION", { TABA_DEV_OPTION: "new" }), undefined);
});
