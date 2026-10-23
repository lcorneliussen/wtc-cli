// Renders the Control UI's collection panel and composer card against a
// minimal DOM, so that a view which throws while rendering fails here rather
// than in someone's browser. Run with `node --test openclaw-plugin/test/`.
import { test } from "node:test";
import assert from "node:assert/strict";

// Just enough of the DOM for el(), css() and the views: elements with
// children, text, attributes and style; nothing is laid out.
class Node {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.children = [];
    this.parentNode = null;
    this.attributes = {};
    this.style = { cssText: "" };
    this.listeners = {};
  }
  get childNodes() { return this.children; }
  get textContent() { return this.children.map((c) => (typeof c === "string" ? c : c.textContent)).join(""); }
  set textContent(text) { this.replaceChildren(String(text ?? "")); }
  get isConnected() {
    let n = this;
    while (n.parentNode) n = n.parentNode;
    return n.root === true;
  }
  #adopt(kids) {
    return kids.map((k) => {
      if (typeof k !== "object") return String(k);
      k.remove();
      k.parentNode = this;
      return k;
    });
  }
  append(...kids) { this.children.push(...this.#adopt(kids)); }
  prepend(...kids) { this.children.unshift(...this.#adopt(kids)); }
  replaceChildren(...kids) {
    for (const c of this.children) if (typeof c === "object") c.parentNode = null;
    this.children = [];
    this.append(...kids);
  }
  remove() {
    if (!this.parentNode) return;
    const siblings = this.parentNode.children;
    siblings.splice(siblings.indexOf(this), 1);
    this.parentNode = null;
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn); }
  removeEventListener(type, fn) { this.listeners[type] = (this.listeners[type] ?? []).filter((f) => f !== fn); }
  // Every element below this one, depth first.
  *all() {
    for (const c of this.children) if (typeof c === "object") { yield c; yield* c.all(); }
  }
}

// A view whose test failed is never disposed; its refresh must not keep the
// run alive.
const setIntervalOf = globalThis.setInterval;
globalThis.setInterval = (fn, ms) => setIntervalOf(fn, ms).unref();

globalThis.document = {
  hidden: false,
  createElement: (tag) => new Node(tag),
  createElementNS: (_ns, tag) => new Node(tag),
  addEventListener() {},
  removeEventListener() {},
};

const { default: plugin } = await import("../dist/control-ui/v1/index.js");

// The plugin's panel and composer mounts, registered the way the Control UI
// registers them.
const mounts = {};
plugin.activate({
  request: async () => ({}),
  ui: {
    registerPanel: (p) => { mounts.panel = p.mount; },
    registerReplacement: (r) => { mounts.composer = r.mount; },
    registerAction: () => {},
    selectReplacement: () => {},
    openPanel: () => {},
  },
});

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const container = () => Object.assign(new Node("div"), { root: true });
const texts = (node) => [...node.all()].map((n) => n.textContent);
const buttons = (node) => [...node.all()].filter((n) => n.tagName === "BUTTON").map((n) => n.textContent);
const titled = (node, title) => [...node.all()].find((n) => n.tagName === "BUTTON" && n.title === title);

// Every collection request is answered from `collections`, by session;
// wtc svc requests are recorded in `calls` and answered with `svcResult`.
const collections = new Map();
const calls = [];
let svcResult = { ok: true, result: { message: "done" } };
let portals = [];
// The person's own GitHub connection, when a test gives them one.
let personal = null;
// The Bitbucket plugin: absent unless a test installs it with a login (or null).
let bitbucket;
const hostRequest = async (method, params) => {
  if (method === "plugins.sessionAction" && params.pluginId === "bitbucket") {
    if (bitbucket === undefined) throw new Error(`unknown plugin session action: bitbucket/${params.actionId}`);
    if (params.actionId === "publish-options") return { ok: true, result: { account: bitbucket } };
    calls.push({ method, ...params });
    if (params.actionId === "publish") {
      return { ok: true, result: bitbucket ? { status: "needs_confirmation", requestId: "bb-1", confirmation: { account: bitbucket.user, name: bitbucket.name, requestDigest: "bd1", branch: "openclaw/test", head: "abcdef0123", dest: "main", uncommitted: 0 } } : { status: "not_connected" } };
    }
    if (params.actionId === "publish-confirm") return { ok: true, result: { status: "published", created: true, url: "https://bitbucket.org/ws/hooks/pull-requests/3" } };
  }
  if (method === "sessions.github.options") return { personal, shared: { source: "system-detected", login: "octo-bot" } };
  if (method === "sessions.github.status") return { confirmation: { generation: personal?.generation, account: personal?.account, requestDigest: "d1" } };
  if (method === "plugins.sessionAction" && params.actionId === "collection") {
    const data = collections.get(params.sessionKey);
    return data instanceof Error ? { ok: false, error: data.message } : { ok: true, result: data };
  }
  if (method === "portal.list") return { portals };
  calls.push({ method, ...params });
  if (method === "plugins.sessionAction" && params.actionId === "svc") return svcResult;
  if (method === "sessions.github.publish") return { status: "needs_confirmation", requestId: "req-1" };
  if (method === "sessions.github.confirm") return { status: "published", url: "https://github.com/octo/api/pull/7" };
  return {};
};

let sessions = 0;
async function mount(kind, data) {
  const sessionKey = `agent:main:test:${++sessions}`;
  collections.set(sessionKey, data);
  const host = container();
  const ctx = { props: { sessionKey, agentId: "main" }, signal: new AbortController().signal, presented: true, mountDefault: () => () => {} };
  ctx.host = { request: hostRequest, ui: { openPanel() {} } };
  const view = mounts[kind](host, ctx);
  await settle();
  return { host, view };
}

const collection = (repos, services = []) => ({
  collection: { label: "test collection", sessionKey: "agent:main:test", folder: "/state/wtc-collections/test" },
  repos,
  services,
});
const repo = (name, fields = {}) => ({
  repo: name, owner: "octo", forge: "github", sessionKey: `agent:main:owner:${name}`,
  branch: "openclaw/test", base: "origin/main", ahead: 0, behind: 0, ...fields,
});
const busy = collection([
  repo("api", {
    ahead: 2, unpublished: 2, behind: 3,
    pullRequest: { number: 7, state: "open", title: "Add paging", url: "https://github.com/octo/api/pull/7",
      checks: "failure", passed: 1, failed: 1, running: 0, mergeable: "conflicting", review: "review_required" },
  }),
  repo("web", { dirty: 1 }),
  repo("hooks", { forge: "bitbucket", ahead: 1, unpublished: 1 }),
], [
  { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" },
  { group: "api/web", path: "api/web/cloudflare", repo: "api", kind: "endpoint", provider: "cloudflare", state: "ready", url: "https://calm-river.trycloudflare.com" },
  { group: "api/worker", path: "api/worker", repo: "api", kind: "service", state: "exited", exit_code: 1 },
]);

// The card's cells, row by row: a table is a grid of a header and rows of
// as many cells, as in wtc status.
const tables = (node) => [...node.all()].filter((n) => n.className === "wtc-table").map((t) => {
  const cells = t.children;
  const width = t.style.cssText.match(/grid-template-columns:([^;]*)/)[1].split(" ").filter((w) => w && !/^\d|^\)|^minmax\(0,$/.test(w)).length;
  const rows = [];
  for (let i = 0; i < cells.length; i += width) rows.push(cells.slice(i, i + width).map((c) => {
    // A choice reads as its prompt, not its options.
    const pick = [...c.all()].find((n) => n.tagName === "SELECT");
    return pick ? pick.children[0].textContent : c.textContent;
  }));
  return rows;
});

test("panel: no collection yet says what one is, not the conversation's label", async () => {
  const { host, view } = await mount("panel", { ...collection([]), collection: { label: "[OpenClaw heartbeat poll]", sessionKey: "agent:main:dashboard:new" }, version: { plugin: "0.1.49-dev+317ca2a.202610230912", wtc: "0.1.49-dev+317ca2a.202610230912" } });
  const all = texts(host);
  assert.ok(all.includes("No collection yet"));
  assert.ok(all.some((t) => /^A collection is this conversation's worktrees of a wtc project's repositories/.test(t ?? "")));
  assert.ok(all.includes("Add a repository"));
  assert.ok(!all.some((t) => /heartbeat poll|^null$|0 repositories|None running/.test(t ?? "")));
  assert.ok(all.includes("plugin 0.1.49-dev+317ca2a.202610230912, wtc 0.1.49-dev+317ca2a.202610230912"));
  view.dispose();
});

test("panel: a busy collection", async () => {
  const { host, view } = await mount("panel", busy);
  const all = texts(host);
  // The shared branch once in the header, every repository's in its row.
  assert.ok(all.includes("3 repositories · 1/2 services running"));
  const [repos, prs, services] = tables(host);
  assert.deepEqual(repos[1], ["api", "openclaw/test", "·", "↑ 2", "↓ 3", "#7", "✗", "✗", "○"]);
  // wtc status's PRs: number, repository, state, C M R, title.
  assert.deepEqual(prs, [["PR", "Repo", "State", "C", "M", "R", "Title"], ["#7", "api", "open", "✗", "✗", "○", "Add paging"]]);
  assert.deepEqual(services[1].slice(1), ["preview", "api", "ready", ":42201", "calm-river✕", "+"]);
  assert.ok(titled(host, "preview is running. Stop it"));
  assert.ok(titled(host, "worker: exited (exit 1). Restart it"));
  assert.ok(titled(host, "Start all services"));
  assert.ok(titled(host, "Stop all services"));
  assert.ok(titled(host, "Close the cloudflare tunnel"));
  view.dispose();
});

test("panel: repositories on different branches", async () => {
  const { host, view } = await mount("panel", collection([
    repo("api"), repo("web"), repo("docs", { branch: "release" }),
  ]));
  assert.deepEqual(tables(host)[0].slice(1).map((r) => r[1]), ["openclaw/test", "openclaw/test", "release"]);
  view.dispose();
});

test("panel: a collection that cannot be loaded", async () => {
  const { host, view } = await mount("panel", new Error("wtc status failed"));
  assert.ok(texts(host).includes("The collection could not be loaded: wtc status failed"));
  view.dispose();
});

test("panel: a tunnel started as a service of its own", async () => {
  const { host, view } = await mount("panel", collection([repo("api")], [
    { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" },
    { group: "api/share", path: "api/share/main", repo: "api", kind: "service", state: "ready", url: "https://calm-river.trycloudflare.com" },
  ]));
  const all = texts(host);
  assert.equal([...host.all()].filter((n) => n.title === "Started apart from the service it exposes").length, 1);
  assert.ok(buttons(host).includes("Consolidate"));
  view.dispose();
});

test("composer: no card for a session without repositories", async () => {
  const { host, view } = await mount("composer", collection([]));
  assert.equal([...host.all()].some((n) => n.className === "wtc-collection-card"), false);
  view.dispose();
});

test("composer: the card for a busy collection", async () => {
  const { host, view } = await mount("composer", busy);
  const card = [...host.all()].find((n) => n.className === "wtc-collection-card");
  assert.ok(card, "the card is shown");
  assert.equal(host.children[0], card, "above the message box");
  const [repos, services] = tables(card);
  // A column per fact, in its place; "·" where there is none.
  assert.deepEqual(repos, [
    ["Repo", "Branch", "±", "↑", "↓", "PR", "C", "M", "R"],
    ["api", "openclaw/test", "·", "↑ 2", "↓ 3", "#7", "✗", "✗", "○"],
    ["web", "openclaw/test", "1", "↑", "·", "·", "·", "·", "·"],
    ["hooks", "openclaw/test", "·", "↑ 1", "·", "·", "·", "·", "·"],
  ]);
  // A service and its tunnel are one row; the tunnel has its column.
  assert.deepEqual(services.map((r) => r.slice(1)), [
    ["Service", "Repo", "State", "URL", "cloudflare", ""],
    ["preview", "api", "ready", ":42201", "calm-river✕", "+"],
    ["worker", "api", "exited (exit 1)", "·", "·", "+"],
  ]);
  assert.ok(titled(card, "preview is running. Stop it"));
  assert.ok(titled(card, "Start all services"));
  assert.ok(titled(card, "Close the cloudflare tunnel"));
  view.dispose();
});

test("composer: the actions are on the facts they change", async () => {
  const { host, view } = await mount("composer", busy);
  const card = [...host.all()].find((n) => n.className === "wtc-collection-card");
  const named = (re) => [...card.all()].find((n) => n.tagName === "BUTTON" && re.test(n.title));
  calls.length = 0;
  await named(/^3 new on main\. Catch up/).onclick();
  assert.deepEqual(calls.map((c) => c.actionId), ["catch-up"]);
  await named(/^#7 conflicts with main\. Resolve/).onclick();
  assert.match(calls.at(-1).message, /Pull request #7 of api conflicts.*Don't push/);
  // Without the Bitbucket plugin's publish, Bitbucket is not pushed from
  // here: pushing is never handed to the agent.
  assert.equal(named(/^1 commit to push/), undefined);
  view.dispose();
});

test("composer: Bitbucket is pushed by the Bitbucket plugin, with the person's own login, after they confirm", async () => {
  bitbucket = null;
  const { host, view } = await mount("composer", busy);
  await settle();
  const card = () => [...host.all()].find((n) => n.className === "wtc-collection-card");
  const push = () => [...card().all()].find((n) => n.tagName === "BUTTON" && /^1 commit to push/.test(n.title));
  calls.length = 0;
  await push().onclick();
  await settle();
  // Not connected: the card says where to connect, and nothing is pushed.
  const howTo = [...card().all()].find((n) => n.className === "wtc-connect");
  assert.match(howTo.textContent, /Pushing needs your own Bitbucket login/);
  assert.ok([...howTo.all()].some((n) => n.tagName === "A" && n.href === "/plugins/bitbucket/connect"));
  assert.equal(calls.some((c) => c.actionId === "publish-confirm"), false);
  // Connected: asking names the commit and account, and confirming pushes.
  bitbucket = { user: "ada@example.com", name: "Ada" };
  await push().onclick();
  await settle();
  assert.deepEqual(calls.at(-1).payload, { draft: true, dest: "main" });
  const confirm = [...card().all()].find((n) => n.tagName === "BUTTON" && n.textContent === "confirm ↑ 1");
  assert.equal(confirm.title, "Push 1 commit of hooks as Ada (ada@example.com)");
  await confirm.onclick();
  await settle();
  assert.deepEqual(calls.at(-1), { method: "plugins.sessionAction", pluginId: "bitbucket", actionId: "publish-confirm", sessionKey: "agent:main:owner:hooks", agentId: "main", payload: { requestId: "bb-1", requestDigest: "bd1" } });
  bitbucket = undefined;
  view.dispose();
});

test("composer: a push is the person's, with their own account, after they confirm", async () => {
  const { host, view } = await mount("composer", busy);
  const card = () => [...host.all()].find((n) => n.className === "wtc-collection-card");
  const push = () => [...card().all()].find((n) => n.tagName === "BUTTON" && /^2 commits to push/.test(n.title));
  // Without their own connection, nothing is pushed: the shared account is
  // never used.
  calls.length = 0;
  personal = null;
  await push().onclick();
  assert.equal(calls.some((c) => c.method === "sessions.github.publish"), false);
  // The card says how to connect it, linking to the profile page.
  const howTo = [...card().all()].find((n) => n.className === "wtc-connect");
  assert.ok(howTo, "how to connect is shown");
  assert.ok([...howTo.all()].some((n) => n.tagName === "A" && n.href === "/settings/profile"));
  assert.match(howTo.textContent, /For me \(not For the system\)/);
  // With it, publishing asks for a confirmation, which the ↑ turns into.
  personal = { state: "connected", account: { id: 1, login: "octo" }, generation: "g1" };
  await push().onclick();
  await settle();
  assert.deepEqual(calls.find((c) => c.method === "sessions.github.publish").selection, { source: "personal", account: { id: 1, login: "octo" }, generation: "g1" });
  const confirm = [...card().all()].find((n) => n.tagName === "BUTTON" && n.textContent === "confirm ↑ 2");
  assert.equal(confirm.title, "Push 2 commits of api as @octo");
  await confirm.onclick();
  await settle();
  assert.deepEqual(calls.find((c) => c.method === "sessions.github.confirm"),
    { method: "sessions.github.confirm", sessionKey: "agent:main:owner:api", agentId: "main", requestId: "req-1", generation: "g1", account: { id: 1, login: "octo" }, requestDigest: "d1" });
  personal = null;
  view.dispose();
});

test("composer: a new pull request opens with the title and description the agent proposed", async () => {
  personal = { state: "connected", account: { id: 1, login: "octo" }, generation: "g1" };
  bitbucket = { user: "ada@example.com", name: "Ada" };
  const proposal = { branch: "openclaw/test", title: "Add a web preview", body: "What, why, and how it was checked." };
  const data = collection([
    repo("web", { dirty: 1, proposal }),
    repo("hooks", { forge: "bitbucket", ahead: 1, unpublished: 1, proposal }),
    repo("docs", { ahead: 1 }),
    // An open pull request keeps its text: the gateway takes it only when it opens one.
    repo("api", { ahead: 1, unpublished: 1, proposal, pullRequest: { number: 7, state: "open", url: "https://github.com/octo/api/pull/7", checks: "none" } }),
  ]);
  const { host, view } = await mount("composer", data);
  await settle();
  const card = () => [...host.all()].find((n) => n.className === "wtc-collection-card");
  const push = (what) => [...card().all()].find((n) => n.tagName === "BUTTON" && n.title.startsWith(what));
  assert.match(push("1 file to push").title, /Title: "Add a web preview", with the agent's description\./);
  assert.match([...card().all()].find((n) => n.tagName === "BUTTON" && /Bitbucket/.test(n.title)).title, /Title: "Add a web preview"\.$/);
  assert.match(push("1 commit to push. Commit what is pending, push the branch and open a draft pull request, with your own GitHub").title, /proposed no title or description/);
  assert.doesNotMatch(push("1 commit to push. Commit what is pending and push it to #7").title, /Title|proposed/);
  calls.length = 0;
  await push("1 file to push").onclick();
  await settle();
  const published = calls.find((c) => c.method === "sessions.github.publish");
  assert.equal(published.title, "Add a web preview");
  assert.equal(published.body, "What, why, and how it was checked.");
  await push("1 commit to push. Commit what is pending and push it to #7").onclick();
  await settle();
  const again = calls.filter((c) => c.method === "sessions.github.publish").at(-1);
  assert.equal("title" in again || "body" in again, false);
  const bb = [...card().all()].find((n) => n.tagName === "BUTTON" && /^1 commit to push.*Bitbucket/.test(n.title));
  await bb.onclick();
  await settle();
  assert.deepEqual(calls.at(-1).payload, { draft: true, dest: "main", title: "Add a web preview" });
  bitbucket = undefined;
  personal = null;
  view.dispose();
});

test("composer: a repository without a branch yet", async () => {
  const { host, view } = await mount("composer", collection([repo("api", { branch: null })]));
  assert.ok(texts(host).includes("at main"));
  view.dispose();
});

test("panel: play, stop and start all run wtc svc", async () => {
  const { host, view } = await mount("panel", collection([repo("api")], [
    { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "absent" },
    { group: "api/docs", path: "api/docs/main", repo: "api", kind: "service", label: "docs", state: "ready", url: "http://127.0.0.1:42202" },
  ]));
  calls.length = 0;
  titled(host, "preview: absent. Start it").onclick({ preventDefault() {}, stopPropagation() {} });
  await settle();
  titled(host, "docs is running. Stop it").onclick({ preventDefault() {}, stopPropagation() {} });
  await settle();
  titled(host, "Start all services").onclick({ preventDefault() {}, stopPropagation() {} });
  await settle();
  assert.deepEqual(calls.filter((c) => c.actionId === "svc").map((c) => c.payload),
    [{ op: "up", group: "api/web" }, { op: "down", group: "api/docs" }, { op: "up" }]);
  view.dispose();
});

test("panel: a new tunnel, and one whose client is missing", async () => {
  const { host, view } = await mount("panel", collection([repo("api")], [
    { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" },
  ]));
  const pick = [...host.all()].find((n) => n.tagName === "SELECT");
  // The gateway's portal first; without tunnel clients, the others are
  // installed first, and Tailscale is not offered on a personal tailnet.
  assert.deepEqual(pick.children.map((o) => o.value), ["", "gateway", "cloudflare", "ngrok", "devtunnel"]);
  assert.equal(pick.children[0].textContent, "+");
  assert.equal(pick.children[1].textContent, "gateway · OpenClaw portal");
  assert.equal(pick.children[2].textContent, "cloudflare · public link · install first");
  // Only reachable on the gateway: the port, not a link.
  assert.ok(texts(host).includes(":42201"));
  calls.length = 0;
  svcResult = { ok: true, result: { missing: "cloudflared" } };
  pick.value = "cloudflare";
  await pick.onchange();
  svcResult = { ok: true, result: { message: "done" } };
  assert.deepEqual(calls[0].payload, { op: "tunnel", group: "api/web", provider: "cloudflare", public: true, add: true });
  const asked = calls.find((c) => c.method === "sessions.send");
  assert.match(asked.message, /cloudflare tunnel to api\/web\. cloudflared is not on the gateway's PATH/);
  view.dispose();
});

test("composer: start and stop all only with more than one service", async () => {
  const one = await mount("composer", collection([repo("api")], [
    { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "absent" },
  ]));
  assert.ok(titled(one.host, "preview: absent. Start it"));
  assert.equal(titled(one.host, "Start all services"), undefined);
  one.view.dispose();
});

test("panel: the ways to a service", async () => {
  const web = { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" };
  const team = await mount("panel", { ...collection([repo("api")], [web]), tunnelProviders: ["tailscale", "cloudflare"], teamTailnet: true });
  const pick = [...team.host.all()].find((n) => n.tagName === "SELECT");
  assert.deepEqual(pick.children.map((o) => o.textContent),
    ["+", "gateway · OpenClaw portal", "cloudflare · public link", "ngrok · public link · install first", "devtunnel · your account · install first", "tailscale · team tailnet"]);
  team.view.dispose();
  // Without portal ingress, the gateway's portal comes last, marked.
  const bare = await mount("panel", { ...collection([repo("api")], [web]), tunnelProviders: ["cloudflare"], portalIngress: false });
  const choice = [...bare.host.all()].find((n) => n.tagName === "SELECT");
  assert.deepEqual(choice.children.map((o) => o.textContent),
    ["+", "cloudflare · public link", "ngrok · public link · install first", "devtunnel · your account · install first", "gateway · OpenClaw portal · needs ingress"]);
  bare.view.dispose();
  // With every other way taken, the one left is named.
  const tunnels = ["cloudflare", "ngrok", "devtunnel"].map((p) => ({ group: "api/web", path: `api/web/${p}`, repo: "api", kind: "endpoint", provider: p, state: "idle" }));
  const last = await mount("panel", { ...collection([repo("api")], [web, ...tunnels]), tunnelProviders: [] });
  assert.ok([...last.host.all()].some((n) => n.tagName === "BUTTON" && n.textContent === "+" && n.title.startsWith("Open gateway (OpenClaw portal)")), "one way left: no choice to make");
  last.view.dispose();
});

test("panel: a portal of the gateway beside its service", async () => {
  const web = { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" };
  portals = [{ id: "p42201", title: "preview", port: 42201, listenPort: 41553, url: "http://172.30.0.2:41553/?openclaw_portal=t", publicUrl: "http://172.30.0.2:41553/", createdAtMs: 0 }];
  const open = await mount("panel", collection([repo("api")], [web]));
  const links = [...open.host.all()].filter((n) => n.tagName === "A").map((n) => n.textContent);
  // In the gateway column; a container's address, which a browser elsewhere
  // cannot reach, is marked.
  assert.ok(links.includes("open ⚠"));
  assert.ok(tables(open.host).at(-1)[0].includes("gateway"));
  // Listed without its URL, it opens again on a click.
  portals = [{ ...portals[0], url: undefined, id: "p42202" }];
  const listed = await mount("panel", collection([repo("api")], [web]));
  assert.ok(titled(listed.host, "Open the portal"));
  listed.view.dispose();
  portals = [{ ...portals[0], id: "p42201", url: "http://172.30.0.2:41553/?openclaw_portal=t" }];
  // Open already: not offered again.
  assert.equal([...open.host.all()].find((n) => n.tagName === "SELECT").children.some((o) => o.value === "gateway"), false);
  calls.length = 0;
  titled(open.host, "Close the portal").onclick({ preventDefault() {}, stopPropagation() {} });
  await settle();
  assert.deepEqual(calls.find((c) => c.method === "portal.close"), { method: "portal.close", id: "p42201" });
  portals = [];
  open.view.dispose();
});

test("composer: collapsed, the same facts summed", async () => {
  const { host, view } = await mount("composer", busy);
  const card = [...host.all()].find((n) => n.className === "wtc-collection-card");
  const summary = card.children.find((n) => n.tagName === "SUMMARY");
  assert.equal(summary.children[1].textContent, "±1↑ 2↓ 3PR1 ✗svc▶1/2");
  // ↑ pushes every repository, ↓ catches every one up.
  assert.ok(titled(summary, "Push api, web, each to its pull request or a new draft one"));
  assert.ok([...summary.all()].some((n) => n.tagName === "BUTTON" && /^Catch up api \(3\)/.test(n.title)));
  // Open, the tables have them and the summary does not repeat them.
  card.open = true;
  for (const fn of card.listeners.toggle) fn();
  assert.equal(summary.children[1].textContent, "");
  view.dispose();
});

test("composer: after a merge, ↓ brings the merge in", async () => {
  const merged = (mergedIn) => ({ number: 2, state: "merged", url: "https://github.com/octo/site/pull/2", checks: "success", mergedIn });
  const { host, view } = await mount("composer", collection([repo("site", { pullRequest: merged(false) }), repo("docs", { pullRequest: merged(true) })]));
  await settle();
  const card = [...host.all()].find((n) => n.className === "wtc-collection-card");
  const downs = [...card.all()].filter((n) => n.tagName === "BUTTON" && n.textContent === "↓");
  assert.ok(downs.some((n) => n.title === "#2 was merged. Catch up: fetch main and merge it in, to continue from the merge; never rebases, never pushes"));
  // Collapsed, the summary offers it too; one that has the merge needs none.
  assert.ok(downs.some((n) => n.title.startsWith("Catch up site (#2 merged)")));
  assert.ok(!downs.some((n) => /docs/.test(n.title)));
  calls.length = 0;
  await downs.find((n) => n.title.startsWith("#2 was merged")).onclick();
  await settle();
  assert.ok(calls.some((c) => c.actionId === "catch-up" && c.sessionKey === "agent:main:owner:site"));
  view.dispose();
});

test("narrow: one header, every fact in its column, the branch below the name", async () => {
  // A ResizeObserver whose width the test sets.
  const observers = [];
  globalThis.ResizeObserver = class { constructor(fn) { this.fn = fn; observers.push(this); } observe() {} disconnect() {} };
  try {
    const { host, view } = await mount("panel", busy);
    for (const o of observers) o.fn([{ contentRect: { width: 360 } }]);
    const [repos, , services] = tables(host);
    // The branch column folds under the repository; the rest stays as wide.
    assert.deepEqual(repos[0], ["Repo", "±", "↑", "↓", "PR", "C", "M", "R"]);
    assert.deepEqual(repos[1], ["apiopenclaw/test", "·", "↑ 2", "↓ 3", "#7", "✗", "✗", "○"]);
    assert.deepEqual(services[1].slice(1), ["previewapi", "ready", ":42201", "calm-river✕", "+"]);
    view.dispose();
  } finally {
    delete globalThis.ResizeObserver;
  }
});
