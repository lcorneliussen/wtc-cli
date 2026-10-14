// wtc for OpenClaw: a collection is the conversation a task started in plus,
// per repository, a gateway-managed worktree held by an owner session (hidden
// unless someone wants a conversation per repository). The gateway knows the
// sessions; `wtc` knows the checkouts and services. This joins the two.
//
// Plain ESM, no build step and no SDK import. Needs `wtc` on the gateway's
// PATH, and `git`; pull requests also need `gh` with the gateway's login.
const PLUGIN_ID = "wtc";

// A child that owns a repository has a managed worktree to look at.
const worktreeOf = (entry) => (entry?.worktree ? entry.workspaceDir || entry.spawnedCwd || null : null);

export default {
  id: PLUGIN_ID,
  name: "Worktree Collection",
  register(api) {
    const run = async (argv, options = {}) => {
      const r = await api.runtime.system.runCommandWithTimeout(argv, { timeoutMs: 60000, killProcessTree: true, ...options });
      return { code: r.code ?? r.exitCode ?? null, stdout: String(r.stdout ?? ""), stderr: String(r.stderr ?? "") };
    };

    // The collection around a session: itself when it has repository
    // children, otherwise its parent.
    const collectionOf = (agentId, sessionKey) => {
      const all = [...api.runtime.agent.session.listSessionEntries({ agentId })];
      const entryOf = new Map(all.map((s) => [s.sessionKey, s.entry]));
      const childrenOf = (key) => all.filter((s) => s.entry?.parentSessionKey === key && worktreeOf(s.entry));
      let parentKey = sessionKey;
      let children = childrenOf(parentKey);
      const parentOfSelf = entryOf.get(sessionKey)?.parentSessionKey;
      if (children.length === 0 && parentOfSelf) {
        parentKey = parentOfSelf;
        children = childrenOf(parentKey);
      }
      return { parentKey, parent: entryOf.get(parentKey), children };
    };

    api.session.controls.registerSessionAction({
      id: "collection",
      description: "The repositories of this session's worktree collection: branch, pending changes and pull request of each.",
      requiredScopes: ["operator.read"],
      async handler(action) {
        if (!action.sessionKey || !action.agentId) return { ok: false, error: "sessionKey and agentId are required" };
        const { parentKey, parent, children } = collectionOf(action.agentId, action.sessionKey);
        const result = {
          schema: "wtc.collection/v1",
          collection: { sessionKey: parentKey, label: parent?.label ?? parent?.displayName ?? null },
          requestedSessionKey: action.sessionKey,
          repos: [],
          warnings: [],
        };
        if (children.length === 0) return { ok: true, result };
        // The collection's name is its worktrees' name. wtc status reads
        // them, keeps the collection's folder and reports its services.
        const names = [...new Set(children.map((c) => String(c.entry.worktree?.branch ?? "").replace(/^openclaw\//, "")).filter(Boolean))];
        let checkouts = [];
        result.services = [];
        for (const name of names) {
          const status = await run(["wtc", "status", name, "--json"], { env: { OPENCLAW_SHELL: "exec" } });
          try {
            const col = JSON.parse(status.stdout)[name] ?? {};
            checkouts.push(...(col.repos ?? []));
            result.services.push(...(col.services ?? []));
            if (col.folder) result.collection.folder = col.folder;
            if (col.error) result.warnings.push(`collection folder: ${col.error}`);
          } catch {
            result.warnings.push(`wtc status failed (exit ${status.code}): ${(status.stderr || status.stdout).trim().slice(0, 300) || "is wtc on the gateway's PATH?"}`);
          }
        }
        const byPath = new Map(checkouts.map((c) => [c.path, c]));
        result.repos = children
          .map(({ sessionKey, entry }) => ({
            sessionKey,
            label: entry.label ?? entry.displayName ?? null,
            project: entry.projectId ?? null,
            // A hidden owner has no conversation of its own.
            owner: sessionKey.includes(":subagent:") ? "hidden" : "conversation",
            ...(byPath.get(worktreeOf(entry)) ?? { path: worktreeOf(entry), branch: entry.worktree?.branch ?? null, error: "no status" }),
          }))
          .sort((a, b) => String(a.project ?? a.repo).localeCompare(String(b.project ?? b.repo)));
        return { ok: true, result };
      },
    });

    api.session.controls.registerSessionAction({
      id: "catch-up",
      description: "Merge the destination branch into this session's worktree (wtc catch-up). Never rebases, never pushes.",
      requiredScopes: ["operator.write"],
      async handler(action) {
        const entry = action.sessionKey && api.runtime.agent.session.getSessionEntry({ agentId: action.agentId, sessionKey: action.sessionKey });
        const cwd = worktreeOf(entry);
        if (!cwd) return { ok: false, error: "this session owns no managed worktree" };
        // wtc reads its host from the environment an agent's shell has.
        const r = await run(["wtc", "catch-up", "--json"], { cwd, timeoutMs: 120000, env: { OPENCLAW_SHELL: "exec" } });
        let json = null;
        try { json = JSON.parse(r.stdout); } catch { /* reported as text below */ }
        return { ok: true, result: { code: r.code, json, message: (json ? "" : r.stdout) + r.stderr } };
      },
    });
  },
};
