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
const hostRequest = async (method, params) => {
  if (method === "plugins.sessionAction" && params.actionId === "collection") {
    const data = collections.get(params.sessionKey);
    return data instanceof Error ? { ok: false, error: data.message } : { ok: true, result: data };
  }
  if (method === "portal.list") return { portals };
  calls.push({ method, ...params });
  if (method === "plugins.sessionAction" && params.actionId === "svc") return svcResult;
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

test("panel: an empty collection", async () => {
  const { host, view } = await mount("panel", collection([]));
  const all = texts(host);
  assert.ok(all.includes("test collection"));
  assert.ok(all.includes("0 repositories"));
  assert.ok(all.some((t) => t.startsWith("No repositories yet")));
  assert.ok(all.some((t) => t.startsWith("None running")));
  view.dispose();
});

test("panel: a busy collection", async () => {
  const { host, view } = await mount("panel", busy);
  const all = texts(host);
  // The shared branch once, in the header, not on every repository.
  assert.equal(all.filter((t) => t === "openclaw/test").length, 1);
  assert.ok(all.includes("3 repositories · 1/2 services running"));
  assert.ok(all.includes("3 behind main"));
  assert.ok(all.includes("2 commits to push"));
  assert.ok(all.includes("conflict"));
  assert.ok(all.includes("CI 1 failed"));
  assert.deepEqual(buttons(host).filter((b) => b.length > 1 && !b.startsWith("■ ") && !b.startsWith("▶ ")), ["Catch up", "Resolve", "Push", "Push + PR", "Push + PR"]);
  // Each service leads with a button that is its state and its action.
  assert.ok(titled(host, "preview is running. Stop it"));
  assert.ok(titled(host, "worker: exited (exit 1). Restart it"));
  assert.ok(titled(host, "Start all services"));
  assert.ok(titled(host, "Stop all services"));
  assert.ok(titled(host, "Close the cloudflare tunnel"));
  // A service and its tunnel are one line.
  assert.equal(all.filter((t) => t === "preview").length, 1);
  assert.ok(all.includes("cloudflare"));
  view.dispose();
});

test("panel: repositories on different branches", async () => {
  const { host, view } = await mount("panel", collection([
    repo("api"), repo("web"), repo("docs", { branch: "release" }),
  ]));
  const all = texts(host);
  assert.equal(all.filter((t) => t === "openclaw/test").length, 1);
  assert.equal(all.filter((t) => t === "release").length, 1);
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
  assert.equal(all.filter((t) => t === "separate tunnel").length, 1);
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
  const all = texts(card);
  assert.ok(all.includes("3 repos: 1 file, 3 commits to push"));
  assert.ok(all.includes("api: 3 behind"));
  assert.ok(all.includes("api #7 · conflict"));
  // Services are marked ■ (some running) or ▶ (none), not with a dot.
  assert.ok(all.includes("■1/2 services running"));
  assert.ok(buttons(card).includes("Resolve"));
  // The branch of every repository.
  assert.equal(all.filter((t) => t === "openclaw/test").length, 3);
  assert.equal(all.filter((t) => t === "preview").length, 1);
  view.dispose();
});

test("composer: a repository without a branch yet", async () => {
  const { host, view } = await mount("composer", collection([repo("api", { branch: null })]));
  assert.ok(texts(host).includes("at main"));
  assert.ok(texts(host).includes("all pushed"));
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
  assert.equal(pick.children[0].textContent, "+ Tunnel…");
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

test("composer: start and stop all in the rows only with more than one service", async () => {
  const one = await mount("composer", collection([repo("api")], [
    { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "absent" },
  ]));
  assert.ok(titled(one.host, "preview: absent. Start it"));
  const detail = [...one.host.all()].find((n) => n.className === "wtc-collection-card").children.find((n) => n.tagName === "DIV");
  assert.equal(titled(detail, "Start all services"), undefined);
  one.view.dispose();
  const { host, view } = await mount("composer", busy);
  assert.ok(titled(host, "Start all services"));
  assert.ok(titled(host, "Stop all services"));
  view.dispose();
});

test("panel: the ways to a service", async () => {
  const web = { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" };
  const team = await mount("panel", { ...collection([repo("api")], [web]), tunnelProviders: ["tailscale", "cloudflare"], teamTailnet: true });
  const pick = [...team.host.all()].find((n) => n.tagName === "SELECT");
  assert.deepEqual(pick.children.map((o) => o.textContent),
    ["+ Tunnel…", "gateway · OpenClaw portal", "cloudflare · public link", "ngrok · public link · install first", "devtunnel · your account · install first", "tailscale · team tailnet"]);
  team.view.dispose();
  // Without portal ingress, the gateway's portal comes last, marked.
  const bare = await mount("panel", { ...collection([repo("api")], [web]), tunnelProviders: ["cloudflare"], portalIngress: false });
  const choice = [...bare.host.all()].find((n) => n.tagName === "SELECT");
  assert.deepEqual(choice.children.map((o) => o.textContent),
    ["+ Tunnel…", "cloudflare · public link", "ngrok · public link · install first", "devtunnel · your account · install first", "gateway · OpenClaw portal · needs ingress"]);
  bare.view.dispose();
  // With every other way taken, the one left is named.
  const tunnels = ["cloudflare", "ngrok", "devtunnel"].map((p) => ({ group: "api/web", path: `api/web/${p}`, repo: "api", kind: "endpoint", provider: p, state: "idle" }));
  const last = await mount("panel", { ...collection([repo("api")], [web, ...tunnels]), tunnelProviders: [] });
  assert.ok(buttons(last.host).includes("+ gateway"), "one way left: no choice to make");
  last.view.dispose();
});

test("panel: a portal of the gateway beside its service", async () => {
  const web = { group: "api/web", path: "api/web/main", repo: "api", kind: "service", label: "preview", state: "ready", url: "http://127.0.0.1:42201" };
  portals = [{ id: "p42201", title: "preview", port: 42201, listenPort: 41553, url: "http://172.30.0.2:41553/?openclaw_portal=t", publicUrl: "http://172.30.0.2:41553/", createdAtMs: 0 }];
  const open = await mount("panel", collection([repo("api")], [web]));
  const links = [...open.host.all()].filter((n) => n.tagName === "A").map((n) => n.textContent);
  // A container's address, which a browser elsewhere cannot reach: marked.
  assert.ok(links.includes("gateway ⚠"));
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

test("composer: the collection's actions on the summary while collapsed", async () => {
  const { host, view } = await mount("composer", busy);
  const card = [...host.all()].find((n) => n.className === "wtc-collection-card");
  const summary = card.children.find((n) => n.tagName === "SUMMARY");
  assert.deepEqual(buttons(summary).filter((b) => b.length > 1), ["Push 3", "Catch up", "▶ Start all", "■ Stop all"]);
  // Open, the rows have them and the summary does not repeat them.
  card.open = true;
  for (const fn of card.listeners.toggle) fn();
  assert.deepEqual(buttons(summary).filter((b) => b.length > 1), []);
  view.dispose();
});
