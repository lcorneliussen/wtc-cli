// The Collection panel: everything a collection's session works with — its
// repositories, their pull requests and the services running for it — in one
// place. Plain ESM, no build step and no SDK import.
//
// A collection is driven from one conversation. Each repository's worktree is
// held by an owner session the gateway publishes for, usually a hidden one;
// this panel publishes through that owner, the way the chat's own Publish PR
// button does for a session's single checkout.
const PLUGIN_ID = "wtc";
const HOME = "https://github.com/lcorneliussen/wtc-cli";
const TITLE = "wtc Collection";

const el = (tag, props = {}, ...kids) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...kids.filter((k) => k != null && k !== false));
  return node;
};
const css = (node, text) => ((node.style.cssText = text), node);

async function call(host, actionId, sessionKey, agentId) {
  const res = await host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId, sessionKey, ...(agentId ? { agentId } : {}) });
  if (res && res.ok === false) throw new Error(res.error ?? "action failed");
  return res?.result ?? res;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;
// http too: a gateway's direct portal is plain HTTP.
const link = (text, href) => (/^https?:\/\//.test(href ?? "") ? el("a", { textContent: text, href, target: "_blank", rel: "noopener noreferrer" }) : text);
const uuid = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`);

const isRunning = (s) => s.state === "ready" || s.state === "running";
const isFailed = (s) => s.state === "failed" || (s.state === "exited" && s.exit_code > 0);
const serviceState = (s) => `${s.state ?? "?"}${s.exit_code > 0 ? ` (exit ${s.exit_code})` : ""}`;

// Pushing is the person's, through the gateway, never the agent's: the
// gateway publishes an owner's worktree (pushes the branch, opens or updates
// a draft pull request) with the person's own account, which only the
// gateway holds, and only after they confirm it. GitHub's is OpenClaw's
// personal connection (and OpenClaw commits what is pending first); the
// shared account the gateway may also have (often the gh login its agents
// use too) is never used for it. Bitbucket's is the person's login in the
// Bitbucket plugin, which publishes what is committed.
const CONNECT = { github: "connect your own GitHub account first", bitbucket: "connect your own Bitbucket login first" };
const BITBUCKET = "bitbucket";
const BITBUCKET_CONNECT = "/plugins/bitbucket/connect";
const PUSHES = new Set(["github", "bitbucket"]);
// The Control UI's own pages sit under its base path, before /chat and the like.
const appHref = (path) => {
  const here = typeof location === "undefined" ? "" : location.pathname.replace(/\/(chat|dashboard|settings|sessions|share|skills|worktrees|agents)(\/.*)?$/, "");
  return `${here.replace(/\/$/, "")}${path}`;
};
const personalAccount = (options) => {
  const mine = options?.personal;
  return mine?.state === "connected" && mine.account && mine.generation ? mine : null;
};
// Asks the gateway to publish; returns the confirmation it waits for, if any.
async function publishRepo(host, agentId, r, say) {
  const name = r.repo ?? r.project;
  const target = { sessionKey: r.sessionKey, agentId };
  const mine = personalAccount(await host.request("sessions.github.options", target));
  if (!mine) {
    say(`${name}: ${CONNECT.github}.`);
    return { connect: true };
  }
  say(`Pushing ${name}…`);
  const p = proposalOf(r);
  const out = await host.request("sessions.github.publish", {
    ...target, idempotencyKey: uuid(), selection: { source: "personal", account: mine.account, generation: mine.generation },
    ...(p?.title ? { title: p.title } : {}), ...(p?.body ? { body: p.body } : {}),
  });
  return settle(host, target, name, out, say);
}
async function confirmRepo(host, agentId, r, pending, say) {
  const name = r.repo ?? r.project;
  const target = { sessionKey: r.sessionKey, agentId };
  say(`Pushing ${name}…`);
  const out = await host.request("sessions.github.confirm", { ...target, requestId: pending.requestId, ...pending.confirmation });
  return settle(host, target, name, out, say);
}
// What a publish or confirm came back with: done, failed, or a confirmation
// to ask the person for (its account, generation and digest, from status).
async function settle(host, target, name, out, say) {
  if (out?.status === "needs_confirmation") {
    const status = await host.request("sessions.github.status", { ...target, requestId: out.requestId }).catch(() => null);
    const c = status?.confirmation ?? out.confirmation ?? null;
    if (!c) { say(`${name}: the gateway waits for a confirmation; confirm it under Publish in the owner session.`); return null; }
    say(`${name}: confirm to push as @${c.account?.login ?? "you"}.`);
    return { requestId: out.requestId, confirmation: { generation: c.generation, account: c.account, requestDigest: c.requestDigest } };
  }
  if (out?.status === "published") say(`${name}: pushed ${out.url ?? out.pullRequestUrl ?? ""}`.trim());
  else say(`${name}: ${out?.status ?? "done"}${out?.error ? ` — ${out.error.message ?? out.error}` : out?.message ? ` — ${out.message}` : ""}`);
  return null;
}

// The Bitbucket plugin's own publish: ask, then confirm what it names.
async function bitbucketAction(host, agentId, r, actionId, payload) {
  let res;
  try {
    res = await host.request("plugins.sessionAction", { pluginId: BITBUCKET, actionId, sessionKey: r.sessionKey, ...(agentId ? { agentId } : {}), payload });
  } catch (err) {
    if (/unknown plugin session action/.test(String(err?.message ?? err))) throw new Error("the gateway has no Bitbucket publish: install openclaw-bitbucket 0.3.0 or later");
    throw err;
  }
  if (res?.ok === false) throw new Error(res.error ?? "refused");
  return res?.result ?? res;
}
const bitbucketAccount = (c) => c?.name ? `${c.name} (${c.account})` : c?.account ?? "you";
async function publishBitbucket(host, agentId, r, say, dest) {
  const name = r.repo ?? r.project;
  say(`Asking to push ${name}…`);
  // Its publish takes a title, not yet a description.
  const title = proposalOf(r)?.title;
  const out = await bitbucketAction(host, agentId, r, "publish", { draft: true, ...(dest ? { dest } : {}), ...(title ? { title } : {}) });
  if (out?.status === "not_connected") {
    say(`${name}: ${CONNECT.bitbucket}.`);
    return { connect: true };
  }
  const c = out?.confirmation;
  if (out?.status !== "needs_confirmation" || !c) { say(`${name}: ${out?.status ?? "no answer"}`); return null; }
  const left = c.uncommitted ? `; ${plural(c.uncommitted, "uncommitted file")} stay behind` : "";
  say(`${name}: confirm to push ${c.branch} (${String(c.head).slice(0, 7)}) to ${c.dest} as ${bitbucketAccount(c)}${left}.`);
  return { forge: BITBUCKET, requestId: out.requestId, confirmation: c };
}
async function confirmBitbucket(host, agentId, r, pending, say) {
  const name = r.repo ?? r.project;
  say(`Pushing ${name}…`);
  const out = await bitbucketAction(host, agentId, r, "publish-confirm", { requestId: pending.requestId, requestDigest: pending.confirmation.requestDigest });
  say(`${name}: ${out?.created ? "pushed, draft pull request opened" : "pushed"} ${out?.url ?? ""}`.trim());
  return null;
}

async function catchUpRepo(host, agentId, r, say) {
  const name = r.repo ?? r.project;
  say(`Catching up ${name}…`);
  const out = await call(host, "catch-up", r.sessionKey, agentId);
  say(`${name}: ${out.json?.summary ?? (out.message || "exit " + out.code)}`);
}

// One load of a session's collection, shared by the panel and the card above
// the message box, so that both show the same thing and the gateway runs wtc status
// once per refresh rather than once per view.
const REFRESH_MS = 30000;
const store = new Map();
function watchCollection(host, sessionKey, agentId, onChange) {
  let entry = store.get(sessionKey);
  if (!entry) {
    entry = { data: null, error: null, at: 0, inflight: null, listeners: new Set(), timer: null };
    store.set(sessionKey, entry);
  }
  const notify = () => entry.listeners.forEach((fn) => fn(entry));
  const load = (force) => {
    if (entry.inflight) return entry.inflight;
    if (!force && entry.data && Date.now() - entry.at < REFRESH_MS / 2) return Promise.resolve(entry);
    // The gateway's portals, to show beside the services they expose.
    entry.inflight = Promise.all([call(host, "collection", sessionKey, agentId), host.request("portal.list", {}).catch(() => null)])
      .then(([data, portals]) => { entry.data = { ...data, portals: portals?.portals ?? [] }; entry.error = null; },
        (err) => { entry.error = String(err?.message ?? err); })
      .then(() => { entry.at = Date.now(); entry.inflight = null; notify(); return entry; });
    return entry.inflight;
  };
  entry.listeners.add(onChange);
  // Pull requests and CI move on their own; look again while someone watches.
  entry.timer ??= setInterval(() => { if (!document.hidden && !entry.paused) void load(true); }, REFRESH_MS);
  if (entry.data || entry.error) onChange(entry);
  void load(false);
  return {
    refresh: () => load(true),
    pause: (on) => { entry.paused = on; },
    dispose() {
      entry.listeners.delete(onChange);
      if (entry.listeners.size === 0) {
        clearInterval(entry.timer);
        store.delete(sessionKey);
      }
    },
  };
}

// The collection card above the message box, where OpenClaw shows a task's
// progress: collapsed, what is not pushed yet, the open pull requests with
// their CI and the services that run; expanded, each repository on its branch
// with its Push and Catch up buttons, and each service with its local address
// and tunnels. It wraps the
// built-in composer and adds nothing to sessions that are not collections.
const OPEN_KEY = "wtc.collectionCard.open";
const remembered = () => { try { return localStorage.getItem(OPEN_KEY) === "1"; } catch { return false; } };
const remember = (open) => { try { localStorage.setItem(OPEN_KEY, open ? "1" : "0"); } catch {} };
const TONE = { success: "var(--ok, #2da44e)", failure: "var(--danger, #cf222e)", pending: "var(--warn, #bf8700)", none: "var(--muted, #888)" };
const ago = (at) => {
  const s = Math.round((Date.now() - at) / 1000);
  return s < 60 ? "just now" : s < 3600 ? `${Math.round(s / 60)}m ago` : `${Math.round(s / 3600)}h ago`;
};

const ciText = (pr) => ({ success: "CI passed", failure: `CI ${pr.failed} failed`, pending: `CI ${pr.running} running` })[pr.checks] ?? "no checks";
// The state of an open pull request at a glance: CI, whether it merges, review.
const prTone = (pr) => pr.mergeable === "conflicting" || pr.checks === "failure" || pr.review === "changes_requested" ? TONE.failure
  : pr.checks === "pending" ? TONE.pending : pr.checks === "success" ? TONE.success : TONE.none;
const prState = (pr) => [
  pr.checks && pr.checks !== "none" ? ciText(pr) : "",
  pr.mergeable === "conflicting" ? "conflicts with its base" : "",
  { approved: "approved", changes_requested: "changes requested", review_required: "review required" }[pr.review] ?? "",
].filter(Boolean);
// Forge marks before repository names, the way people know their repositories.
const FORGE_PATHS = {
  github: "M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12",
  bitbucket: "M.778 1.213a.768.768 0 00-.768.892l3.263 19.81c.084.5.515.868 1.022.873H19.95a.772.772 0 00.77-.646l3.27-20.03a.768.768 0 00-.768-.891zM14.52 15.53H9.522L8.17 8.466h7.561z",
};
const FORGE_COLORS = { github: "currentColor", bitbucket: "#2684FF" };
function forgeIcon(forge, size = 14) {
  const d = FORGE_PATHS[forge];
  if (!d) return css(el("span"), `flex:none;width:${size}px;height:${size}px;border-radius:3px;background:var(--muted, #888);opacity:.4`);
  const NS = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("width", String(size));
  svg.setAttribute("height", String(size));
  svg.setAttribute("aria-label", forge === "github" ? "GitHub" : "Bitbucket");
  svg.style.cssText = "flex:none";
  const pathEl = document.createElementNS(NS, "path");
  pathEl.setAttribute("d", d);
  pathEl.setAttribute("fill", FORGE_COLORS[forge]);
  svg.append(pathEl);
  return svg;
}
const webUrlOf = (r) => r.owner && r.repo
  ? (r.forge === "github" ? `https://github.com/${r.owner}/${r.repo}` : r.forge === "bitbucket" ? `https://bitbucket.org/${r.owner}/${r.repo}` : null)
  : null;
// icon, owner/ (muted), name; the name links to the repository.
function repoTitle(r, size = "14px") {
  const name = r.repo ?? r.project ?? r.label ?? "?";
  const href = webUrlOf(r);
  const label = el(href ? "a" : "span", { title: r.path ?? "" },
    r.owner ? css(el("span", { textContent: `${r.owner}/` }), "font-weight:400;color:var(--muted, #888)") : null,
    css(el("span", { textContent: name }), "font-weight:600;color:var(--text-strong, inherit)"));
  if (href) Object.assign(label, { href, target: "_blank", rel: "noopener noreferrer" });
  label.style.cssText = `text-decoration:none;color:inherit;font-size:${size};overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0`;
  return css(el("span", {}, forgeIcon(r.forge), label), "display:inline-flex;align-items:center;gap:6px;min-width:0");
}
// The branch most of a collection's repositories are on: shown once, and per
// repository only where it differs.
const collectionBranch = (repos) => {
  const counts = new Map();
  for (const r of repos) if (r.branch) counts.set(r.branch, (counts.get(r.branch) ?? 0) + 1);
  return [...counts].sort((a, b) => b[1] - a[1])[0]?.[0] ?? null;
};
const serviceName = (s) => s.label || String(s.group ?? s.path ?? "?").split("/").pop();
// A service with its tunnels is one line, as in wtc status: wtc svc puts a
// tunnel in the group of the service it exposes and marks it an endpoint.
function serviceGroups(services) {
  const groups = new Map();
  for (const s of services) {
    const key = s.group ?? s.path;
    if (!groups.has(key)) groups.set(key, { group: s.group, repo: s.repo, members: [], tunnels: [], main: null });
    const g = groups.get(key);
    g.members.push(s);
    if (s.kind === "endpoint") g.tunnels.push(s);
    else g.main ??= s;
  }
  return [...groups.values()].map((g) => {
    const main = g.main ?? g.members[0];
    const failed = g.members.some(isFailed);
    return {
      ...g, label: serviceName(main), url: g.main?.url ?? null,
      // A tunnel started as a service of its own rather than with wtc svc
      // tunnel: it shows apart from the service it exposes.
      loose: !g.main || TUNNEL_URL.test(g.main.url ?? ""),
      failed, running: !failed && isRunning(main), exited: g.members.some((s) => s.state === "exited"),
      // The state that most needs attention.
      state: serviceState(g.members.find(isFailed) ?? g.members.find((s) => !isRunning(s)) ?? main),
    };
  });
}
// A service listens on the gateway, often in a container: its loopback address
// opens nowhere else, so it shows as its port, and a tunnel makes it reachable.
const LOOPBACK = /^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])(:\d+)?(\/|$)/;
const address = (url) => LOOPBACK.test(url)
  ? css(el("span", { textContent: `:${new URL(url).port || "80"}`, title: `${url} on the gateway: open a tunnel to reach it from here` }), "font-family:var(--mono, ui-monospace, monospace);color:var(--muted, #888)")
  : link(url.replace(/^https?:\/\//, ""), url);
const TUNNEL_URL = /^https:\/\/[^/]*(trycloudflare\.com|\.ts\.net|ngrok[-a-z]*\.(app|io|dev)|devtunnels\.ms)(:\d+)?(\/|$)/;
const consolidate = (g) => `The tunnel ${g.group} runs as a service of its own. Consolidate it as the wtc-svc skill says: remove it and open it again with wtc svc tunnel on the service it exposes.`;
const tunnelName = (t) => t.provider || (/trycloudflare\.com/.test(t.url ?? "") ? "cloudflare" : /\.ts\.net/.test(t.url ?? "") ? "tailscale" : "tunnel");
// Things only the conversation can do (resolve a conflict, publish where the
// gateway cannot) are asked of it, in the conversation, where it shows.
const askConversation = (host, sessionKey, message) => host.request("sessions.send", { key: sessionKey, message });
// Pushing goes to the repository's own branch and, without one yet, opens a
// draft pull request for it.
const pushTitle = (pr) => pr ? `Commit what is pending and push it to #${pr.number}` : "Commit what is pending, push the branch and open a draft pull request";
// The title and description the agent proposed with wtc publish, which a new
// pull request opens with; the gateway takes them only when it opens one.
const proposalOf = (r) => (r.pullRequest ? null : r.proposal?.title || r.proposal?.body ? r.proposal : null);
const proposalTitle = (r) => {
  if (r.pullRequest) return "";
  const p = proposalOf(r);
  if (!p) return " The agent proposed no title or description: ask it to run wtc publish with them, or the pull request opens as \"Publish <branch>\".";
  return p.title ? ` Title: "${p.title}"${p.body ? ", with the agent's description" : ""}.` : " With the agent's description.";
};

// Services. wtc svc starts, stops and restarts them (one, or all without a
// group) and opens or closes their tunnels; the gateway runs it in the
// collection's folder.
async function svcRequest(host, sessionKey, agentId, payload) {
  // A gateway portal is OpenClaw's own: the Control UI opens and closes it.
  if (payload.op === "portal") {
    if (payload.stop) return host.request("portal.close", { id: payload.id });
    const opened = await host.request("portal.open", { port: payload.port, title: payload.title });
    if (opened?.id && opened.url) portalUrls.set(opened.id, opened.url);
    return opened;
  }
  const res = await host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId: "svc", sessionKey, agentId, payload });
  if (res?.ok === false) throw new Error(res.error ?? "wtc svc failed");
  return res?.result ?? res;
}
// What a new tunnel can go through, first what to try first: the gateway's
// own portal, then a public link for when a portal does not work, a dev
// tunnel for the person's account, and the tailnet when it is the team's.
const TUNNEL_PROVIDERS = [
  { name: "gateway", reach: "OpenClaw portal", portal: true },
  { name: "cloudflare", reach: "public link", public: true },
  { name: "ngrok", reach: "public link", public: true },
  { name: "devtunnel", reach: "your account" },
  { name: "tailscale", reach: "team tailnet", team: true },
];
// portal.list leaves out a portal's URL with its access token; opening the
// same port again returns it. Kept for the portals this page opened.
const portalUrls = new Map();
const portOf = (url) => { try { return Number(new URL(url).port) || null; } catch { return null; } };
// A portal at a loopback or private address the browser is not on (a
// container's, say) needs portal ingress on the gateway to be reached.
const PRIVATE_HOST = /^(127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|localhost$|\[::1\]$)/;
const unreachable = (url) => {
  try {
    const host = new URL(url).hostname;
    const here = typeof location === "undefined" ? "" : location.hostname;
    return host !== here && PRIVATE_HOST.test(host) && !PRIVATE_HOST.test(here);
  } catch { return false; }
};
const installTunnel = (g, provider, program) => `Open a ${provider} tunnel to ${g.group}. ${program} is not on the gateway's PATH: install it for this collection, then open the tunnel as the wtc-svc skill says.`;
// A button in front of a service, where a repository has its forge icon and
// as small, that is also its state: ■ stops a running one, ▶ starts a
// stopped one, ↻ restarts a failed one.
function stateGlyph(glyph, title, tone, run) {
  const b = css(el("button", { type: "button", textContent: glyph, title, ariaLabel: title }),
    `flex:none;box-sizing:border-box;width:14px;height:14px;display:inline-flex;align-items:center;justify-content:center;border:0;background:transparent;color:${tone};cursor:pointer;font-size:${glyph === "↻" ? 13 : 10}px;line-height:1;padding:0`);
  b.onclick = (e) => { e.preventDefault(); e.stopPropagation(); void run(); };
  return b;
}
// A pill like Push + PR, for the actions on the right.
function pill(text, title, run) {
  const b = css(el("button", { type: "button", textContent: text, title, ariaLabel: title }),
    "border:1px solid var(--border-strong, rgba(128,128,128,.4));background:transparent;color:inherit;cursor:pointer;font:inherit;font-size:12px;line-height:18px;padding:0 9px;border-radius:999px;white-space:nowrap");
  b.onclick = (e) => { e.preventDefault(); e.stopPropagation(); void run(); };
  return b;
}
const busyMark = () => css(el("span", { textContent: "…", title: "Working" }),
  "flex:none;width:14px;text-align:center;color:var(--muted, #888)");
// svc: { run(payload, doing), busy(key), ask(text) } from the view.
function stateButton(g, svc) {
  if (svc.busy(g.group) || svc.busy("*")) return busyMark();
  if (g.failed) return stateGlyph("↻", `${g.label}: ${g.state}. Restart it`, TONE.failure, () => svc.run({ op: "restart", group: g.group }, `Restarting ${g.label}…`));
  if (g.running) return stateGlyph("■", `${g.label} is running. Stop it`, TONE.success, () => svc.run({ op: "down", group: g.group }, `Stopping ${g.label}…`));
  return stateGlyph("▶", `${g.label}: ${g.state}. Start it`, TONE.none, () => svc.run({ op: "up", group: g.group }, `Starting ${g.label}…`));
}
// A service's tunnels: a running one links to its address and closes with ✕,
// a closed one is a ▶ button to open it again. A new one opens through the
// one provider the gateway has ("+ cloudflare"), or a choice of them
// ("+ Tunnel…"); with none, any of them, installed by the conversation first.
const smallButton = (text, title, run) => {
  const b = css(el("button", { type: "button", textContent: text, title, ariaLabel: title }),
    "border:0;background:transparent;color:var(--muted, #888);cursor:pointer;font:inherit;font-size:11px;padding:0 2px");
  b.onclick = (e) => { e.preventDefault(); e.stopPropagation(); void run(); };
  return b;
};
// The gateway's portals to a service, by the port it listens on.
const portalsOf = (g, data = {}) => {
  const port = portOf(g.url);
  return (data.portals ?? []).filter((p) => port && p.port === port);
};
// An open portal: a link (or, listed without its URL, a button that opens it
// again) and ✕. label is what the link says, "gateway" by default.
function portalNode(g, p, svc, label = "gateway") {
  const far = unreachable(p.publicUrl);
  const name = far ? `${label} ⚠` : label;
  const url = p.url ?? portalUrls.get(p.id);
  // Without its URL, the portal is opened again for it, in a tab opened
  // on the click so that no popup blocker stops it.
  const go = url ? link(name, url) : smallButton(name, "Open the portal", async () => {
    const tab = typeof window === "undefined" ? null : window.open("about:blank", "_blank");
    const r = await svc.run({ op: "portal", port: p.port, title: p.title }, "Opening the portal…", `${g.group}:gateway`);
    if (tab && r?.url) { tab.opener = null; tab.location.href = r.url; } else tab?.close();
  });
  if (!url) go.style.color = "inherit";
  return css(el("span", { title: far ? `The portal listens on ${new URL(p.publicUrl).host}, which this browser cannot reach: the gateway needs portal ingress (managed Tailscale or a wildcard domain). Use a tunnel instead.` : `OpenClaw portal ${p.publicUrl}` },
    go,
    smallButton("✕", "Close the portal", () => svc.run({ op: "portal", stop: true, id: p.id }, "Closing the portal…", `${g.group}:gateway`))),
  `display:inline-flex;align-items:center;gap:2px;white-space:nowrap;${far ? `color:${TONE.pending}` : ""}`);
}
// A wtc tunnel: running, a link and ✕; closed, a button that opens it again.
// label is what the link says, the provider by default; closed, opener is
// what the button says.
function tunnelNode(g, t, svc, { label, opener } = {}) {
  const name = tunnelName(t);
  const pub = TUNNEL_PROVIDERS.some((p) => p.name === name && p.public);
  if (svc.busy(`${g.group}:${name}`)) return css(el("span", { textContent: label ? "…" : `${name} …` }), "color:var(--muted, #888)");
  // A closed tunnel is an action: open it again.
  if (!isRunning(t)) {
    const title = `Open the ${name} tunnel again${pub ? ": anyone with the link can use it" : ""} (${serviceState(t)})`;
    const again = () => svc.run({ op: "tunnel", group: g.group, provider: name, public: pub }, `Opening the ${name} tunnel…`, `${g.group}:${name}`);
    return opener ? opener(title, again) : pill(`▶ ${name}`, title, again);
  }
  return css(el("span", {},
    t.url ? link(label ?? name, t.url) : label ?? name,
    smallButton("✕", `Close the ${name} tunnel`, () => svc.run({ op: "tunnel", group: g.group, provider: name, stop: true }, `Closing the ${name} tunnel…`, `${g.group}:${name}`))),
  "display:inline-flex;align-items:center;gap:2px;white-space:nowrap");
}
// A new way to a service, or null when none is left. compact: a "+" only,
// whose title and options name the ways.
function newTunnel(g, svc, data = {}, { compact = false } = {}) {
  const port = portOf(g.url);
  const portals = portalsOf(g, data);
  const taken = (p) => p.portal ? portals.length > 0 || !port : g.tunnels.some((t) => tunnelName(t) === p.name);
  // Without portal ingress a portal comes back at the gateway's own address,
  // which a remote browser rarely reaches: it is offered last, as such.
  const ingress = data.portalIngress !== false;
  const free = TUNNEL_PROVIDERS.filter((p) => !taken(p) && (!p.team || data.teamTailnet))
    .sort((a, b) => Number(!ingress && !!a.portal) - Number(!ingress && !!b.portal));
  const ready = (p) => p.portal || data.tunnelProviders?.includes(p.name);
  const note = (p) => p.portal ? (ingress ? "" : " · needs ingress") : ready(p) ? "" : " · install first";
  const open = async (p) => {
    if (p.portal) return svc.run({ op: "portal", port, title: g.label }, `Opening a portal to ${g.label}…`, `${g.group}:gateway`);
    const r = await svc.run({ op: "tunnel", group: g.group, provider: p.name, public: !!p.public, add: true }, `Opening a ${p.name} tunnel to ${g.label}…`, `${g.group}:${p.name}`);
    if (r?.missing) await svc.ask(installTunnel(g, p.name, r.missing));
  };
  if (!g.main || g.loose || !free.length) return null;
  // One way left is a button that names it; more are a choice.
  if (free.length === 1) {
    const p = free[0];
    return pill(compact ? "+" : `+ ${p.name}`, `Open ${p.name} (${p.reach}) to this service${p.public ? ": anyone with the link can use it" : ""}${!ready(p) ? "; the conversation installs it first" : p.portal && !ingress ? "; the gateway has no portal ingress, so it may not be reachable from here: a tunnel is" : ""}`, () => open(p));
  }
  const pick = css(el("select", { title: "Open a way to this service from your browser: choose how" },
    el("option", { value: "", textContent: compact ? "+" : "+ Tunnel…" }),
    ...free.map((p) => el("option", { value: p.name, textContent: `${p.name} · ${p.reach}${note(p)}` }))),
  `appearance:none;-webkit-appearance:none;font:inherit;font-size:12px;line-height:18px;color:inherit;background:transparent;border:1px solid var(--border-strong, rgba(128,128,128,.4));border-radius:999px;padding:0 ${compact ? 7 : 9}px;cursor:pointer;field-sizing:content;max-width:8em`);
  pick.onclick = (e) => e.stopPropagation();
  pick.onchange = async () => {
    const p = free.find((x) => x.name === pick.value);
    pick.value = "";
    if (p) await open(p);
  };
  return pick;
}

// "wtc Collection", wtc linking to the project.
const brand = (size) => css(el("span", {},
  css(el("a", { textContent: "wtc", href: HOME, target: "_blank", rel: "noopener noreferrer", title: "wtc — worktree collections (opens on GitHub)" }),
    "color:var(--accent, inherit);text-decoration:none;font-weight:700"),
  el("span", { textContent: "Collection" })), `display:inline-flex;align-items:baseline;gap:.3em;flex:none;white-space:nowrap;font-weight:600;font-size:${size}`);

// The collection as wtc status's tables, for the card and the panel alike: a
// column per fact, always in its place, "·" where there is none. What can be
// done about a fact is done on it: the ↑ count pushes, the ↓ count catches up,
// a conflict is resolved, a service's state starts or stops it.
//
// v is the view: { host, key(), agent(), svc, say(text), refresh(), pause(on),
// rerender() }. Narrow, a row wraps into two lines: what it is (the repository
// and its branch, the service), then its facts under the header, still in
// their columns.
const NARROW_PX = 520;
function collectionTables(v) {
  // Repositories being pushed, and being caught up.
  const busy = new Set();
  const catching = new Set();
  // What a repository's push would send: its commits not on the forge yet,
  // or, before its first pull request, everything ahead of the base.
  const toPush = (r) => r.unpublished || (!r.pullRequest ? r.ahead ?? 0 : 0);
  const pushable = (r) => !r.error && (r.dirty || toPush(r) > 0);
  const nameOf = (r) => r.repo ?? r.project ?? r.label;
  const baseOf = (r) => (r.base ? r.base.replace(/^origin\//, "") : "base");
  // Pushes waiting for the person's confirmation, by owner session.
  const confirming = new Map();
  // Whether the person has their own account on each forge: unknown until
  // asked, asked again at most every 30 seconds while something waits to be
  // pushed there. false for Bitbucket: the gateway has no publish for it.
  const mine = { github: undefined, bitbucket: undefined };
  const askedAt = { github: 0, bitbucket: 0 };
  let showConnect = null;
  const accountOf = {
    github: (r) => v.host.request("sessions.github.options", { sessionKey: r.sessionKey, agentId: v.agent() }).then(personalAccount),
    bitbucket: (r) => bitbucketAction(v.host, v.agent(), r, "publish-options", {}).then((o) => o?.account ?? null,
      (err) => (/no Bitbucket publish/.test(err.message) ? false : Promise.reject(err))),
  };
  function checkAccount(repos) {
    for (const forge of PUSHES) {
      const r = repos.find((x) => x.forge === forge && pushable(x));
      if (!r || mine[forge] === false || Date.now() - askedAt[forge] < 30000) continue;
      askedAt[forge] = Date.now();
      accountOf[forge](r).then((now) => {
        if (now && showConnect === forge) showConnect = null;
        if (!!now !== !!mine[forge] || mine[forge] === undefined || now === false) { mine[forge] = now; v.rerender(); }
      }, () => {});
    }
  }
  // How to connect it: shown once a push needed it, until it is there.
  function notice() {
    if (!showConnect || mine[showConnect]) return null;
    const bitbucket = showConnect === BITBUCKET;
    const open = el("a", bitbucket
      ? { textContent: "Open the Bitbucket page", href: BITBUCKET_CONNECT, target: "_blank", rel: "noopener" }
      : { textContent: "Open Settings → Profile", href: appHref("/settings/profile") });
    open.style.cssText = "color:var(--accent, inherit);font-weight:600";
    const close = smallButton("✕", "Hide", () => { showConnect = null; v.rerender(); });
    const steps = bitbucket
      ? [el("li", {}, open, " in its own tab (the Bitbucket tab in the sidebar shows it, but cannot take the form)."),
        el("li", { textContent: "Enter your Atlassian account e-mail and a Bitbucket API token with repositories and pull requests read and write." }),
        el("li", { textContent: "Connect. Then press ↑ again." })]
      : [el("li", {}, open, ", then GitHub connections → Connect."),
        el("li", { textContent: "Choose For me (not For the system), open github.com/login/device and enter the code shown." }),
        el("li", { textContent: "Approve, including workflow. Then press ↑ again." })];
    return css(el("div", { className: "wtc-connect" },
      css(el("div", {}, css(el("strong", { textContent: bitbucket ? "Pushing needs your own Bitbucket login" : "Pushing needs your own GitHub account" }), ""), close), "display:flex;justify-content:space-between;align-items:center;gap:8px"),
      el("div", { textContent: "The gateway pushes for you with it, after you confirm each push." }),
      css(el("ol", {}, ...steps), "margin:0;padding-left:18px;display:grid;gap:2px"),
      bitbucket ? null : css(el("div", { textContent: "A GitHub identity under Agents → Tools is the agent's own; it is not used for pushing." }), "color:var(--muted, #888)")),
    "font-size:12px;line-height:1.45;display:grid;gap:4px;padding:8px 10px;border:1px solid var(--border-strong, rgba(128,128,128,.4));border-radius:12px");
  }
  async function publish(r) {
    if (!PUSHES.has(r.forge) || mine[r.forge] === false) return;
    busy.add(r.sessionKey);
    v.pause(true);
    v.rerender();
    try {
      const pending = confirming.get(r.sessionKey);
      confirming.delete(r.sessionKey);
      const bitbucket = r.forge === BITBUCKET;
      const next = pending
        ? await (bitbucket ? confirmBitbucket : confirmRepo)(v.host, v.agent(), r, pending, v.say)
        : bitbucket
          ? await publishBitbucket(v.host, v.agent(), r, v.say, r.base ? baseOf(r) : undefined)
          : await publishRepo(v.host, v.agent(), r, v.say);
      if (next?.connect) { mine[r.forge] = null; showConnect = r.forge; v.reveal?.(); }
      else {
        // It is connected: look again rather than keep "not connected".
        if (mine[r.forge] === null) { mine[r.forge] = undefined; askedAt[r.forge] = 0; if (showConnect === r.forge) showConnect = null; }
        if (next) confirming.set(r.sessionKey, next);
      }
    } catch (err) {
      v.say(`${nameOf(r)}: ${String(err?.message ?? err)}`);
    } finally {
      busy.delete(r.sessionKey);
      v.pause(false);
      await v.refresh();
    }
  }
  async function catchUp(list) {
    list.forEach((r) => catching.add(r.sessionKey));
    v.rerender();
    try { for (const r of list) await catchUpRepo(v.host, v.agent(), r, v.say); } finally {
      list.forEach((r) => catching.delete(r.sessionKey));
      await v.refresh();
    }
  }
  async function pushAll(list) {
    for (const r of list) await publish(r);
  }

  // The card is wtc status's tables: a column per fact, always in its place,
  // "·" where there is none. What can be done about a fact is done on it: the
  // ↑ count pushes, the ↓ count catches up, a conflict is resolved, a
  // service's state starts or stops it.
  const DOT = () => css(el("span", { textContent: "·" }), "color:var(--muted, #888)");
  const muted = (text, title) => css(el("span", { textContent: text, title: title ?? "" }), "color:var(--muted, #888)");
  const toned = (text, tone, title) => css(el("span", { textContent: text, title: title ?? "" }), `color:${tone}`);
  // A fact that is also an action: a count in a pill.
  const count = (text, title, tone, run) => {
    const b = css(el("button", { type: "button", textContent: String(text), title, ariaLabel: title }),
      `border:1px solid var(--border-strong, rgba(128,128,128,.4));background:transparent;color:${tone};cursor:pointer;font:inherit;font-size:12px;line-height:16px;padding:0 6px;border-radius:999px;white-space:nowrap;font-variant-numeric:tabular-nums`);
    b.onclick = async (e) => {
      e?.preventDefault();
      e?.stopPropagation();
      b.disabled = true;
      try { await run(); } catch (err) { v.say(String(err?.message ?? err)); } finally { b.disabled = false; }
    };
    return b;
  };
  const working = () => muted("…", "Working");
  // columns: { head, title, width, align, under }. Narrow, a column marked
  // under (a repository's branch, a service's repository) goes below the
  // cell before it, so that every fact keeps its column under one header,
  // as wtc status keeps them when it narrows.
  const table = (columns, rows, narrow = false) => {
    const align = (c) => (c.align === "center" ? "justify-self:center;" : "");
    const shown = columns.map((c, i) => i).filter((i) => !(narrow && columns[i].under));
    const grid = css(el("div", { className: "wtc-table" }),
      `display:grid;grid-template-columns:${shown.map((i) => columns[i].width ?? "auto").join(" ")};column-gap:${narrow ? 8 : 14}px;row-gap:${narrow ? 6 : 4}px;align-items:center;font-size:12px;font-variant-numeric:tabular-nums`);
    const empty = (value) => value == null || value === "" || value === 0;
    grid.append(...shown.map((i) => css(el("span", { title: columns[i].title ?? "" }, columns[i].head ?? ""),
      `${align(columns[i])}font-size:10.5px;letter-spacing:.06em;text-transform:uppercase;color:var(--muted, #888);white-space:nowrap`)));
    for (const row of rows) {
      grid.append(...shown.map((i) => {
        const c = columns[i];
        const below = narrow && columns[i + 1]?.under && !empty(row[i + 1]) ? row[i + 1] : null;
        const value = empty(row[i]) ? DOT() : row[i];
        if (below) {
          return css(el("span", {}, css(el("span", {}, value), "display:flex;align-items:center;gap:6px;min-width:0;overflow:hidden;white-space:nowrap"),
            css(el("span", {}, below), "display:block;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:11px")),
          `${align(c)}min-width:0;display:grid;gap:1px`);
        }
        return css(el("span", {}, value), `${align(c)}min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap`);
      }));
    }
    return grid;
  };

  // REPO BRANCH ± ↑ ↓ PR C M R, as in wtc status.
  const REPO_COLUMNS = [
    { head: "Repo", width: "minmax(0,max-content)" },
    { head: "Branch", width: "minmax(0,1fr)", under: true },
    { head: "±", title: "Changed files not committed yet", align: "center" },
    { head: "↑", title: "Commits to push: click to push them, opening a draft pull request if there is none", align: "center" },
    { head: "↓", title: "Commits on the base not merged in: click to catch up", align: "center" },
    { head: "PR", title: "The pull request" },
    { head: "C", title: "CI", align: "center" },
    { head: "M", title: "Merges with its base: a conflict is resolved by the conversation", align: "center" },
    { head: "R", title: "Review", align: "center" },
  ];
  const repoName = (r) => {
    const href = webUrlOf(r);
    const label = css(el(href ? "a" : "span", { textContent: nameOf(r) ?? "?", title: [r.owner ? `${r.owner}/${nameOf(r)}` : nameOf(r), r.path].filter(Boolean).join("\n") }),
      "color:var(--text-strong, inherit);font-weight:600;text-decoration:none;overflow:hidden;text-overflow:ellipsis");
    if (href) Object.assign(label, { href, target: "_blank", rel: "noopener noreferrer" });
    return css(el("span", {}, forgeIcon(r.forge, 13), label), "display:inline-flex;align-items:center;gap:6px;min-width:0;max-width:100%");
  };
  // ↑: the push, as a button only where the gateway pushes it (GitHub and,
  // with its plugin, Bitbucket, each with the person's own account); a
  // confirmation waiting is "confirm ↑".
  function pushCell(r, n) {
    if (!pushable(r)) return null;
    const label = n ? `↑ ${n}` : "↑";
    const what = [r.dirty ? plural(r.dirty, "file") : "", n ? plural(n, "commit") : ""].filter(Boolean).join(" and ");
    const account = r.forge === BITBUCKET ? "Bitbucket login" : "GitHub account";
    if (!PUSHES.has(r.forge) || mine[r.forge] === false) {
      const why = r.forge === BITBUCKET ? "The gateway has no Bitbucket publish (the openclaw-bitbucket plugin, 0.3.0 or later)" : `The gateway does not push ${r.forge ?? "this forge"}'s repositories`;
      return toned(label, TONE.pending, `${what} to push. ${why}, and the card does not hand pushing to the agent.`);
    }
    const pending = confirming.get(r.sessionKey);
    if (pending) {
      const as = pending.forge === BITBUCKET ? bitbucketAccount(pending.confirmation) : `@${pending.confirmation.account?.login ?? "you"}`;
      return count(`confirm ${label}`, `Push ${what} of ${nameOf(r)} as ${as}`, TONE.success, () => publish(r));
    }
    // Bitbucket publishes commits only; files not committed stay behind.
    const left = r.forge === BITBUCKET && r.dirty ? ` Uncommitted files stay behind: ask the conversation to commit them first.` : "";
    // Asked again on the click, so a connection just made is used at once.
    if (mine[r.forge] === null) return count(label, `${what} to push. Pushing needs your own ${account}: click to push, or for how to connect it`, TONE.none, () => publish(r));
    return count(label, `${what} to push. ${pushTitle(r.pullRequest)}, with your own ${account}, after you confirm.${proposalTitle(r)}${left}`, TONE.pending, () => publish(r));
  }
  function repoRow(r) {
    const pr = r.pullRequest;
    const open = pr?.state === "open";
    const n = toPush(r);
    const ci = pr?.checks && pr.checks !== "none"
      ? toned({ success: "✓", failure: "✗", pending: "●" }[pr.checks] ?? "·", TONE[pr.checks] ?? TONE.none, `${ciText(pr)}: ${pr.passed ?? 0} passed, ${pr.failed ?? 0} failed, ${pr.running ?? 0} running`)
      : null;
    const conflict = open && pr.mergeable === "conflicting"
      ? count("✗", `#${pr.number} conflicts with ${baseOf(r)}. Resolve: ask the conversation to merge the base in and resolve the conflicts`, TONE.failure,
        () => askConversation(v.host, v.key(), `Pull request #${pr.number} of ${nameOf(r)} conflicts with its base. Merge the base branch into ${nameOf(r)}, resolve the conflicts and commit. Don't push: pushing is mine, through the gateway.`))
      : null;
    const review = open ? { approved: toned("✓", TONE.success, "approved"), changes_requested: toned("✗", TONE.failure, "changes requested"), review_required: muted("○", "review required") }[pr.review] ?? null : null;
    return [
      repoName(r),
      r.error ? toned(r.error, TONE.failure, r.error)
        : css(el("code", { textContent: r.branch ?? `at ${baseOf(r)}`, title: r.branch ? `${r.branch}, ${r.ahead ?? 0} ahead of ${baseOf(r)}` : "No branch yet: the first push creates one" }), "font-size:12px;color:var(--muted, #888)"),
      r.dirty ? toned(String(r.dirty), TONE.pending, `${plural(r.dirty, "file")} changed, not committed yet`) : null,
      busy.has(r.sessionKey) ? working() : pushCell(r, n),
      catching.has(r.sessionKey) ? working() : r.behind ? count(`↓ ${r.behind}`, `${r.behind} new on ${baseOf(r)}. Catch up: merge them in; never rebases, never pushes`, TONE.pending, () => catchUp([r])) : null,
      pr ? css(el("span", { title: pr.title ?? "" }, link(`#${pr.number}`, pr.url), pr.draft && open ? muted("draft") : pr.state !== "open" ? muted(pr.state) : null), "display:inline-flex;gap:4px") : null,
      ci, conflict, review,
    ];
  }

  // ▶/■ NAME REPO STATE URL, then a column per way in (gateway, cloudflare…)
  // and the + that opens a new one.
  const slug = (url) => { try { return new URL(url).hostname.split(".")[0]; } catch { return url; } };
  function serviceTable(groups, data, narrow = false) {
    const ways = [];
    for (const g of groups) {
      if (portalsOf(g, data).length && !ways.includes("gateway")) ways.push("gateway");
      for (const t of g.tunnels) if (!ways.includes(tunnelName(t))) ways.push(tunnelName(t));
    }
    const all = groups.length > 1 && !v.svc.busy("*")
      ? css(el("span", {},
        groups.some((g) => !g.running) ? stateGlyph("▶", "Start all services", TONE.none, () => v.svc.run({ op: "up" }, "Starting all services…")) : null,
        groups.some((g) => g.running) ? stateGlyph("■", "Stop all services", TONE.success, () => v.svc.run({ op: "down" }, "Stopping all services…")) : null),
      "display:inline-flex;gap:2px")
      : v.svc.busy("*") ? busyMark() : "";
    const columns = [
      { head: all, width: groups.length > 1 ? "max-content" : "14px" },
      { head: "Service", width: narrow ? "minmax(0,1fr)" : "minmax(0,max-content)" },
      { head: "Repo", width: "minmax(0,max-content)", under: true },
      { head: "State" },
      { head: "URL", title: "Where it listens on the gateway" },
      ...ways.map((w) => ({ head: w, title: w === "gateway" ? "OpenClaw portals" : `${w} tunnels` })),
      { head: "", width: "minmax(0,1fr)" },
    ];
    const rows = groups.map((g) => [
      stateButton(g, v.svc),
      css(el("span", { textContent: g.label, title: g.group ?? "" }), "font-weight:600;color:var(--text-strong, inherit)"),
      g.repo ? muted(g.repo) : null,
      g.loose ? toned("separate tunnel", TONE.pending, "Started apart from the service it exposes")
        : toned(g.state === "absent" || g.state === "idle" ? "stopped" : g.state, g.failed ? TONE.failure : g.running ? TONE.success : TONE.none),
      g.url && !g.loose ? address(g.url) : null,
      ...ways.map((w) => {
        if (w === "gateway") {
          const ps = portalsOf(g, data);
          return ps.length ? portalNode(g, ps[0], v.svc, "open") : null;
        }
        const t = g.tunnels.find((x) => tunnelName(x) === w);
        return t ? tunnelNode(g, t, v.svc, { label: t.url ? slug(t.url) : "open", opener: (title, run) => stateGlyph("▶", title, TONE.none, run) }) : null;
      }),
      css(el("span", {}, g.loose
        ? count("Consolidate", "Ask the conversation to open it with wtc svc tunnel instead", "inherit", () => askConversation(v.host, v.key(), consolidate(g)))
        : newTunnel(g, v.svc, data, { compact: true }) ?? ""), "justify-self:end"),
    ]);
    return table(columns, rows, narrow);
  }

  // Collapsed, the same facts summed, in the same order: ± ↑ ↓, the pull
  // requests, the services. ↑ pushes and ↓ catches up every repository.
  function totals(repos, groups) {
    // An action says what it is (↑ 3), a fact has its head before it (± 1).
    const slot = (head, value, title) => css(el("span", { title: title ?? "" }, value?.tagName === "BUTTON" ? null : muted(head), value ?? DOT()),
      "display:inline-flex;align-items:center;gap:4px;white-space:nowrap;font-size:12px;font-variant-numeric:tabular-nums");
    const files = repos.reduce((n, r) => n + (r.dirty ?? 0), 0);
    const ready = repos.filter((r) => pushable(r) && PUSHES.has(r.forge) && mine[r.forge] !== false);
    const commits = ready.reduce((n, r) => n + toPush(r), 0);
    const behind = repos.filter((r) => r.behind > 0);
    const prs = repos.filter((r) => r.pullRequest?.state === "open");
    const worst = prs.map((r) => r.pullRequest).sort((a, b) => [TONE.failure, TONE.pending, TONE.success, TONE.none].indexOf(prTone(a)) - [TONE.failure, TONE.pending, TONE.success, TONE.none].indexOf(prTone(b)))[0];
    const out = [
      slot("±", files ? toned(String(files), TONE.pending) : null, "Changed files not committed yet"),
      slot("↑", ready.some((r) => busy.has(r.sessionKey)) ? working() : ready.length
        ? count(commits ? `↑ ${commits}` : "↑", `Push ${ready.map(nameOf).join(", ")}, each to its pull request or a new draft one`, TONE.pending, () => pushAll(ready)) : null),
      slot("↓", behind.some((r) => catching.has(r.sessionKey)) ? working() : behind.length
        ? count(`↓ ${behind.reduce((n, r) => n + r.behind, 0)}`, `Catch up ${behind.map((r) => `${nameOf(r)} (${r.behind})`).join(", ")}: merge the base in; never rebases, never pushes`, TONE.pending, () => catchUp(behind)) : null),
      slot("PR", prs.length ? toned(`${prs.length}${worst && prTone(worst) === TONE.failure ? " ✗" : worst && prTone(worst) === TONE.success ? " ✓" : ""}`, prTone(worst),
        prs.map((r) => `${nameOf(r)} #${r.pullRequest.number}: ${[r.pullRequest.title, ...prState(r.pullRequest)].filter(Boolean).join(", ")}`).join("\n")) : null),
    ];
    if (groups.length) {
      const up = groups.filter((g) => g.running).length;
      const glyph = v.svc.busy("*") ? busyMark() : up < groups.length
        ? stateGlyph("▶", groups.length > 1 ? "Start all services" : `Start ${groups[0].label}`, TONE.none, () => v.svc.run({ op: "up" }, "Starting all services…"))
        : stateGlyph("■", groups.length > 1 ? "Stop all services" : `Stop ${groups[0].label}`, TONE.success, () => v.svc.run({ op: "down" }, "Stopping all services…"));
      out.push(css(el("span", { title: groups.map((g) => `${g.label}: ${g.state}`).join("\n") }, muted("svc"), glyph,
        toned(`${up}/${groups.length}`, groups.some((g) => g.failed) ? TONE.failure : up ? TONE.success : TONE.none)),
      "display:inline-flex;align-items:center;gap:4px;white-space:nowrap;font-size:12px;font-variant-numeric:tabular-nums"));
    }
    return out;
  }

  function prTable(repos, narrow = false) {
    const open = repos.filter((r) => r.pullRequest);
    const columns = [
      { head: "PR" },
      { head: "Repo", width: "minmax(0,max-content)" },
      { head: "State" },
      { head: "C", title: "CI", align: "center" },
      { head: "M", title: "Merges with its base", align: "center" },
      { head: "R", title: "Review", align: "center" },
      { head: "Title", width: "minmax(0,1fr)" },
    ];
    return table(columns, open.map((r) => {
      const pr = r.pullRequest;
      const [, , , , , , ci, conflict, review] = repoRow(r);
      return [link(`#${pr.number}`, pr.url), muted(nameOf(r)), muted(pr.draft && pr.state === "open" ? "draft" : pr.state), ci, conflict, review,
        css(el("span", { textContent: pr.title ?? "", title: pr.title ?? "" }), "overflow:hidden;text-overflow:ellipsis")];
    }), narrow);
  }
  return { table, checkAccount, notice, repoTable: (repos, narrow) =>
    // Narrow, the repository (with its branch below) takes the room the
    // branch had.
    table(narrow ? REPO_COLUMNS.map((c, i) => (i === 0 ? { ...c, width: "minmax(0,1fr)" } : c)) : REPO_COLUMNS, repos.map(repoRow), narrow), serviceTable, totals, prTable };
}

function mountComposer(container, context) {
  let ctx = context;
  let watch = null;
  let key = null;
  let entry = null;
  let note = "";

  const card = css(el("details", { className: "wtc-collection-card", open: remembered() }),
    "box-sizing:border-box;width:calc(100% - var(--chat-composer-side-inset, 36px));max-width:var(--chat-thread-max-width, 48rem);margin:8px auto 0;background:var(--card, #fff);color:var(--text, inherit);border:1px solid var(--border, rgba(0,0,0,.08));border-radius:25px;font:inherit;font-size:14px");
  const summary = css(el("summary"), "list-style:none;display:flex;align-items:center;gap:8px;padding:6px 8px 6px 13px;cursor:pointer");
  const detail = css(el("div"), "padding:4px 18px 14px 18px;display:grid;gap:14px");
  card.append(summary, detail);
  card.addEventListener("toggle", () => remember(card.open));
  const composer = el("div");
  container.append(composer);
  const unmountDefault = ctx.mountDefault(composer);

  const iconButton = (text, title, run) => {
    const b = css(el("button", { type: "button", textContent: text, title }),
      "border:0;background:transparent;color:var(--muted, #888);cursor:pointer;font:inherit;padding:2px 6px;border-radius:8px");
    b.onclick = (e) => { e.preventDefault(); e.stopPropagation(); run(); };
    return b;
  };
  const say = (text) => { note = text; if (entry) render(entry); };
  // Narrow (a phone, a narrow window), rows wrap into two lines.
  let narrow = false;
  const sized = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(([e]) => {
    const now = e.contentRect.width > 0 && e.contentRect.width < NARROW_PX;
    if (now !== narrow) { narrow = now; if (entry) render(entry); }
  });
  sized?.observe(card);
  const svcBusy = new Set();
  const svc = {
    busy: (k) => svcBusy.has(k),
    ask: (text) => askConversation(ctx.host, key, text),
    async run(payload, doing, k = payload.group ?? "*") {
      svcBusy.add(k);
      say(doing);
      try {
        const r = await svcRequest(ctx.host, key, agentOf(ctx), payload);
        note = "";
        return r;
      } catch (err) {
        note = String(err?.message ?? err);
      } finally {
        svcBusy.delete(k);
        await watch?.refresh();
        if (entry) render(entry);
      }
    },
  };
  const tables = collectionTables({
    get host() { return ctx.host; }, key: () => key, agent: () => agentOf(ctx), svc, say,
    // What a push needs (the how-to of connecting GitHub) shows open.
    reveal: () => { if (!card.open) { card.open = true; remember(true); } },
    refresh: () => watch?.refresh(), pause: (on) => watch?.pause(on), rerender: () => entry && render(entry),
  });

  function render(next) {
    entry = next;
    const repos = entry?.data?.repos ?? [];
    if (!ctx.presented || ctx.signal.aborted || repos.length === 0) {
      card.remove();
      return;
    }
    const groups = serviceGroups(entry.data.services ?? []);
    summary.replaceChildren(
      brand("14px"),
      css(el("span", {}, ...(card.open ? [] : tables.totals(repos, groups))), "flex:1;min-width:0;display:flex;flex-wrap:wrap;align-items:center;gap:14px;padding-left:6px"),
      // What an action is doing or why it failed shows here.
      css(el("span", {}, note ? css(el("span", { textContent: note, title: note }), "color:var(--muted, #888);font-size:12px;max-width:22em;overflow:hidden;text-overflow:ellipsis") : null,
        iconButton("↻", `Updated ${ago(entry.at)}. Refresh`, () => void watch?.refresh()),
        iconButton("⤢", "Open the wtc Collection panel", () => ctx.host.ui.openPanel("collection", { sessionKey: key, agentId: agentOf(ctx) })),
        css(el("span", { textContent: card.open ? "⌃" : "⌄" }), "color:var(--muted, #888);padding:0 6px")),
      "flex:none;margin-left:auto;display:inline-flex;align-items:center;gap:2px;white-space:nowrap"),
    );
    tables.checkAccount(repos);
    detail.replaceChildren(...[tables.repoTable(repos, narrow), tables.notice(), groups.length ? tables.serviceTable(groups, entry.data, narrow) : null].filter(Boolean));
    if (card.parentNode !== container) container.prepend(card);
  }
  card.addEventListener("toggle", () => entry && render(entry));

  const keyOf = (c) => c.props?.sessionKey ?? c.props?.session?.key;
  const agentOf = (c) => c.props?.agentId ?? c.props?.session?.agentId;
  function follow(next) {
    const sessionKey = keyOf(next);
    if (sessionKey === key) return;
    watch?.dispose();
    watch = null;
    key = sessionKey;
    entry = null;
    note = "";
    card.remove();
    if (sessionKey) watch = watchCollection(next.host, sessionKey, agentOf(next), render);
  }
  follow(ctx);
  // "Updated … ago" stays honest between refreshes.
  const tick = setInterval(() => { if (entry && card.isConnected) render(entry); }, 15000);
  return {
    update(next) {
      ctx = next;
      follow(next);
      if (entry) render(entry);
    },
    dispose() {
      clearInterval(tick);
      sized?.disconnect();
      watch?.dispose();
      card.remove();
      unmountDefault?.();
    },
  };
}

// The Collection panel beside a conversation: the whole collection at once —
// a card per repository with its branch, what is not published yet and its
// pull request, the services, and adding a repository.
function mountCollection(container, context) {
  let ctx = context;
  let entry = null;
  let watch = null;

  const root = css(el("section", { className: "wtc-collection" }),
    "position:relative;padding:14px 16px;font:inherit;font-size:14px;color:var(--text, inherit);overflow:auto;height:100%;box-sizing:border-box;display:grid;gap:14px;align-content:start");
  container.append(root);

  const muted = (text, extra = "") => css(el("span", { textContent: text }), `color:var(--muted, #888);font-size:12px;${extra}`);
  const mono = (text, title) => css(el("code", { textContent: text, title: title ?? text }),
    "font-family:var(--mono, ui-monospace, monospace);font-size:12px;color:var(--muted, #888);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0");
  const heading = (text, ...extra) => css(el("div", {}, css(el("span", { textContent: text }), "font-size:12px;font-weight:600;color:var(--muted, #888)"), ...extra),
    "display:flex;align-items:center;gap:8px;margin-bottom:6px");
  const row = (...kids) => css(el("div", {}, ...kids), "display:flex;align-items:center;gap:8px;min-width:0;flex-wrap:wrap");
  const button = (text, title, run, primary) => {
    const b = css(el("button", { textContent: text, type: "button", title }),
      `border:1px solid ${primary ? "transparent" : "var(--border-strong, rgba(128,128,128,.4))"};background:${primary ? "var(--accent, #bd4531)" : "transparent"};color:${primary ? "var(--accent-foreground, #fff)" : "inherit"};` +
      "font:inherit;font-size:12px;line-height:20px;padding:1px 10px;border-radius:var(--radius-full, 999px);cursor:pointer;white-space:nowrap");
    b.onclick = async () => {
      b.disabled = true;
      b.style.opacity = ".6";
      try { await run(); } catch (err) { say(String(err?.message ?? err)); } finally { b.disabled = false; b.style.opacity = ""; }
    };
    return b;
  };
  const iconButton = (text, title, run) => {
    const b = css(el("button", { textContent: text, type: "button", title }),
      "border:0;background:transparent;color:var(--muted, #888);font:inherit;font-size:15px;padding:2px 6px;border-radius:8px;cursor:pointer");
    b.onclick = run;
    return b;
  };

  const header = el("div");
  const log = css(el("div"), "font-size:12px;color:var(--muted, #888);min-height:0");
  const body = css(el("div"), "display:grid;gap:16px");
  root.append(header, log, body);
  let note = "";
  const say = (text) => { note = text; log.textContent = text; log.style.minHeight = text ? "1em" : "0"; };

  const sessionKeyOf = () => entry?.data?.collection?.sessionKey ?? ctx.props.sessionKey;
  const svcBusy = new Set();
  const svc = {
    busy: (k) => svcBusy.has(k),
    ask: (text) => askConversation(ctx.host, sessionKeyOf(), text),
    async run(payload, doing, k = payload.group ?? "*") {
      svcBusy.add(k);
      say(doing);
      render();
      try {
        const r = await svcRequest(ctx.host, sessionKeyOf(), ctx.props.agentId, payload);
        say("");
        return r;
      } catch (err) {
        say(String(err?.message ?? err));
      } finally {
        svcBusy.delete(k);
        await watch?.refresh();
        render();
      }
    },
  };

  const tables = collectionTables({
    get host() { return ctx.host; }, key: sessionKeyOf, agent: () => ctx.props.agentId, svc, say,
    refresh: () => watch?.refresh(), pause: (on) => watch?.pause(on), rerender: () => render(),
  });
  // The panel is often narrow, beside the conversation or on a phone: rows
  // then wrap into two lines.
  let narrow = false;
  const sized = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(([e]) => {
    const now = e.contentRect.width > 0 && e.contentRect.width < NARROW_PX;
    if (now !== narrow) { narrow = now; render(); }
  });
  sized?.observe(root);

  // Adding a repository: what is typed is a GitHub owner/name, a GitHub or
  // Bitbucket URL, or a search. The search offers the projects the gateway
  // has, GitHub repositories (the gateway's own search, with the person's
  // GitHub account when they connected one) and, with a Bitbucket login, the
  // person's Bitbucket repositories. The repository is put on the gateway
  // first, then the collection's session is asked to take it in.
  const GITHUB = /^(?:https:\/\/github\.com\/|git@github\.com:)?([A-Za-z0-9][A-Za-z0-9-]*)\/([A-Za-z0-9._-]+?)(?:\.git)?\/?$/;
  const BITBUCKET = /^(?:https:\/\/(?:[^@/]+@)?|git@)?bitbucket\.org[/:]([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/;
  const sourceOf = (spec) => {
    const text = spec.trim();
    let m = text.match(BITBUCKET);
    if (m) return { forge: "bitbucket", fullName: `${m[1]}/${m[2]}`, name: m[2], cloneUrl: `https://bitbucket.org/${m[1]}/${m[2]}.git` };
    m = text.match(GITHUB);
    if (m) return { forge: "github", fullName: `${m[1]}/${m[2]}`, name: m[2], cloneUrl: `https://github.com/${m[1]}/${m[2]}.git` };
    return null;
  };

  async function addRepository(source, branch) {
    const sessionKey = entry?.data?.collection?.sessionKey ?? ctx.props.sessionKey;
    let ask = source.forge === "project" ? source.name : source.cloneUrl.replace(/\.git$/, "");
    if (source.forge === "github") {
      say(`Adding ${source.fullName} to the gateway…`);
      await ctx.host.request("projects.add", { gitUrl: source.cloneUrl }, { timeoutMs: null });
    } else if (source.forge === "bitbucket") {
      say(`Cloning ${source.fullName} onto the gateway…`);
      const res = await ctx.host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId: "clone-repo", sessionKey, agentId: ctx.props.agentId, payload: { url: source.cloneUrl } }, { timeoutMs: null });
      if (res?.ok === false) throw new Error(res.error ?? "clone failed");
      const cloned = res?.result ?? res;
      try {
        await ctx.host.request("projects.register", { path: cloned.path, name: cloned.name });
      } catch (err) {
        // Already registered is fine; anything else is the person's to see.
        if (!/exist|already/i.test(String(err?.message ?? err))) throw err;
      }
    }
    const message = branch
      ? `Add ${ask} to this collection, on branch ${branch}: wtc add-repo ${ask} --base ${branch}`
      : `Add ${ask} to this collection.`;
    await ctx.host.request("sessions.send", { key: sessionKey, message });
    say(`${source.fullName ?? source.name} is on the gateway; the collection's session is adding it.`);
  }

  // Before anything is cloned or added: which repository this is, with a
  // link to check it, and the branch the collection works against.
  function confirmAdd(source) {
    const overlay = css(el("div"), "position:absolute;inset:0;background:color-mix(in srgb, var(--bg, #000) 55%, transparent);display:flex;align-items:flex-start;justify-content:center;padding:48px 16px;z-index:10");
    const dialog = css(el("div", { role: "dialog", ariaModal: "true", ariaLabel: "Add repository" }),
      "width:100%;max-width:420px;background:var(--card, #fff);color:var(--text, inherit);border:1px solid var(--border, rgba(128,128,128,.3));border-radius:var(--radius-lg, 14px);padding:16px;display:grid;gap:12px;box-shadow:0 12px 40px rgba(0,0,0,.25)");
    const close = () => { overlay.remove(); document.removeEventListener("keydown", onKey); };
    const onKey = (e) => { if (e.key === "Escape") close(); };
    document.addEventListener("keydown", onKey);
    overlay.onmousedown = (e) => { if (e.target === overlay) close(); };

    const [owner, name] = String(source.fullName ?? source.name).includes("/") ? String(source.fullName).split("/") : [null, source.name];
    const href = source.webUrl ?? (source.forge === "github" ? `https://github.com/${source.fullName}` : source.forge === "bitbucket" ? `https://bitbucket.org/${source.fullName}` : null);
    const where = { github: "GitHub", bitbucket: "Bitbucket", project: "already on the gateway" }[source.forge];
    const select = css(el("select", { disabled: true }, el("option", { textContent: "Reading branches…" })),
      "font:inherit;font-size:13px;color:inherit;background:var(--bg-elevated, transparent);border:1px solid var(--border, rgba(128,128,128,.3));border-radius:8px;padding:4px 8px");
    const status = muted("");
    let defaultBranch = null;
    const add = button("Add", "Put the repository on the gateway and add it to this collection", async () => {
      const branch = select.disabled ? null : select.value;
      close();
      await addRepository(source, branch && branch !== defaultBranch ? branch : null);
      await watch?.refresh();
    }, true);
    add.disabled = true;
    add.style.opacity = ".6";
    dialog.append(
      css(el("div", { textContent: "Add repository" }), "font-weight:600;font-size:15px;color:var(--text-strong, inherit)"),
      css(el("div", {},
        repoTitle({ forge: source.forge === "project" ? null : source.forge, owner, repo: name }),
        css(el("div", {}, muted([where, source.private === true ? "private" : source.private === false ? "public" : ""].filter(Boolean).join(" · ")),
          href ? css(el("span", {}, link("Open to check ↗", href)), "font-size:12px") : null), "display:flex;gap:10px;align-items:center;flex-wrap:wrap"),
        source.description ? css(el("div", { textContent: source.description }), "font-size:13px") : null),
      "display:grid;gap:4px"),
      css(el("label", {}, css(el("span", { textContent: "Default branch" }), "font-size:12px;font-weight:600;color:var(--muted, #888)"), select,
        muted("The collection's worktree starts from it, and is compared with, caught up from and published to it.")),
      "display:grid;gap:4px"),
      status,
      css(el("div", {}, button("Cancel", "", async () => close()), add), "display:flex;gap:8px;justify-content:flex-end"),
    );
    overlay.append(dialog);
    root.append(overlay);

    const payload = source.forge === "project" ? { project: source.name } : { url: source.cloneUrl };
    ctx.host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId: "repo-branches", sessionKey: sessionKeyOf(), agentId: ctx.props.agentId, payload })
      .then((res) => {
        if (res?.ok === false) throw new Error(res.error ?? "cannot read the branches");
        const { defaultBranch: head, branches } = res?.result ?? res;
        defaultBranch = head ?? source.defaultBranch ?? null;
        // The gateway's own working branches are not a base to choose.
        const sorted = [...new Set([defaultBranch, ...(branches ?? []).filter((b) => !b.startsWith("openclaw/"))].filter(Boolean))];
        select.replaceChildren(...sorted.map((b) => el("option", { value: b, textContent: b === defaultBranch ? `${b} (the repository's default)` : b, selected: b === defaultBranch })));
        select.disabled = sorted.length === 0;
        status.textContent = `${sorted.length} ${sorted.length === 1 ? "branch" : "branches"}`;
      })
      .catch((err) => {
        select.replaceChildren(el("option", { textContent: "the repository's default" }));
        status.textContent = `${String(err?.message ?? err)} The repository's default branch is used.`;
      })
      .finally(() => {
        const already = (entry?.data?.repos ?? []).some((r) => String(r.repo ?? r.project).toLowerCase() === String(name).toLowerCase()
          && (!owner || !r.owner || String(r.owner).toLowerCase() === String(owner).toLowerCase()));
        if (already) status.textContent = "Already in this collection.";
        else { add.disabled = false; add.style.opacity = ""; }
      });
  }

  const adder = (() => {
    const input = css(el("input", { placeholder: "Search, owner/name, or a GitHub or Bitbucket URL", type: "text" }),
      "flex:1;min-width:0;font:inherit;font-size:13px;color:inherit;background:var(--bg-elevated, transparent);border:1px solid var(--border, rgba(128,128,128,.3));border-radius:var(--radius-full, 999px);padding:4px 12px;outline:none");
    const results = css(el("div"), "display:none;border:1px solid var(--border, rgba(128,128,128,.25));border-radius:var(--radius-lg, 14px);background:var(--card, transparent);overflow:hidden");
    let options = [];
    let active = 0;
    let seq = 0;
    let timer = null;
    let projects = null;

    const choose = (source) => {
      results.style.display = "none";
      input.value = "";
      confirmAdd(source);
    };
    const forgeLabel = { github: "GitHub", bitbucket: "Bitbucket", project: "On the gateway" };
    function show(list, footer) {
      options = list;
      active = 0;
      const rows = list.map((o, i) => {
        const r = css(el("div", { title: o.description ?? "" },
          o.forge === "project" ? css(el("span", { textContent: "on gateway", title: forgeLabel.project }), "flex:none;font-size:11px;color:var(--muted, #888)") : forgeIcon(o.forge),
          css(el("span", { textContent: o.fullName ?? o.name }), "font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0"),
          o.private ? muted("private") : null,
          o.description ? muted(o.description, "overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0;flex:1") : null),
        `display:flex;align-items:center;gap:8px;padding:6px 12px;cursor:pointer;${i === active ? "background:var(--bg-hover, rgba(128,128,128,.12))" : ""}`);
        r.onmousedown = (e) => { e.preventDefault(); choose(o); };
        return r;
      });
      if (footer) rows.push(css(el("div", {}, muted(footer)), "padding:6px 12px"));
      results.replaceChildren(...rows);
      results.style.display = rows.length ? "block" : "none";
    }
    async function search(text) {
      const mine = ++seq;
      const direct = sourceOf(text);
      const q = text.trim().toLowerCase();
      if (q.length < 2) return show(direct ? [direct] : []);
      const sessionKey = entry?.data?.collection?.sessionKey ?? ctx.props.sessionKey;
      projects ??= ctx.host.request("projects.list", {}).then((r) => r?.projects ?? []).catch(() => []);
      const [onGateway, github, bitbucket] = await Promise.all([
        projects.then((all) => all.filter((p) => p.source !== "workspace" && String(p.displayName ?? p.id).toLowerCase().includes(q))
          .map((p) => ({ forge: "project", name: p.id, fullName: p.displayName ?? p.id }))),
        direct ? Promise.resolve(null) : ctx.host.request("projects.searchRemote", { query: text.trim() }).catch(() => null),
        ctx.host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId: "repo-search", sessionKey, agentId: ctx.props.agentId, payload: { query: text.trim() } })
          .then((r) => r?.result ?? r).catch(() => null),
      ]);
      if (mine !== seq) return;
      const have = new Set((entry?.data?.repos ?? []).map((r) => String(r.repo ?? r.project).toLowerCase()));
      const list = [
        ...(direct ? [direct] : []),
        ...onGateway,
        ...(bitbucket?.repos ?? []),
        ...(github?.projects ?? []).map((p) => ({ forge: "github", name: p.name, fullName: p.fullName, cloneUrl: p.cloneUrl, webUrl: p.webUrl, private: p.private, description: p.description, defaultBranch: p.defaultBranch })),
      ].filter((o, i, all) => !have.has(String(o.name).toLowerCase()) && all.findIndex((x) => x.forge === o.forge && x.fullName === o.fullName) === i).slice(0, 10);
      const notes = [];
      if (github?.credential === "missing") notes.push("GitHub: public repositories; connect your GitHub account to search yours");
      if (bitbucket && !bitbucket.bitbucket?.connected) notes.push("Bitbucket: connect your account on the Bitbucket tab to search yours");
      if (bitbucket?.bitbucket?.error) notes.push(`Bitbucket: ${bitbucket.bitbucket.error}`);
      show(list, notes.join(" · "));
    }
    input.oninput = () => {
      clearTimeout(timer);
      timer = setTimeout(() => void search(input.value), 300);
    };
    input.onkeydown = (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        if (!options.length) return;
        active = (active + (e.key === "ArrowDown" ? 1 : options.length - 1)) % options.length;
        [...results.children].forEach((r, i) => { if (i < options.length) r.style.background = i === active ? "var(--bg-hover, rgba(128,128,128,.12))" : ""; });
      } else if (e.key === "Enter") {
        const source = options[active] ?? sourceOf(input.value);
        if (source) choose(source);
        else say("Pick a repository from the list, or give owner/name or a GitHub or Bitbucket URL.");
      } else if (e.key === "Escape") results.style.display = "none";
    };
    input.onblur = () => setTimeout(() => { results.style.display = "none"; }, 150);
    input.onfocus = () => { if (options.length) results.style.display = "block"; };
    return css(el("div", {}, input, results), "display:grid;gap:6px");
  })();

  function render() {
    if (!entry?.data) {
      header.replaceChildren(css(el("div", { textContent: entry?.error ? `The collection could not be loaded: ${entry.error}` : "Loading…" }), "color:var(--muted, #888)"));
      body.replaceChildren();
      return;
    }
    const data = entry.data;
    const repos = data.repos ?? [];
    const services = data.services ?? [];
    const branch = collectionBranch(repos);
    const groups = serviceGroups(services);
    header.replaceChildren(
      row(brand("12px")),
      row(css(el("span", { textContent: data.collection.label ?? data.collection.sessionKey }), "font-size:16px;font-weight:600;color:var(--text-strong, inherit)"),
        css(el("span", {}, muted(`Updated ${ago(entry.at)}`), iconButton("↻", "Refresh", () => void watch?.refresh())), "margin-left:auto;display:flex;align-items:center;gap:2px")),
      row(branch ? mono(branch, "The collection's branch; a repository on another one shows its own") : null, branch ? muted("·") : null,
        muted(`${repos.length} ${repos.length === 1 ? "repository" : "repositories"}${groups.length ? ` · ${groups.filter((g) => g.running).length}/${plural(groups.length, "service")} running` : ""}`)),
      data.collection.folder ? css(el("div", {}, mono(data.collection.folder, "The collection's folder: one link per repository to its worktree")), "display:flex;margin-top:2px") : null,
    );

    const repoSection = css(el("div", {}, heading("Repositories")), "display:grid;gap:10px");
    if (repos.length === 0) repoSection.append(muted("No repositories yet. Ask in the conversation, or add one below.", "font-size:13px"));
    else repoSection.append(tables.repoTable(repos, narrow));
    tables.checkAccount(repos);
    const howTo = tables.notice();
    if (howTo) repoSection.append(howTo);
    repoSection.append(adder);

    const prSection = repos.some((r) => r.pullRequest) ? el("div", {}, heading("Pull requests"), tables.prTable(repos, narrow)) : null;

    const serviceSection = el("div", {}, heading("Services"));
    if (groups.length === 0) serviceSection.append(muted(data.servicesNote ?? "None running. Start them in the collection folder with wtc svc.", "font-size:13px"));
    else serviceSection.append(tables.serviceTable(groups, data, narrow));

    const warnings = (data.warnings ?? []).map((w) => css(el("div", { textContent: w }), "font-size:12px;color:var(--warn, #bf8700)"));
    body.replaceChildren(...[repoSection, prSection, serviceSection, ...warnings].filter(Boolean));
    log.textContent = note;
  }

  const show = (next) => { if (!ctx.signal.aborted) { entry = next; render(); } };
  function follow() {
    watch?.dispose();
    watch = null;
    entry = null;
    const { sessionKey, agentId } = ctx.props ?? {};
    if (!sessionKey) {
      header.textContent = "No session selected.";
      return;
    }
    render();
    watch = watchCollection(ctx.host, sessionKey, agentId, show);
  }
  follow();
  const tick = setInterval(() => { if (entry?.data) render(); }, 15000);
  return {
    update(next) {
      const changed = next.props?.sessionKey !== ctx.props?.sessionKey;
      ctx = next;
      if (changed) follow();
    },
    dispose() {
      clearInterval(tick);
      sized?.disconnect();
      watch?.dispose();
      root.remove();
    },
  };
}

export default {
  id: PLUGIN_ID,
  activate(host) {
    const disposers = [
      host.ui.registerPanel({ id: "collection", label: TITLE, icon: "folder", mount: mountCollection }),
      host.ui.registerReplacement({ id: "collection-composer", surface: "composer", label: "Composer with collection", mount: mountComposer }),
      // "header": a button in the chat header. "session": an entry in the
      // session's menu in the sidebar.
      ...["header", "session"].map((placement) =>
        host.ui.registerAction({
          id: `open-collection-${placement}`,
          label: TITLE,
          placement,
          run: ({ sessionKey, agentId }) => host.ui.openPanel("collection", { sessionKey, agentId }),
        })),
    ];
    // The card lives with the message box, so this composer is selected on
    // activation. Customize UI → Built-in switches back for this browser.
    try { host.ui.selectReplacement("composer", "collection-composer"); } catch {}
    return () => disposers.forEach((d) => (typeof d === "function" ? d() : d?.dispose?.()));
  },
};
