// The Collection panel: everything a collection's session works with — its
// repositories, their pull requests and the services running for it — in one
// place. Plain ESM, no build step and no SDK import.
//
// A collection is driven from one conversation. Each repository's worktree is
// held by an owner session the gateway publishes for, usually a hidden one;
// this panel publishes through that owner, the way the chat's own Publish PR
// button does for a session's single checkout.
const PLUGIN_ID = "wtc";

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

const COLORS = { success: "#2da44e", failure: "#cf222e", pending: "#bf8700", none: "rgba(128,128,128,.8)" };
const pill = (text, color, title) => css(el("span", { textContent: text, title: title ?? "" }),
  `display:inline-block;padding:0 6px;border-radius:9px;font-size:11px;line-height:17px;color:#fff;background:${color};white-space:nowrap`);
const ciPill = (pr) => {
  if (!pr || pr.checks === "none" || !pr.checks) return null;
  const label = { success: "CI passed", failure: `CI ${pr.failed} failed`, pending: `CI ${pr.running} running` }[pr.checks];
  return pill(label, COLORS[pr.checks], `${pr.passed} passed, ${pr.failed} failed, ${pr.running} running`);
};
const prLabel = (pr) => `#${pr.number} ${pr.draft && pr.state === "open" ? "draft" : pr.state}`;

function mountCollection(container, context) {
  let ctx = context;
  let data = null;
  const busy = new Set();

  const root = css(el("section", { className: "wtc-collection" }), "padding:12px;font:13px/1.45 system-ui;overflow:auto;height:100%;box-sizing:border-box");
  const title = css(el("h3", { textContent: "Collection" }), "margin:0");
  const sub = css(el("div"), "opacity:.7;margin-bottom:8px");
  const refresh = el("button", { textContent: "Refresh", type: "button" });
  const body = el("div");
  const log = css(el("p"), "font-size:12px;opacity:.85;min-height:1em;margin:6px 0");
  root.append(css(el("div", {}, title, refresh), "display:flex;gap:8px;align-items:center;justify-content:space-between"), sub, log, body);
  container.append(root);

  const say = (text) => (log.textContent = text);
  const heading = (text) => css(el("h4", { textContent: text }), "margin:14px 0 6px;font-size:12px;text-transform:uppercase;letter-spacing:.04em;opacity:.7");
  const cell = (tag, ...kids) => css(el(tag, {}, ...kids), "text-align:left;padding:4px 10px 4px 0;border-bottom:1px solid rgba(128,128,128,.25);vertical-align:top");
  const table = (headers, rows) => {
    const t = css(el("table"), "border-collapse:collapse;width:100%");
    t.append(el("tr", {}, ...headers.map((h) => cell("th", h))), ...rows);
    return t;
  };
  const button = (text, title, run) => {
    const b = el("button", { textContent: text, type: "button", title });
    b.onclick = async () => {
      b.disabled = true;
      try { await run(); } catch (err) { say(String(err?.message ?? err)); } finally { b.disabled = false; }
    };
    return b;
  };

  // Publishes the owner's worktree: commits what is pending, pushes the
  // branch and opens or updates a draft pull request.
  async function publish(r) {
    const name = r.repo ?? r.project;
    busy.add(r.sessionKey);
    say(`Publishing ${name}…`);
    try {
      const out = await ctx.host.request("sessions.github.publish", {
        sessionKey: r.sessionKey, agentId: ctx.props.agentId, idempotencyKey: uuid(), selection: { source: "shared" },
      });
      if (out?.status === "published") say(`${name}: published ${out.url ?? ""}`);
      else if (out?.status === "needs_confirmation") say(`${name}: the gateway asks for confirmation; open Publish in the owner session.`);
      else say(`${name}: ${out?.status ?? "done"}${out?.error ? ` — ${out.error.message ?? out.error}` : ""}`);
    } finally {
      busy.delete(r.sessionKey);
      await load();
    }
  }

  async function catchUp(r) {
    const name = r.repo ?? r.project;
    say(`Catching up ${name}…`);
    const out = await call(ctx.host, "catch-up", r.sessionKey, ctx.props.agentId);
    say(`${name}: ${out.json?.summary ?? (out.message || "exit " + out.code)}`);
    await load();
  }

  // Adds a GitHub repository to the gateway and asks the collection's
  // session to take it into the collection.
  async function addRepository(spec) {
    const m = spec.trim().match(/^(?:https:\/\/github\.com\/|git@github\.com:)?([A-Za-z0-9][A-Za-z0-9-]*)\/([A-Za-z0-9._-]+?)(?:\.git)?\/?$/);
    if (!m) throw new Error("Give a GitHub repository as owner/name or its URL.");
    const [, owner, repo] = m;
    say(`Adding ${owner}/${repo} to the gateway…`);
    await ctx.host.request("projects.add", { gitUrl: `https://github.com/${owner}/${repo}.git` }, { timeoutMs: null });
    await ctx.host.request("sessions.send", {
      key: data?.collection?.sessionKey ?? ctx.props.sessionKey,
      message: `Add ${owner}/${repo} to this collection.`,
    });
    say(`${owner}/${repo} is on the gateway; the collection's session is adding it.`);
  }

  function repoRows(repos) {
    return repos.map((r) => {
      const name = el("span", { textContent: r.repo ?? r.project ?? r.label, title: r.path ?? "" });
      if (r.error) return el("tr", {}, cell("td", name), cell("td", r.branch ?? "?"), cell("td", "?"), cell("td", r.error), cell("td", "—"), cell("td"));
      const base = `${r.ahead} ahead${r.behind ? `, ${r.behind} behind` : ""}`;
      const pending = [r.dirty ? plural(r.dirty, "file") : "", r.unpublished ? plural(r.unpublished, "commit") : ""].filter(Boolean).join(", ");
      const pr = r.pullRequest;
      const prCell = pr ? css(el("span", {}, link(prLabel(pr), pr.url), " ", ciPill(pr)), "white-space:nowrap") : (r.published ? "published" : "—");
      const actions = css(el("span"), "display:flex;gap:4px;flex-wrap:wrap");
      if (r.behind) actions.append(button("Catch up", "Merge the base branch into this worktree. Never rebases, never pushes.", () => catchUp(r)));
      if (r.forge === "github" && (pending || !pr)) {
        actions.append(button(pr ? "Update PR" : "Publish", "Commit what is pending, push, and open or update a draft pull request.", () => publish(r)));
      }
      if (busy.has(r.sessionKey)) actions.replaceChildren("publishing…");
      return el("tr", {}, cell("td", name), cell("td", css(el("code", { textContent: r.branch ?? "?" }), "font-size:12px")),
        cell("td", r.base ? `${base} of ${r.base}` : base), cell("td", pending ? pending + (r.conflicted ? ` (${r.conflicted} in conflict)` : "") : "nothing"),
        cell("td", prCell), cell("td", actions));
    });
  }

  function render() {
    const repos = data.repos ?? [];
    sub.textContent = `${data.collection.label ?? data.collection.sessionKey} — ${repos.length === 1 ? "1 repository" : `${repos.length} repositories`}`;
    const parts = [];
    if (data.collection.folder) {
      parts.push(css(el("div", {}, "Folder ", css(el("code", { textContent: data.collection.folder, title: "One link per repository to its worktree" }), "font-size:12px")), "opacity:.75"));
    }

    parts.push(heading("Repositories"));
    if (repos.length === 0) {
      parts.push(el("p", { textContent: "No repositories yet. Ask in this conversation (“add owner/name”), or add one below." }));
    } else {
      parts.push(table(["Repository", "Branch", "Against base", "To publish", "Pull request", ""], repoRows(repos)));
    }
    const input = css(el("input", { placeholder: "owner/name", type: "text" }), "min-width:14em");
    const add = button("Add repository", "Clone a GitHub repository on the gateway and add it to this collection", async () => {
      await addRepository(input.value);
      input.value = "";
    });
    input.onkeydown = (e) => { if (e.key === "Enter") add.click(); };
    parts.push(css(el("div", {}, input, add), "display:flex;gap:6px;margin-top:8px"));

    const prs = repos.filter((r) => r.pullRequest).map((r) => ({ repo: r.repo ?? r.project, ...r.pullRequest }));
    parts.push(heading("Pull requests"));
    if (prs.length === 0) parts.push(el("p", { textContent: "None yet." }));
    else {
      parts.push(table(["", "Repository", "Title", "CI"], prs.map((p) => el("tr", {},
        cell("td", link(prLabel(p), p.url)), cell("td", p.repo), cell("td", p.title ?? ""),
        cell("td", ciPill(p) ?? "no checks", p.checks && p.checks !== "none" ? ` ${p.passed}✓ ${p.failed}✗ ${p.running}…` : "")))));
    }

    const services = data.services ?? [];
    parts.push(heading("Services"));
    if (services.length === 0) {
      parts.push(el("p", { textContent: data.servicesNote ?? "No services running for this collection." }));
    } else {
      const state = (s) => pill(s.state ?? "?", s.state === "ready" || s.state === "running" ? COLORS.success : s.state === "failed" ? COLORS.failure : COLORS.none);
      parts.push(table(["Service", "Repository", "State", "Address"], services.map((s) => el("tr", {},
        cell("td", s.label || s.path || s.group), cell("td", s.repo ?? ""), cell("td", state(s)), cell("td", link(s.url ?? "", s.url))))));
    }

    for (const warning of data.warnings ?? []) parts.push(css(el("p", { textContent: warning }), "color:#bf8700"));
    body.replaceChildren(...parts);
  }

  async function load() {
    const { sessionKey, agentId } = ctx.props ?? {};
    if (!sessionKey) {
      body.textContent = "No session selected.";
      return;
    }
    try {
      const next = await call(ctx.host, "collection", sessionKey, agentId);
      if (!ctx.signal.aborted) {
        data = next;
        render();
      }
    } catch (err) {
      if (!ctx.signal.aborted) body.textContent = `The collection could not be loaded: ${String(err?.message ?? err)}`;
    }
  }

  refresh.onclick = () => void load();
  void load();
  // Pull requests and CI move on their own; look again while the panel is open.
  const timer = setInterval(() => { if (!document.hidden && busy.size === 0) void load(); }, 30000);
  return {
    update(next) {
      const changed = next.props?.sessionKey !== ctx.props?.sessionKey;
      ctx = next;
      if (changed) void load();
    },
    dispose() {
      clearInterval(timer);
      root.remove();
    },
  };
}

export default {
  id: PLUGIN_ID,
  activate(host) {
    const disposers = [
      host.ui.registerPanel({ id: "collection", label: "Collection", icon: "folder", mount: mountCollection }),
      // "header": a button in the chat header. "session": an entry in the
      // session's menu in the sidebar.
      ...["header", "session"].map((placement) =>
        host.ui.registerAction({
          id: `open-collection-${placement}`,
          label: "Collection",
          placement,
          run: ({ sessionKey, agentId }) => host.ui.openPanel("collection", { sessionKey, agentId }),
        })),
    ];
    return () => disposers.forEach((d) => (typeof d === "function" ? d() : d?.dispose?.()));
  },
};
