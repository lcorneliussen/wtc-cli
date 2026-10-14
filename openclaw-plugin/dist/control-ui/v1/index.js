// The Collection panel: one row per repository of the session's worktree
// collection. Plain ESM, no build step and no SDK import.
const PLUGIN_ID = "wtc";

const el = (tag, props = {}, ...kids) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...kids.filter((k) => k != null));
  return node;
};

async function call(host, actionId, sessionKey, agentId) {
  const res = await host.request("plugins.sessionAction", { pluginId: PLUGIN_ID, actionId, sessionKey, ...(agentId ? { agentId } : {}) });
  if (res && res.ok === false) throw new Error(res.error ?? "action failed");
  return res?.result ?? res;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;

function mountCollection(container, context) {
  let ctx = context;
  const root = el("section", { className: "wtc-collection" });
  root.style.cssText = "padding:12px;font:13px/1.4 system-ui;overflow:auto;height:100%;box-sizing:border-box";
  const title = el("h3", { textContent: "Collection" });
  title.style.margin = "0 0 4px";
  const sub = el("div");
  sub.style.opacity = "0.7";
  const refresh = el("button", { textContent: "Refresh", type: "button" });
  refresh.style.marginTop = "8px";
  const body = el("div");
  body.style.marginTop = "10px";
  const log = el("p");
  log.style.cssText = "font-size:12px;opacity:.8";
  root.append(title, sub, refresh, body, log);
  container.append(root);

  const cell = (tag, ...kids) => {
    const c = el(tag, {}, ...kids);
    c.style.cssText = "text-align:left;padding:4px 10px 4px 0;border-bottom:1px solid rgba(128,128,128,.3);vertical-align:top";
    return c;
  };

  const pullRequest = (r) => {
    const p = r.pullRequest;
    if (!p) return r.published ? "published" : "—";
    const label = `#${p.number} ${p.draft && p.state === "open" ? "draft" : p.state}`;
    // Only a web address becomes a link.
    return /^https:\/\//.test(p.url ?? "") ? el("a", { textContent: label, href: p.url, target: "_blank", rel: "noopener noreferrer" }) : label;
  };

  const row = (r, requested) => {
    const tr = el("tr");
    if (r.sessionKey === requested) tr.style.fontWeight = "600";
    const open = el("a", { textContent: r.project ?? r.repo ?? r.label ?? r.sessionKey, href: "#" });
    open.onclick = (e) => {
      e.preventDefault();
      ctx.host.sessions.open({ sessionKey: r.sessionKey, agentId: ctx.props.agentId });
    };
    const catchUp = el("button", { textContent: "Catch up", type: "button", title: "Merge the destination branch into this worktree. Never rebases, never pushes." });
    catchUp.onclick = async () => {
      catchUp.disabled = true;
      log.textContent = `Catching up ${r.project ?? r.repo}…`;
      try {
        const out = await call(ctx.host, "catch-up", r.sessionKey, ctx.props.agentId);
        log.textContent = `${r.project ?? r.repo}: ${out.json?.summary ?? (out.message || "exit " + out.code)}`;
        await load();
      } catch (err) {
        log.textContent = String(err?.message ?? err);
      } finally {
        catchUp.disabled = false;
      }
    };
    if (r.error) {
      tr.append(cell("td", open), cell("td", r.branch ?? "?"), cell("td", "?"), cell("td", r.error), cell("td", "—"), cell("td"));
      return tr;
    }
    const base = `${r.ahead} ahead${r.behind ? `, ${r.behind} behind` : ""}${r.base ? ` of ${r.base}` : ""}`;
    const pending = [r.dirty ? plural(r.dirty, "file") : "", r.unpublished ? plural(r.unpublished, "commit") : ""].filter(Boolean).join(", ");
    tr.append(cell("td", open), cell("td", r.branch ?? "?"), cell("td", base),
      cell("td", pending ? pending + (r.conflicted ? ` (${r.conflicted} in conflict)` : "") : "nothing"),
      cell("td", pullRequest(r)), cell("td", catchUp));
    return tr;
  };

  const render = (data) => {
    const count = data.repos.length === 1 ? "1 repository" : `${data.repos.length} repositories`;
    sub.textContent = `${data.collection.label ?? data.collection.sessionKey} — ${count}`;
    if (data.repos.length === 0) {
      body.replaceChildren(el("p", { textContent: "This session has no repositories yet. A collection is this session plus one child session per repository; add one with wtc add-repo." }));
    } else {
      const table = el("table");
      table.style.cssText = "border-collapse:collapse;width:100%";
      table.append(el("tr", {}, ...["Repository", "Branch", "Against base", "To publish", "Pull request", ""].map((h) => cell("th", h))));
      for (const r of data.repos) table.append(row(r, data.requestedSessionKey));
      body.replaceChildren(table);
    }
    for (const warning of data.warnings ?? []) body.append(el("p", { textContent: warning }));
  };

  async function load() {
    const { sessionKey, agentId } = ctx.props ?? {};
    if (!sessionKey) {
      body.textContent = "No session selected.";
      return;
    }
    try {
      const data = await call(ctx.host, "collection", sessionKey, agentId);
      if (!ctx.signal.aborted) render(data);
    } catch (err) {
      if (!ctx.signal.aborted) body.textContent = `The collection could not be loaded: ${String(err?.message ?? err)}`;
    }
  }

  refresh.onclick = () => void load();
  void load();
  return {
    update(next) {
      const changed = next.props?.sessionKey !== ctx.props?.sessionKey;
      ctx = next;
      if (changed) void load();
    },
    dispose() {
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
