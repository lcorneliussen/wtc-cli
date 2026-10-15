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

// Every collection request is answered from `collections`, by session.
const collections = new Map();
const hostRequest = async (method, params) => {
  if (method === "plugins.sessionAction" && params.actionId === "collection") {
    const data = collections.get(params.sessionKey);
    return data instanceof Error ? { ok: false, error: data.message } : { ok: true, result: data };
  }
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
  assert.deepEqual(buttons(host).filter((b) => b !== "↻"), ["Catch up", "Resolve", "Push", "Push + PR", "Push + PR", "Restart"]);
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
  assert.ok(all.includes("1/2 services running"));
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
