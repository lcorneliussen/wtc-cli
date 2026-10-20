// The gateway side of the plugin, where it needs no gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import plugin, { versionWarning } from "../index.js";

test("the panel warns when the gateway's wtc is not the release the plugin came from", () => {
  assert.equal(versionWarning("0.1.42", "wtc version 0.1.42\n"), null);
  assert.match(versionWarning("0.1.42", "wtc version 0.1.43\n"), /the wtc 0\.1\.42 plugin.*runs wtc 0\.1\.43.*wtc openclaw install/);
  assert.match(versionWarning("0.1.0", "wtc version v0.1.43"), /0\.1\.43/);
  // Dev builds on either side, a missing wtc and an unknown plugin version
  // say nothing.
  assert.equal(versionWarning("0.1.42", "wtc version dev"), null);
  assert.equal(versionWarning("0.1.42", "wtc version 0.1.43 (dev build, in place of 0.1.42)"), null);
  assert.equal(versionWarning("0.0.0-dev", "wtc version 0.1.43"), null);
  assert.equal(versionWarning("0.1.42", ""), null);
  assert.equal(versionWarning(null, "wtc version 0.1.43"), null);
});

test("overlapping collection requests share one wtc --version", async () => {
  const actions = new Map();
  let asked = 0;
  plugin.register({
    pluginConfig: {},
    runtime: {
      state: { resolveStateDir: () => "/nonexistent" },
      agent: { session: { listSessionEntries: () => [], getSessionEntry: async () => undefined } },
      system: {
        runCommandWithTimeout: async (argv) => {
          if (argv[1] === "--version") asked += 1;
          await new Promise((resolve) => setTimeout(resolve, 20));
          return { code: 0, stdout: "wtc version 99.0.0\n", stderr: "" };
        },
      },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const collection = () => actions.get("collection").handler({ sessionKey: "s", agentId: "main", payload: {} });
  const results = await Promise.all([collection(), collection(), collection()]);
  assert.equal(asked, 1);
  for (const r of results) assert.match(r.result.warnings[0], /runs wtc 99\.0\.0/);
});
