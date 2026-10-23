// The gateway side of the plugin, where it needs no gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import plugin, { collectionFolder, pullRequestChanges, pullRequestWatch, versionWarning } from "../index.js";
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

test("a service action runs in the collection's folder, and says when there is none yet", async () => {
  const state = mkdtempSync(path.join(os.tmpdir(), "wtc-state-"));
  const actions = new Map();
  const cwds = [];
  const entries = [
    { sessionKey: "agent:main:c", entry: {} },
    { sessionKey: "agent:main:repo:api", entry: { parentSessionKey: "agent:main:c", projectId: "api", worktree: { id: "w1", branch: "openclaw/9-x" }, workspaceDir: "/worktrees/w1" } },
  ];
  plugin.register({
    pluginConfig: {},
    runtime: {
      state: { resolveStateDir: () => state },
      agent: { session: { listSessionEntries: () => entries, getSessionEntry: async () => undefined } },
      system: { runCommandWithTimeout: async (argv, options) => { cwds.push(options.cwd); return { code: 0, stdout: "", stderr: "" }; } },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const up = () => actions.get("svc").handler({ sessionKey: "agent:main:c", agentId: "main", payload: { op: "up" } });
  assert.match((await up()).error, /no folder yet/);
  mkdirSync(path.join(state, "wtc/platform/9-x"), { recursive: true });
  assert.equal((await up()).ok, true);
  assert.equal(cwds.at(-1), path.join(state, "wtc/platform/9-x"));
});

test("what changed on a pull request, one line each", () => {
  const pr = { number: 2, url: "https://github.com/o/site/pull/2", state: "open", draft: true, checks: "pending", failed: 0, review: "", mergeable: "mergeable" };
  const changes = (prev, cur) => pullRequestChanges("site", prev, cur);
  // The first look has nothing to compare with; a look without one is no news.
  assert.deepEqual(changes(undefined, pr), []);
  assert.deepEqual(changes(null, null), []);
  assert.deepEqual(changes(null, pr), ["site: pull request #2 (https://github.com/o/site/pull/2) opened as a draft."]);
  assert.deepEqual(changes(pr, pr), []);
  assert.deepEqual(changes(pr, { ...pr, draft: false, checks: "failure", failed: 2, review: "changes_requested", mergeable: "conflicting" }), [
    "site: pull request #2 (https://github.com/o/site/pull/2) is ready for review.",
    "site: checks failed on #2 (https://github.com/o/site/pull/2) (2 failed).",
    "site: a review requested changes on #2 (https://github.com/o/site/pull/2).",
    "site: pull request #2 (https://github.com/o/site/pull/2) conflicts with its base.",
  ]);
  const ready = { ...pr, draft: false };
  assert.deepEqual(changes(ready, pr), ["site: pull request #2 (https://github.com/o/site/pull/2) went back to draft."]);
  assert.deepEqual(changes(pr, { ...pr, review: "approved" }), ["site: pull request #2 (https://github.com/o/site/pull/2) was approved."]);
  assert.deepEqual(changes({ ...pr, state: "closed" }, pr), ["site: pull request #2 (https://github.com/o/site/pull/2) was reopened."]);
  assert.deepEqual(changes({ ...pr, checks: "failure" }, { ...pr, checks: "success" }), ["site: checks passed on #2 (https://github.com/o/site/pull/2) again."]);
  // Merged, the rest of it is no longer news.
  assert.deepEqual(changes(pr, { ...pr, state: "merged", checks: "failure" }), ["site: pull request #2 (https://github.com/o/site/pull/2) was merged."]);
  assert.deepEqual(changes(pr, { ...pr, state: "closed" }), ["site: pull request #2 (https://github.com/o/site/pull/2) was closed without merging."]);
  assert.deepEqual(changes(pr, { ...pr, number: 3, url: "u3", draft: false }), ["site: pull request #3 (u3) opened."]);
});

test("the pull requests seen are kept across restarts, and a failed look is no news", () => {
  const file = path.join(mkdtempSync(path.join(os.tmpdir(), "wtc-watch-")), "pull-requests.json");
  const pr = { number: 2, url: "u", state: "open", draft: true, checks: "none" };
  const watch = pullRequestWatch(file);
  assert.deepEqual(watch.observe("main", "agent:main:c", [{ path: "/w/site", repo: "site", pullRequest: null }]), []);
  assert.deepEqual(pullRequestWatch(file).observe("main", "agent:main:c", [{ path: "/w/site", repo: "site", pullRequest: pr }]), ["site: pull request #2 (u) opened as a draft."]);
  const again = pullRequestWatch(file);
  assert.deepEqual(again.observe("main", "agent:main:c", [{ path: "/w/site", repo: "site", pullRequest: null }]), []);
  assert.deepEqual(again.open().map((c) => c.sessionKey), ["agent:main:c"]);
  assert.deepEqual(again.observe("main", "agent:main:c", [{ path: "/w/site", repo: "site", pullRequest: { ...pr, state: "merged" } }]), ["site: pull request #2 (u) was merged."]);
  assert.deepEqual(again.open(), []);
});

test("the collection's conversation hears what changed, and is woken for it", async () => {
  const state = mkdtempSync(path.join(os.tmpdir(), "wtc-state-"));
  const actions = new Map();
  const services = [];
  const events = [];
  const wakes = [];
  let pr = null;
  const entries = [
    { sessionKey: "agent:main:c", entry: {} },
    { sessionKey: "agent:main:repo:site", entry: { parentSessionKey: "agent:main:c", projectId: "site", worktree: { id: "w1", branch: "openclaw/9-x" }, workspaceDir: "/worktrees/w1" } },
  ];
  const register = (pluginConfig) => plugin.register({
    pluginConfig,
    runtime: {
      state: { resolveStateDir: () => state },
      agent: { session: { listSessionEntries: () => entries, getSessionEntry: async () => undefined } },
      system: {
        runCommandWithTimeout: async (argv) => argv[1] === "status"
          ? { code: 0, stdout: JSON.stringify({ "9-x": { repos: [{ path: "/worktrees/w1", repo: "site", pullRequest: pr }] } }), stderr: "" }
          : { code: 0, stdout: "", stderr: "" },
        enqueueSystemEvent: (text, options) => events.push({ text, ...options }),
        requestHeartbeat: (options) => wakes.push(options),
      },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerService: (s) => services.push(s),
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  register({});
  const look = () => actions.get("collection").handler({ sessionKey: "agent:main:repo:site", agentId: "main", payload: {} });
  await look();
  pr = { number: 2, url: "u", state: "open", draft: true, checks: "none" };
  await look();
  assert.deepEqual(events, [{ text: "wtc: pull request news for this collection\n- site: pull request #2 (u) opened as a draft.", agentId: "main", sessionKey: "agent:main:c" }]);
  assert.deepEqual(wakes, [{ source: "other", intent: "immediate", reason: "wtc-pull-request", agentId: "main", sessionKey: "agent:main:c", heartbeat: { target: "last" } }]);
  // Merged while nobody looks: the service's next round finds it.
  assert.equal(services.at(-1).id, "pull-request-watch");
  pr = { ...pr, state: "merged" };
  const { mock } = await import("node:test");
  mock.timers.enable({ apis: ["setInterval"] });
  services.at(-1).start();
  mock.timers.tick(5 * 60000);
  await new Promise((resolve) => setImmediate(resolve));
  await new Promise((resolve) => setImmediate(resolve));
  services.at(-1).stop();
  mock.timers.reset();
  assert.match(events.at(-1).text, /#2 \(u\) was merged/);
  // queue tells without waking; off says nothing.
  register({ pullRequestEvents: "queue" });
  pr = { ...pr, number: 3, state: "open" };
  await look();
  assert.match(events.at(-1).text, /#3/);
  assert.equal(wakes.length, 2);
  register({ pullRequestEvents: "off" });
  pr = { ...pr, number: 4 };
  const before = events.length;
  await look();
  assert.equal(events.length, before);
});
