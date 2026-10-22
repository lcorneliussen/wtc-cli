// The gateway side of the plugin, where it needs no gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import plugin, { collectionFolder, versionWarning } from "../index.js";
import { mkdirSync, mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";

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

test("the branches of a gateway project are read outside its clone", async () => {
  const { mkdtempSync, mkdirSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const path = await import("node:path");
  const state = mkdtempSync(path.join(tmpdir(), "wtc-plugin-"));
  mkdirSync(path.join(state, "repos", "docs", ".git"), { recursive: true });
  const actions = new Map();
  const calls = [];
  let origin = "https://github.com/example-org/docs.git";
  plugin.register({
    pluginConfig: {},
    runtime: {
      state: { resolveStateDir: () => state },
      agent: { session: { listSessionEntries: () => [], getSessionEntry: async () => undefined } },
      system: {
        runCommandWithTimeout: async (argv) => {
          calls.push(argv);
          if (argv.includes("--get")) return { code: 0, stdout: origin + "\n", stderr: "" };
          return { code: 0, stdout: "ref: refs/heads/main\tHEAD\n" + "a".repeat(40) + "\trefs/heads/main\n", stderr: "" };
        },
      },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const branches = (payload) => actions.get("repo-branches").handler({ sessionKey: "s", agentId: "main", payload });
  const r = await branches({ project: "docs" });
  assert.deepEqual(r.result, { defaultBranch: "main", branches: ["main"] });
  const lsRemote = calls.find((argv) => argv.includes("ls-remote"));
  assert.ok(!lsRemote.includes("-C"), "ls-remote runs in no repository");
  assert.ok(lsRemote.includes(origin), "with the URL read from the clone");
  // A clone whose origin was pointed elsewhere is not read.
  origin = "ext::sh -c touch% /tmp/pwned";
  calls.length = 0;
  const refused = await branches({ project: "docs" });
  assert.equal(refused.ok, false);
  assert.ok(!calls.some((argv) => argv.includes("ls-remote")));
});

test("a collection's folder is where wtc made it", () => {
  const state = mkdtempSync(path.join(os.tmpdir(), "wtc-state-"));
  assert.equal(collectionFolder(state, "1-x"), null);
  for (const dir of ["wtc/platform/1-x", "wtc/ad-hoc/2-x", "wtc-collections/3-x"]) mkdirSync(path.join(state, dir), { recursive: true });
  assert.equal(collectionFolder(state, "1-x"), path.join(state, "wtc/platform/1-x"));
  assert.equal(collectionFolder(state, "2-x"), path.join(state, "wtc/ad-hoc/2-x"));
  assert.equal(collectionFolder(state, "3-x"), path.join(state, "wtc-collections/3-x"));
});
