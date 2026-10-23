// The gateway side of the plugin, where it needs no gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import plugin, { collectionFolder, decideProjects, matchProjects, openaiDecisions, pullRequestChanges, pullRequestWatch, versionWarning, wtcVersionOf } from "../index.js";
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

test("the collection names the gateway's wtc version", () => {
  assert.equal(wtcVersionOf("wtc version 0.1.49\n"), "0.1.49");
  assert.equal(wtcVersionOf("wtc version 0.1.50-dev+gb8e3804.20261023.0644"), "0.1.50-dev+gb8e3804.20261023.0644");
  assert.equal(wtcVersionOf(""), null);
  assert.equal(wtcVersionOf(null), null);
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
  assert.equal(results[0].result.version.wtc, "99.0.0");
});

test("a new conversation is a collection of its own, not its parent's", async () => {
  const actions = new Map();
  const entries = [
    { sessionKey: "agent:main:main", entry: { label: "[OpenClaw heartbeat poll]" } },
    { sessionKey: "agent:main:repo:api", entry: { parentSessionKey: "agent:main:main", projectId: "api", worktree: { id: "w1", branch: "openclaw/old" }, workspaceDir: "/worktrees/w1" } },
    { sessionKey: "agent:main:dashboard:new", entry: { parentSessionKey: "agent:main:main" } },
  ];
  plugin.register({
    pluginConfig: {},
    runtime: {
      state: { resolveStateDir: () => "/nonexistent" },
      agent: { session: { listSessionEntries: () => entries, getSessionEntry: async () => undefined } },
      system: { runCommandWithTimeout: async () => ({ code: 1, stdout: "", stderr: "" }) },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const collection = async (sessionKey) => (await actions.get("collection").handler({ sessionKey, agentId: "main", payload: {} })).result;
  const fresh = await collection("agent:main:dashboard:new");
  assert.deepEqual(fresh.collection, { sessionKey: "agent:main:dashboard:new", label: null });
  assert.equal(fresh.repos.length, 0);
  // A repository's own session still shows the collection it belongs to.
  assert.equal((await collection("agent:main:repo:api")).collection.sessionKey, "agent:main:main");
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
  assert.deepEqual(events, [{ text: "wtc: pull request news for this collection\n- site: pull request #2 (u) opened as a draft.\nBring the task's progress up to date with this.\nTell the person only what they need to act on.", agentId: "main", sessionKey: "agent:main:c" }]);
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
  assert.match(events.at(-1).text, /#2 \(u\) was merged\.\n.*progress.*\nAfter a merge, run wtc catch-up/);
  // First seen already merged: the catch-up hint too.
  pr = { ...pr, number: 5, url: "u5", state: "merged" };
  await look();
  assert.match(events.at(-1).text, /#5 \(u5\) was opened and merged\.\n.*\nAfter a merge, run wtc catch-up/);
  // queue tells without waking; off says nothing.
  register({ pullRequestEvents: "queue" });
  pr = { ...pr, number: 3, state: "open" };
  await look();
  assert.match(events.at(-1).text, /#3/);
  assert.equal(wakes.length, 3);
  register({ pullRequestEvents: "off" });
  pr = { ...pr, number: 4 };
  const before = events.length;
  await look();
  assert.equal(events.length, before);
});

test("the collection names its wtc project: its harness, or ad hoc by its folder", async () => {
  for (const [col, want] of [
    [{ harness: { name: "wtc", path: "/w" }, folder: "/state/wtc/wtc/9-x" }, "wtc"],
    [{ folder: "/state/wtc/ad-hoc/9-x" }, "ad-hoc"],
    [{ folder: "/state/wtc-collections/9-x" }, undefined],
  ]) {
    const actions = new Map();
    const entries = [
      { sessionKey: "agent:main:c", entry: {} },
      { sessionKey: "agent:main:repo:site", entry: { parentSessionKey: "agent:main:c", projectId: "site", worktree: { id: "w1", branch: "openclaw/9-x" }, workspaceDir: "/worktrees/w1" } },
    ];
    plugin.register({
      pluginConfig: {},
      runtime: {
        state: { resolveStateDir: () => "/nonexistent" },
        agent: { session: { listSessionEntries: () => entries, getSessionEntry: async () => undefined } },
        system: { runCommandWithTimeout: async (argv) => ({ code: 0, stdout: argv[1] === "status" ? JSON.stringify({ "9-x": { repos: [], ...col } }) : "", stderr: "" }) },
      },
      session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
      registerHttpRoute() {},
      on() {},
      logger: {},
    });
    const r = await actions.get("collection").handler({ sessionKey: "agent:main:c", agentId: "main", payload: {} });
    assert.equal(r.result.collection.project, want, JSON.stringify(col));
  }
});

const PROJECTS = [
  { name: "wtc", repos: [
    { name: "wtc-site", role: "The official wtc site; what it says about wtc comes from wtc-cli." },
    { name: "wtc-cli", role: "The wtc CLI and OpenClaw plugin, what the site describes." },
  ] },
  { name: "shop", repos: [{ name: "storefront", role: "The web shop" }, { name: "billing", role: "Invoices and payments" }] },
];

test("a message's words suggest a project and its repositories", () => {
  assert.deepEqual(matchProjects("lets work on the site", PROJECTS), { project: "wtc", repos: ["wtc-site"], source: "match" });
  assert.deepEqual(matchProjects("the invoices are wrong", PROJECTS), { project: "shop", repos: ["billing"], source: "match" });
  assert.deepEqual(matchProjects("fix the cli", PROJECTS).repos, ["wtc-cli"]);
  // The project's own name sets no repository of it apart.
  assert.deepEqual(matchProjects("fix the wtc cli", PROJECTS), { project: "wtc", repos: ["wtc-cli"], source: "match" });
  // A repository named like its project; a project named without a match.
  assert.deepEqual(matchProjects("fix the shop", [{ name: "shop", repos: [{ name: "shop" }] }]).repos, ["shop"]);
  assert.deepEqual(matchProjects("wtc payments", PROJECTS), { project: "shop", repos: ["billing"], source: "match" });
  assert.equal(matchProjects("hello there", PROJECTS), null);
  assert.equal(matchProjects("", PROJECTS), null);
});

test("the decision model chooses when the gateway selected one", async () => {
  const asked = [];
  const decisions = {
    evaluate: async (batch, opts) => {
      asked.push({ batch, opts });
      return { status: "ok", answers: { where: { choice: "shop/storefront", probabilities: { none: 0.05, "wtc/wtc-site": 0.02, "wtc/wtc-cli": 0.01, "shop/storefront": 0.6, "shop/billing": 0.32 } } } };
    },
  };
  assert.deepEqual(await decideProjects(decisions, "main", "the checkout page breaks", PROJECTS), { project: "shop", repos: ["storefront", "billing"], source: "decision" });
  const { batch, opts } = asked[0];
  assert.equal(batch.questions.where.type, "choice");
  assert.ok("none" in batch.questions.where.criteria && "wtc/wtc-site" in batch.questions.where.criteria);
  assert.equal(opts.agentId, "main");
  assert.equal(batch.state, "the checkout page breaks");
  // Contract 2: typed evidence.
  let v2;
  await decideProjects({ evaluateV2: async (b) => { v2 = b; return { status: "ok", result: { answers: { where: { type: "choice", choice: "wtc/wtc-site", probabilities: { "wtc/wtc-site": 0.9 } } } } }; }, evaluate: async () => { throw new Error("v1 used"); } }, "main", "the site", PROJECTS)
    .then((r) => assert.deepEqual(r, { project: "wtc", repos: ["wtc-site"], source: "decision" }));
  assert.deepEqual(v2.state, { type: "text", text: "the site" });
  // None chosen, unavailable, failing or absent: no suggestion from it.
  assert.deepEqual(await decideProjects({ evaluate: async () => ({ status: "ok", answers: { where: { choice: "none", probabilities: {} } } }) }, "main", "hi", PROJECTS), { none: true });
  assert.equal(await decideProjects({ evaluate: async () => ({ status: "unavailable", reason: "no model" }) }, "main", "site", PROJECTS), null);
  assert.equal(await decideProjects({ evaluate: async () => { throw new Error("down"); } }, "main", "site", PROJECTS), null);
  assert.equal(await decideProjects(undefined, "main", "site", PROJECTS), null);
});

test("the suggest action asks wtc projects once a minute, the decision model first", async () => {
  const actions = new Map();
  let projectRuns = 0;
  let decide = null;
  const agents = [];
  plugin.register({
    pluginConfig: {},
    runtime: {
      decisions: { evaluate: async (batch, opts) => { agents.push(opts.agentId); return decide; } },
      state: { resolveStateDir: () => "/nonexistent" },
      agent: { session: { listSessionEntries: () => [], getSessionEntry: async () => undefined } },
      system: { runCommandWithTimeout: async (argv) => {
        if (argv[1] === "projects") projectRuns += 1;
        return { code: 0, stdout: argv[1] === "projects" ? JSON.stringify({ ok: true, data: { projects: PROJECTS } }) : "", stderr: "" };
      } },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const suggest = (text) => actions.get("suggest").handler({ sessionKey: "s", agentId: "main", payload: { text } });
  // The model's choice wins over the words, and its "none" stands.
  decide = { status: "ok", answers: { where: { choice: "shop/billing", probabilities: { "shop/billing": 0.9 } } } };
  assert.deepEqual((await suggest("work on the site")).result, { project: "shop", repos: ["billing"], source: "decision" });
  decide = { status: "ok", answers: { where: { choice: "none", probabilities: {} } } };
  assert.equal((await suggest("work on the site")).result, null);
  assert.deepEqual(agents, ["main", "main"]);
  // Unavailable: the words.
  decide = { status: "unavailable", reason: "no model" };
  assert.deepEqual((await suggest("work on the site")).result, { project: "wtc", repos: ["wtc-site"], source: "match" });
  assert.equal((await suggest("hello")).result, null);
  assert.equal((await suggest("")).result, null);
  assert.equal(projectRuns, 1);
});

test("without a runtime (CLI metadata, setup) the plugin registers nothing", () => {
  for (const registrationMode of ["cli-metadata", "setup-only"]) {
    plugin.register({ registrationMode, get runtime() { throw new Error("runtime is intentionally unavailable"); }, session: { controls: { registerSessionAction() { throw new Error("registered"); } } } });
  }
});

test("OpenAI's Decisions API called directly answers like the gateway's runtime", async () => {
  const sent = [];
  const reply = (status, body) => async (url, init) => { sent.push({ url, init }); return { ok: status === 200, status, json: async () => body }; };
  const ok = openaiDecisions({ apiKey: "sk-test", fetchImpl: reply(200, { model: "gpt-6-luna", answers: [{ name: "where", type: "choice", choice: "shop/billing", confidence: 0.9, probabilities: [{ value: "none", probability: 0.05 }, { value: "shop/billing", probability: 0.8 }, { value: "shop/storefront", probability: 0.15 }] }] }) });
  assert.deepEqual(await decideProjects(ok, "main", "the invoices are wrong", PROJECTS), { project: "shop", repos: ["billing"], source: "decision" });
  const { url, init } = sent[0];
  assert.equal(url, "https://api.openai.com/v1/decisions");
  assert.equal(init.headers.authorization, "Bearer sk-test");
  const body = JSON.parse(init.body);
  assert.equal(body.model, "gpt-6-luna");
  assert.equal(body.input, "the invoices are wrong");
  assert.equal(body.questions[0].type, "choice");
  assert.ok(body.questions[0].choices.some((c) => c.value === "none") && body.questions[0].choices.some((c) => c.value === "wtc/wtc-site"));
  // Refused, failing or malformed: unavailable, so the words decide.
  for (const [status, body] of [[401, {}], [500, {}], [200, { answers: [] }]]) {
    assert.equal(await decideProjects(openaiDecisions({ apiKey: "k", fetchImpl: reply(status, body) }), "main", "site", PROJECTS), null);
  }
  assert.equal(await decideProjects(openaiDecisions({ apiKey: "k", fetchImpl: async () => { throw new Error("offline"); } }), "main", "site", PROJECTS), null);
});

test("the suggest action calls OpenAI itself with WTC_DECISIONS_OPENAI_API_KEY, and only then", async () => {
  const actions = new Map();
  plugin.register({
    pluginConfig: { decisionModel: "gpt-6-test" },
    runtime: {
      state: { resolveStateDir: () => "/nonexistent" },
      agent: { session: { listSessionEntries: () => [], getSessionEntry: async () => undefined } },
      system: { runCommandWithTimeout: async (argv) => ({ code: 0, stdout: argv[1] === "projects" ? JSON.stringify({ ok: true, data: { projects: PROJECTS } }) : "", stderr: "" }) },
    },
    session: { controls: { registerSessionAction: (a) => actions.set(a.id, a), registerControlUiDescriptor() {} } },
    registerHttpRoute() {},
    on() {},
    logger: {},
  });
  const realFetch = globalThis.fetch;
  const models = [];
  globalThis.fetch = async (url, init) => {
    models.push(JSON.parse(init.body).model);
    return { ok: true, status: 200, json: async () => ({ model: "gpt-6-test", answers: [{ name: "where", type: "choice", choice: "none", probabilities: [{ value: "none", probability: 0.9 }] }] }) };
  };
  const was = process.env.WTC_DECISIONS_OPENAI_API_KEY;
  try {
    const suggest = () => actions.get("suggest").handler({ sessionKey: "s", agentId: "main", payload: { text: "work on the site" } });
    delete process.env.WTC_DECISIONS_OPENAI_API_KEY;
    assert.equal((await suggest()).result.source, "match");
    assert.deepEqual(models, []);
    process.env.WTC_DECISIONS_OPENAI_API_KEY = "sk-test";
    // Its "none" stands over the words.
    assert.equal((await suggest()).result, null);
    assert.deepEqual(models, ["gpt-6-test"]);
  } finally {
    globalThis.fetch = realFetch;
    if (was === undefined) delete process.env.WTC_DECISIONS_OPENAI_API_KEY; else process.env.WTC_DECISIONS_OPENAI_API_KEY = was;
  }
});
