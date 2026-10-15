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
const link = (text, href) => (/^https:\/\//.test(href ?? "") ? el("a", { textContent: text, href, target: "_blank", rel: "noopener noreferrer" }) : text);
const uuid = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`);

const prLabel = (pr) => `#${pr.number} ${pr.draft && pr.state === "open" ? "draft" : pr.state}`;
const isRunning = (s) => s.state === "ready" || s.state === "running";
const isFailed = (s) => s.state === "failed" || (s.state === "exited" && s.exit_code > 0);
const serviceState = (s) => `${s.state ?? "?"}${s.exit_code > 0 ? ` (exit ${s.exit_code})` : ""}`;

// Publishes an owner's worktree: commits what is pending, pushes the branch
// and opens or updates a draft pull request.
async function publishRepo(host, agentId, r, say) {
  const name = r.repo ?? r.project;
  say(`Pushing ${name}…`);
  const out = await host.request("sessions.github.publish", {
    sessionKey: r.sessionKey, agentId, idempotencyKey: uuid(), selection: { source: "shared" },
  });
  if (out?.status === "published") say(`${name}: pushed ${out.url ?? ""}`);
  else if (out?.status === "needs_confirmation") say(`${name}: the gateway asks for confirmation; open Publish in the owner session.`);
  else say(`${name}: ${out?.status ?? "done"}${out?.error ? ` — ${out.error.message ?? out.error}` : ""}`);
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
    entry.inflight = call(host, "collection", sessionKey, agentId)
      .then((data) => { entry.data = data; entry.error = null; },
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
const prBadges = (pr) => [
  pr.checks && pr.checks !== "none" ? chip(ciText(pr), TONE[pr.checks], `${pr.passed} passed, ${pr.failed} failed, ${pr.running} running`) : null,
  pr.state === "open" && pr.mergeable === "conflicting" ? chip("conflict", TONE.failure, "Conflicts with its base branch: catch up and resolve") : null,
  pr.state === "open" && pr.review === "approved" ? chip("approved", TONE.success) : null,
  pr.state === "open" && pr.review === "changes_requested" ? chip("changes requested", TONE.failure) : null,
  pr.state === "open" && pr.review === "review_required" ? chip("review required", TONE.none) : null,
];
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
const TUNNEL_URL = /^https:\/\/[^/]*(trycloudflare\.com|\.ts\.net|ngrok[-a-z]*\.(app|io|dev)|devtunnels\.ms)(:\d+)?(\/|$)/;
const consolidate = (g) => `The tunnel ${g.group} runs as a service of its own. Consolidate it as the wtc-svc skill says: remove it and open it again with wtc svc tunnel on the service it exposes.`;
const tunnelName = (t) => t.provider || (/trycloudflare\.com/.test(t.url ?? "") ? "cloudflare" : /\.ts\.net/.test(t.url ?? "") ? "tailscale" : "tunnel");
// Things only the conversation can do (resolve a conflict, publish where the
// gateway cannot) are asked of it, in the conversation, where it shows.
const askConversation = (host, sessionKey, message) => host.request("sessions.send", { key: sessionKey, message });
// Pushing goes to the repository's own branch and, without one yet, opens a
// draft pull request for it.
const pushTitle = (pr) => pr ? `Commit what is pending and push it to #${pr.number}` : "Commit what is pending, push the branch and open a draft pull request";
const pushRequest = (name, pr) => pr ? `Push ${name} to its pull request #${pr.number}.` : `Push ${name} to its branch and open a draft pull request.`;
async function restartService(host, sessionKey, agentId, s) {
  const res = await host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId: "svc-restart", sessionKey, agentId, payload: { group: s.group } });
  if (res?.ok === false) throw new Error(res.error ?? "restart failed");
}

// "wtc Collection", wtc linking to the project.
const brand = (size) => css(el("span", {},
  css(el("a", { textContent: "wtc", href: HOME, target: "_blank", rel: "noopener noreferrer", title: "wtc — worktree collections (opens on GitHub)" }),
    "color:var(--accent, inherit);text-decoration:none;font-weight:700"),
  el("span", { textContent: "Collection" })), `display:inline-flex;align-items:baseline;gap:.3em;flex:none;white-space:nowrap;font-weight:600;font-size:${size}`);
const dot = (tone) => css(el("span"), `flex:none;width:7px;height:7px;border-radius:50%;background:${tone}`);
const chip = (text, tone, title) => css(el("span", { title: title ?? "" }, dot(tone), text),
  "display:inline-flex;align-items:center;gap:5px;padding:0 8px;border-radius:10px;border:1px solid var(--border, rgba(0,0,0,.12));font-size:12px;line-height:20px;white-space:nowrap");

function mountComposer(container, context) {
  let ctx = context;
  let watch = null;
  let key = null;
  let entry = null;
  let note = "";
  const busy = new Set();

  const card = css(el("details", { className: "wtc-collection-card", open: remembered() }),
    "box-sizing:border-box;width:calc(100% - var(--chat-composer-side-inset, 36px));max-width:var(--chat-thread-max-width, 48rem);margin:8px auto 0;background:var(--card, #fff);color:var(--text, inherit);border:1px solid var(--border, rgba(0,0,0,.08));border-radius:25px;font:inherit;font-size:14px");
  const summary = css(el("summary"), "list-style:none;display:flex;align-items:center;gap:8px;padding:6px 8px 6px 13px;cursor:pointer");
  const detail = css(el("div"), "padding:2px 14px 12px 14px;display:grid;gap:6px");
  card.append(summary, detail);
  card.addEventListener("toggle", () => remember(card.open));
  const composer = el("div");
  container.append(composer);
  const unmountDefault = ctx.mountDefault(composer);

  const small = (text) => css(el("span", { textContent: text }), "color:var(--muted, #888);font-size:12px");
  const iconButton = (text, title, run) => {
    const b = css(el("button", { type: "button", textContent: text, title }),
      "border:0;background:transparent;color:var(--muted, #888);cursor:pointer;font:inherit;padding:2px 6px;border-radius:8px");
    b.onclick = (e) => { e.preventDefault(); e.stopPropagation(); run(); };
    return b;
  };
  const action = (text, title, run) => {
    const b = css(el("button", { type: "button", textContent: text, title }),
      "border:1px solid var(--border, rgba(0,0,0,.15));background:transparent;color:inherit;cursor:pointer;font:inherit;font-size:12px;padding:1px 10px;border-radius:10px");
    b.onclick = async () => {
      b.disabled = true;
      try { await run(); } catch (err) { say(String(err?.message ?? err)); } finally { b.disabled = false; }
    };
    return b;
  };
  const say = (text) => { note = text; if (entry) render(entry); };
  const pending = (r) => [r.dirty ? plural(r.dirty, "file") : "", r.unpublished ? plural(r.unpublished, "commit") : ""].filter(Boolean).join(", ");
  const openPRs = (repos) => repos.filter((r) => r.pullRequest?.state === "open");

  function chips(repos, services) {
    const out = [];
    const dirty = repos.filter((r) => r.dirty || r.unpublished);
    if (dirty.length) {
      const files = dirty.reduce((n, r) => n + (r.dirty ?? 0), 0);
      const commits = dirty.reduce((n, r) => n + (r.unpublished ?? 0), 0);
      const what = [files ? plural(files, "file") : "", commits ? plural(commits, "commit") : ""].filter(Boolean).join(", ");
      const where = dirty.length === 1 ? `${dirty[0].repo ?? dirty[0].project}: ` : `${dirty.length} repos: `;
      out.push(chip(`${where}${what} to push`, TONE.pending, dirty.map((r) => `${r.repo ?? r.project}: ${pending(r)}`).join("\n")));
    } else {
      out.push(chip("all pushed", TONE.none));
    }
    const behind = repos.filter((r) => r.behind > 0);
    if (behind.length) {
      out.push(chip(behind.length === 1 ? `${behind[0].repo ?? behind[0].project}: ${behind[0].behind} behind` : `${behind.length} repos behind`,
        TONE.pending, behind.map((r) => `${r.repo ?? r.project}: ${r.behind} behind ${r.base ?? "its base"}`).join("\n") + "\nExpand to catch up."));
    }
    for (const r of openPRs(repos)) {
      const pr = r.pullRequest;
      const state = prState(pr);
      out.push(chip(`${r.repo ?? r.project} #${pr.number}${pr.draft ? " draft" : ""}${pr.mergeable === "conflicting" ? " · conflict" : ""}`,
        prTone(pr), [pr.title ?? "", ...(state.length ? state : ["no checks"])].filter(Boolean).join("\n")));
    }
    const groups = serviceGroups(services);
    if (groups.length) {
      const up = groups.filter((g) => g.running).length;
      const failed = groups.some((g) => g.failed);
      out.push(chip(`${up}/${plural(groups.length, "service")} running`, failed ? TONE.failure : up ? TONE.success : TONE.none));
    }
    return out;
  }

  async function publish(r) {
    busy.add(r.sessionKey);
    watch?.pause(true);
    render(entry);
    try { await publishRepo(ctx.host, agentOf(ctx), r, say); } finally {
      busy.delete(r.sessionKey);
      watch?.pause(false);
      await watch?.refresh();
    }
  }

  function repoLine(r) {
    const name = r.repo ?? r.project ?? r.label;
    const what = pending(r);
    const pr = r.pullRequest;
    const base = r.base ? r.base.replace(/^origin\//, "") : "base";
    const parts = [
      repoTitle(r, "13px"),
      css(el("code", { textContent: r.branch ?? `at ${base}`, title: r.branch ? `Branch ${r.branch}, based on ${base}` : "No branch yet: the first push creates one" }), "font-size:12px;color:var(--muted, #888)"),
      small(r.error ?? `${r.ahead ?? 0} ahead`),
      r.behind ? chip(`${r.behind} behind`, TONE.pending, `${r.behind} new commits on ${r.base ?? "the base"}`) : null,
      what ? chip(`${what} to push`, TONE.pending) : null,
      pr ? css(el("span", {}, link(prLabel(pr), pr.url)), "white-space:nowrap;font-size:12px") : null,
      ...(pr ? prBadges(pr) : []),
    ];
    const actions = css(el("span"), "margin-left:auto;display:flex;gap:4px");
    if (busy.has(r.sessionKey)) actions.append(small("pushing…"));
    else {
      if (r.behind) actions.append(action("Catch up", "Merge the base branch into this worktree. Never rebases, never pushes.",
        async () => { await catchUpRepo(ctx.host, agentOf(ctx), r, say); await watch?.refresh(); }));
      if (pr?.state === "open" && pr.mergeable === "conflicting") actions.append(action("Resolve", "Ask the conversation to merge the base branch in and resolve the conflicts",
        () => askConversation(ctx.host, key, `Pull request #${pr.number} of ${name} conflicts with its base. Merge the base branch into ${name}, resolve the conflicts and commit.`)));
      if (what || (!pr && r.ahead > 0)) {
        actions.append(r.forge === "github"
          ? action(pr ? "Push" : "Push + PR", pushTitle(pr), () => publish(r))
          : action(pr ? "Push" : "Push + PR", pushTitle(pr), () => askConversation(ctx.host, key, pushRequest(name, pr))));
      }
    }
    return css(el("div", {}, ...parts, actions), "display:flex;align-items:center;gap:8px;flex-wrap:wrap");
  }

  function render(next) {
    entry = next;
    const repos = entry?.data?.repos ?? [];
    if (!ctx.presented || ctx.signal.aborted || repos.length === 0) {
      card.remove();
      return;
    }
    const services = entry.data.services ?? [];
    summary.replaceChildren(
      brand("14px"),
      css(el("span", {}, ...chips(repos, services)), "flex:1;min-width:0;display:flex;flex-wrap:wrap;gap:6px"),
      css(el("span", {}, small(`Updated ${ago(entry.at)}`),
        iconButton("↻", "Refresh", () => void watch?.refresh()),
        iconButton("⤢", "Open the wtc Collection panel", () => ctx.host.ui.openPanel("collection", { sessionKey: key, agentId: agentOf(ctx) })),
        css(el("span", { textContent: card.open ? "⌃" : "⌄" }), "color:var(--muted, #888);padding:0 6px")),
      "flex:none;margin-left:auto;display:inline-flex;align-items:center;gap:2px;white-space:nowrap"),
    );
    const lines = repos.map(repoLine);
    for (const g of serviceGroups(services)) {
      lines.push(css(el("div", {},
        dot(g.running ? TONE.success : g.failed ? TONE.failure : TONE.none),
        css(el("span", { textContent: g.label }), "font-weight:600"),
        small(`${g.repo ?? ""} ${g.state}`.trim()),
        g.loose ? chip("separate tunnel", TONE.pending, "Started apart from the service it exposes") : null,
        css(el("span", {},
          g.loose ? action("Consolidate", "Ask the conversation to open it with wtc svc tunnel instead", () => askConversation(ctx.host, key, consolidate(g))) : null,
          ...g.tunnels.filter((t) => t.url).map((t) => css(el("span", {}, link(tunnelName(t), t.url)), "font-size:12px")),
          g.url ? css(el("span", {}, link(g.url, g.url)), "font-size:12px") : null,
          g.failed || g.exited ? action("Restart", "wtc svc restart", async () => { await restartService(ctx.host, key, agentOf(ctx), g); await watch?.refresh(); }) : null),
        "margin-left:auto;display:flex;align-items:center;gap:10px")),
      "display:flex;align-items:center;gap:8px"));
    }
    if (note) lines.push(small(note));
    detail.replaceChildren(...lines);
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
  const busy = new Set();

  const root = css(el("section", { className: "wtc-collection" }),
    "position:relative;padding:14px 16px;font:inherit;font-size:14px;color:var(--text, inherit);overflow:auto;height:100%;box-sizing:border-box;display:grid;gap:14px;align-content:start");
  container.append(root);

  const muted = (text, extra = "") => css(el("span", { textContent: text }), `color:var(--muted, #888);font-size:12px;${extra}`);
  const mono = (text, title) => css(el("code", { textContent: text, title: title ?? text }),
    "font-family:var(--mono, ui-monospace, monospace);font-size:12px;color:var(--muted, #888);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0");
  const heading = (text, ...extra) => css(el("div", {}, css(el("span", { textContent: text }), "font-size:12px;font-weight:600;color:var(--muted, #888)"), ...extra),
    "display:flex;align-items:center;gap:8px;margin-bottom:6px");
  const card = (...kids) => css(el("div", {}, ...kids),
    "background:var(--card, transparent);border:1px solid var(--border, rgba(128,128,128,.25));border-radius:var(--radius-lg, 14px);padding:10px 12px;display:grid;gap:6px;min-width:0");
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

  async function publish(r) {
    busy.add(r.sessionKey);
    watch?.pause(true);
    render();
    try { await publishRepo(ctx.host, ctx.props.agentId, r, say); } finally {
      busy.delete(r.sessionKey);
      watch?.pause(false);
      await watch?.refresh();
    }
  }

  const sessionKeyOf = () => entry?.data?.collection?.sessionKey ?? ctx.props.sessionKey;

  async function catchUp(r) {
    await catchUpRepo(ctx.host, ctx.props.agentId, r, say);
    await watch?.refresh();
  }

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

  function repoCard(r, branch) {
    const name = r.repo ?? r.project ?? r.label;
    const what = [r.dirty ? plural(r.dirty, "file") : "", r.unpublished ? plural(r.unpublished, "commit") : ""].filter(Boolean).join(", ");
    const pr = r.pullRequest;
    const actions = css(el("span"), "margin-left:auto;display:flex;gap:6px");
    if (busy.has(r.sessionKey)) actions.append(muted("Pushing…"));
    else if (!r.error) {
      if (r.behind) actions.append(button("Catch up", "Merge the base branch into this worktree. Never rebases, never pushes.", () => catchUp(r)));
      if (pr?.state === "open" && pr.mergeable === "conflicting") actions.append(button("Resolve", "Ask the conversation to merge the base branch in and resolve the conflicts",
        () => askConversation(ctx.host, sessionKeyOf(), `Pull request #${pr.number} of ${name} conflicts with its base. Merge the base branch into ${name}, resolve the conflicts and commit.`)));
      if (what || (!pr && r.ahead > 0)) {
        actions.append(r.forge === "github"
          ? button(pr ? "Push" : "Push + PR", pushTitle(pr), () => publish(r), true)
          : button(pr ? "Push" : "Push + PR", pushTitle(pr), () => askConversation(ctx.host, sessionKeyOf(), pushRequest(name, pr)), true));
      }
    }
    const base = r.base ? r.base.replace(/^origin\//, "") : "base";
    const position = r.ahead ? `${r.ahead} ahead of ${base}` : r.behind ? `on ${base}` : `even with ${base}`;
    const lines = [
      row(repoTitle(r), actions),
      row(r.branch && r.branch !== branch ? mono(r.branch) : null, r.branch && r.branch !== branch ? muted("·") : null,
        muted(r.error ?? position), what || r.conflicted || r.behind ? null : muted("· clean")),
    ];
    if (what || r.conflicted || r.behind) {
      lines.push(row(
        r.behind ? chip(`${r.behind} behind ${base}`, TONE.pending, `${r.base} has ${plural(r.behind, "commit")} this worktree does not: Catch up merges them in`) : null,
        what ? chip(`${what} to push`, TONE.pending) : null,
        r.conflicted ? chip(`${r.conflicted} in conflict`, TONE.failure) : null));
    }
    if (pr) {
      const badges = prBadges(pr).filter(Boolean);
      const ci = badges.length ? badges : [muted("no checks")];
      lines.push(css(el("div", {},
        css(el("div", { textContent: pr.title ?? `Pull request #${pr.number}`, title: pr.title ?? "" }), "font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap"),
        row(css(el("span", {}, link(prLabel(pr), pr.url)), "font-size:12px;white-space:nowrap"), ...ci)),
      "display:grid;gap:4px;padding-top:8px;border-top:1px solid var(--border, rgba(128,128,128,.2))"));
    }
    return card(...lines);
  }

  // A service with its tunnels: the local address, then one link per tunnel.
  function serviceRow(g) {
    const tone = g.running ? TONE.success : g.failed ? TONE.failure : TONE.none;
    return css(el("div", {}, dot(tone),
      css(el("span", { textContent: g.label }), "font-weight:600"),
      muted([g.repo, g.state].filter(Boolean).join(" · ")),
      g.loose ? chip("separate tunnel", TONE.pending, "Started apart from the service it exposes") : null,
      css(el("span", {},
        g.loose ? button("Consolidate", "Ask the conversation to open it with wtc svc tunnel instead", () => askConversation(ctx.host, sessionKeyOf(), consolidate(g))) : null,
        g.url ? link(g.url.replace(/^https?:\/\//, ""), g.url) : null,
        ...g.tunnels.filter((t) => t.url).map((t) => el("span", { title: t.url }, link(tunnelName(t), t.url))),
        g.failed || g.exited ? button("Restart", "wtc svc restart", async () => { await restartService(ctx.host, sessionKeyOf(), ctx.props.agentId, g); await watch?.refresh(); }) : null),
      "margin-left:auto;font-size:12px;display:flex;align-items:center;gap:10px")),
    "display:flex;align-items:center;gap:8px;padding:8px 12px;flex-wrap:wrap");
  }

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

    const repoSection = el("div", {}, heading("Repositories"));
    const list = css(el("div"), "display:grid;gap:8px");
    if (repos.length === 0) list.append(muted("No repositories yet. Ask in the conversation, or add one below.", "font-size:13px"));
    list.append(...repos.map((r) => repoCard(r, branch)));
    list.append(adder);
    repoSection.append(list);

    const serviceSection = el("div", {}, heading("Services"));
    if (groups.length === 0) serviceSection.append(muted(data.servicesNote ?? "None running. Start them in the collection folder with wtc svc.", "font-size:13px"));
    else {
      const box = card();
      box.style.cssText += ";padding:0;gap:0";
      groups.forEach((g, i) => {
        const r = serviceRow(g);
        if (i) r.style.borderTop = "1px solid var(--border, rgba(128,128,128,.2))";
        box.append(r);
      });
      serviceSection.append(box);
    }

    const warnings = (data.warnings ?? []).map((w) => css(el("div", { textContent: w }), "font-size:12px;color:var(--warn, #bf8700)"));
    body.replaceChildren(repoSection, serviceSection, ...warnings);
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
