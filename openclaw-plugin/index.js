// wtc for OpenClaw: a collection is the conversation a task started in plus,
// per repository, a gateway-managed worktree held by an owner session (hidden
// unless someone wants a conversation per repository). The gateway knows the
// sessions; `wtc` knows the checkouts and services. This joins the two.
//
// Plain ESM, no build step and no SDK import. Needs `wtc` on the gateway's
// PATH, and `git`; pull requests also need `gh` with the gateway's login.
// Bitbucket repositories use the logins of the Bitbucket plugin
// (openclaw-bitbucket) when it is installed.
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";

const PLUGIN_ID = "wtc";

// The client each tunnel provider of wtc svc tunnel runs, and the providers
// whose tunnel anyone with the link can open.
const TUNNEL_PROGRAMS = { tailscale: "tailscale", devtunnel: "devtunnel", cloudflare: "cloudflared", ngrok: "ngrok" };
const PUBLIC_TUNNELS = new Set(["cloudflare", "ngrok"]);
const onPath = (program) => String(process.env.PATH ?? "").split(path.delimiter).some((dir) => dir && existsSync(path.join(dir, program)));

// A Bitbucket repository as a person types it: a bitbucket.org URL in https
// or scp form, or bitbucket.org/<workspace>/<repo>.
export function parseBitbucketRepo(spec) {
  const m = String(spec ?? "").trim().match(/^(?:https:\/\/(?:[^@/]+@)?|git@)?bitbucket\.org[/:]([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/);
  if (!m || [m[1], m[2]].some((part) => part === "." || part === "..")) return null;
  return { workspace: m[1], repo: m[2], cloneUrl: `https://bitbucket.org/${m[1]}/${m[2]}.git`, webUrl: `https://bitbucket.org/${m[1]}/${m[2]}` };
}

// The person a session belongs to, as the Bitbucket plugin decides it: the
// assigned human owner, otherwise the signed-in person who created it, for a
// spawned session that of its parent.
async function sessionOwnerProfileId(getSessionEntry, sessionKey, agentId) {
  let key = sessionKey;
  for (let depth = 0; key && depth <= 5; depth += 1) {
    const entry = await getSessionEntry({ sessionKey: key, ...(agentId ? { agentId } : {}) });
    if (!entry) return undefined;
    if (entry.owner?.actor?.type === "human") return entry.owner.actor.id || undefined;
    const creator = entry.createdActor;
    if (creator?.type === "human") return creator.source === "profile" ? creator.id || undefined : undefined;
    key = entry.parentSessionKey ?? entry.spawnedBy;
  }
  return undefined;
}

// That person's Bitbucket login, from the file the Bitbucket plugin keeps
// for its credential helper: e-mail and API token.
function bitbucketLogin(stateDir, profileId) {
  if (!profileId) return undefined;
  const digest = createHash("sha256").update(profileId).digest("hex").slice(0, 40);
  try {
    const login = {};
    for (const line of readFileSync(path.join(stateDir, "plugins", "bitbucket", "logins", `${digest}.login`), "utf8").split("\n")) {
      const at = line.indexOf("=");
      if (at > 0) login[line.slice(0, at)] = line.slice(at + 1);
    }
    return login.profile === profileId && login.user && login.token ? login : undefined;
  } catch {
    return undefined;
  }
}

// Bitbucket lists repositories per workspace only: the person's workspaces
// first, then the repositories they are a member of in each.
export async function bitbucketRepos(login, query) {
  const get = async (url) => {
    const response = await fetch(`https://api.bitbucket.org/2.0/${url}`, {
      headers: { accept: "application/json", authorization: `Basic ${Buffer.from(`${login.user}:${login.token}`).toString("base64")}` },
      redirect: "error",
      signal: AbortSignal.timeout(15000),
    });
    if (!response.ok) throw new Error(`Bitbucket answered ${response.status}`);
    return response.json();
  };
  const workspaces = ((await get("user/workspaces?pagelen=10")).values ?? []).map((w) => w.workspace?.slug).filter(Boolean);
  const params = new URLSearchParams({ role: "member", pagelen: "20", sort: "-updated_on" });
  const q = String(query ?? "").replace(/["\\]/g, "").trim();
  if (q) params.set("q", `full_name ~ "${q}"`);
  const pages = await Promise.all(workspaces.map((w) => get(`repositories/${encodeURIComponent(w)}?${params}`).catch(() => ({ values: [] }))));
  return pages.flatMap((page) => page.values ?? []).map((r) => ({
    forge: "bitbucket",
    name: r.name ?? r.slug,
    fullName: r.full_name,
    cloneUrl: `https://bitbucket.org/${r.full_name}.git`,
    webUrl: r.links?.html?.href ?? `https://bitbucket.org/${r.full_name}`,
    private: Boolean(r.is_private),
    description: r.description || "",
  })).slice(0, 20);
}

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

    const stateDir = api.runtime.state.resolveStateDir(process.env);
    const getSessionEntry = async (params) => api.runtime.agent.session.getSessionEntry(params);

    // Repositories from the accounts the person connected that the gateway
    // has no search for itself. GitHub's is the gateway's projects.searchRemote.
    api.session.controls.registerSessionAction({
      id: "repo-search",
      description: "Search the Bitbucket repositories of the person this session belongs to, with their Bitbucket login.",
      requiredScopes: ["operator.read"],
      async handler(action) {
        const login = bitbucketLogin(stateDir, await sessionOwnerProfileId(getSessionEntry, action.sessionKey, action.agentId));
        if (!login) return { ok: true, result: { bitbucket: { connected: false }, repos: [] } };
        try {
          return { ok: true, result: { bitbucket: { connected: true, user: login.user }, repos: await bitbucketRepos(login, action.payload?.query) } };
        } catch (err) {
          return { ok: true, result: { bitbucket: { connected: true, user: login.user, error: String(err?.message ?? err) }, repos: [] } };
        }
      },
    });

    // The branches of a repository and its default, for choosing the branch a
    // collection works against before adding it. Read with git, so GitHub
    // and Bitbucket behave the same, with the gateway's GitHub login and the
    // person's Bitbucket login for private repositories.
    api.session.controls.registerSessionAction({
      id: "repo-branches",
      description: "List the branches of a GitHub or Bitbucket repository, or of a project on the gateway.",
      requiredScopes: ["operator.read"],
      async handler(action) {
        let url = String(action.payload?.url ?? "");
        let cwd;
        const project = String(action.payload?.project ?? "");
        if (project) {
          if (!/^[A-Za-z0-9._-]+$/.test(project) || project === "." || project === "..") return { ok: false, error: "not a project name" };
          const dirs = [path.join(stateDir, "repos", project)];
          try {
            for (const hash of readdirSync(path.join(stateDir, "projects"))) dirs.unshift(path.join(stateDir, "projects", hash, project));
          } catch { /* no clones yet */ }
          cwd = dirs.find((d) => existsSync(path.join(d, ".git")));
          if (!cwd) return { ok: false, error: `${project} is not on the gateway` };
          url = "origin";
        } else if (!/^https:\/\/(github\.com|bitbucket\.org)\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+?(\.git)?$/.test(url)) {
          return { ok: false, error: "give a GitHub or Bitbucket https URL" };
        }
        const origin = cwd ? (await run(["git", "-C", cwd, "remote", "get-url", "origin"])).stdout.trim() : url;
        const env = { GIT_TERMINAL_PROMPT: "0" };
        let auth = [];
        if (/github\.com/.test(origin)) {
          auth = ["-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential"];
        } else if (/bitbucket\.org/.test(origin)) {
          const login = bitbucketLogin(stateDir, await sessionOwnerProfileId(getSessionEntry, action.sessionKey, action.agentId));
          if (login) {
            auth = ["-c", "credential.helper=", "-c", "credential.helper=!f() { [ \"$1\" = get ] && printf 'username=x-bitbucket-api-token-auth\\npassword=%s\\n' \"$WTC_BITBUCKET_TOKEN\"; }; f"];
            env.WTC_BITBUCKET_TOKEN = login.token;
          }
        }
        const r = await run(["git", ...(cwd ? ["-C", cwd] : []), ...auth, "ls-remote", "--symref", url, "HEAD", "refs/heads/*"], { timeoutMs: 30000, env });
        if (r.code !== 0) return { ok: false, error: `cannot read the branches: ${r.stderr.trim().split("\n").pop()}` };
        const lines = r.stdout.split("\n");
        const head = lines.map((l) => l.match(/^ref: refs\/heads\/(\S+)\s+HEAD$/)).find(Boolean)?.[1] ?? null;
        const branches = lines.map((l) => l.match(/^[0-9a-f]{40}\s+refs\/heads\/(\S+)$/)?.[1]).filter(Boolean);
        return { ok: true, result: { defaultBranch: head, branches } };
      },
    });

    // Clones a Bitbucket repository onto the gateway, with the person's login
    // when the repository is private. The gateway's own projects.add takes
    // GitHub only; the Control UI registers the clone with projects.register.
    api.session.controls.registerSessionAction({
      id: "clone-repo",
      description: "Clone a Bitbucket repository onto the gateway for this session's collection.",
      requiredScopes: ["operator.write"],
      async handler(action) {
        const repo = parseBitbucketRepo(action.payload?.url);
        if (!repo) return { ok: false, error: "Give a Bitbucket repository as bitbucket.org/<workspace>/<repo> or its URL." };
        const dir = path.join(stateDir, "repos", repo.repo);
        if (existsSync(dir)) {
          const origin = await run(["git", "-C", dir, "remote", "get-url", "origin"]);
          const known = parseBitbucketRepo(origin.stdout.trim());
          if (origin.code !== 0 || known?.workspace.toLowerCase() !== repo.workspace.toLowerCase() || known?.repo.toLowerCase() !== repo.repo.toLowerCase()) {
            return { ok: false, error: `${dir} already holds another repository.` };
          }
          return { ok: true, result: { path: dir, name: repo.repo, cloned: false } };
        }
        const login = bitbucketLogin(stateDir, await sessionOwnerProfileId(getSessionEntry, action.sessionKey, action.agentId));
        // The token reaches git through its environment, never a URL or a config file.
        const auth = login ? ["-c", "credential.helper=", "-c", "credential.helper=!f() { [ \"$1\" = get ] && printf 'username=x-bitbucket-api-token-auth\\npassword=%s\\n' \"$WTC_BITBUCKET_TOKEN\"; }; f"] : [];
        const r = await run(["git", ...auth, "clone", "--quiet", repo.cloneUrl, dir], {
          timeoutMs: 300000,
          env: { GIT_TERMINAL_PROMPT: "0", ...(login ? { WTC_BITBUCKET_TOKEN: login.token } : {}) },
        });
        if (r.code !== 0) {
          return { ok: false, error: `git clone ${repo.cloneUrl} failed${login ? "" : " (no Bitbucket login: connect one on the Bitbucket tab for private repositories)"}: ${r.stderr.trim().split("\n").pop()}` };
        }
        return { ok: true, result: { path: dir, name: repo.repo, cloned: true } };
      },
    });

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
        // The tunnel providers whose client the gateway has, and whether its
        // tailnet is the team's, so that a Tailscale tunnel is worth offering.
        result.tunnelProviders = Object.keys(TUNNEL_PROGRAMS).filter((p) => onPath(TUNNEL_PROGRAMS[p]));
        result.teamTailnet = api.pluginConfig?.teamTailnet === true;
        // Whether a portal gets an address beyond the gateway's own: a
        // wildcard domain or Tailscale run by the gateway.
        const gateway = (api.runtime?.config?.current?.() ?? api.config)?.gateway;
        result.portalIngress = !!gateway?.portals?.ingress?.domain || ["serve", "funnel"].includes(gateway?.tailscale?.mode);
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

    // Starts, stops or restarts the collection's services (one, or all without
    // a group) and opens or closes a service's tunnel, from its folder.
    const svcAction = async (action, op) => {
      const payload = action.payload ?? {};
      const group = String(payload.group ?? "");
      const provider = String(payload.provider ?? "");
      if (!["up", "down", "restart", "tunnel"].includes(op)) return { ok: false, error: "op is up, down, restart or tunnel" };
      if (group && !/^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/.test(group)) return { ok: false, error: "name a service as <repo>/<service>" };
      if (op === "tunnel" && !group) return { ok: false, error: "name the service to tunnel to" };
      if (op === "tunnel" && !TUNNEL_PROGRAMS[provider]) return { ok: false, error: `the provider is one of ${Object.keys(TUNNEL_PROGRAMS).join(", ")}` };
      if (!action.sessionKey || !action.agentId) return { ok: false, error: "sessionKey and agentId are required" };
      const { children } = collectionOf(action.agentId, action.sessionKey);
      const name = children.map((c) => String(c.entry.worktree?.branch ?? "").replace(/^openclaw\//, "")).find(Boolean);
      if (!name) return { ok: false, error: "this session has no collection" };
      const argv = ["wtc", "svc", op, ...(group ? [group] : [])];
      if (op === "tunnel") {
        argv.push(provider);
        if (payload.stop) argv.push("--stop");
        // Only a person's click opens a tunnel anyone with the link can use.
        else if (payload.public && PUBLIC_TUNNELS.has(provider)) argv.push("--public");
        // A new tunnel runs the provider's client from the PATH; without it
        // the conversation can install one first.
        if (payload.add && !payload.stop && !onPath(TUNNEL_PROGRAMS[provider])) return { ok: true, result: { missing: TUNNEL_PROGRAMS[provider] } };
      }
      const r = await run(argv, { cwd: path.join(stateDir, "wtc-collections", name), timeoutMs: 180000, env: { OPENCLAW_SHELL: "exec" } });
      return r.code === 0 ? { ok: true, result: { message: `${argv.slice(2).join(" ")}: done` } } : { ok: false, error: (r.stderr || r.stdout).trim().slice(-400) || `exit ${r.code}` };
    };
    api.session.controls.registerSessionAction({
      id: "svc",
      description: "Start, stop or restart this session's collection's services, all or one, or open or close a service's tunnel (wtc svc up|down|restart|tunnel).",
      requiredScopes: ["operator.write"],
      handler: (action) => svcAction(action, String(action.payload?.op ?? "")),
    });
    // The earlier name, for a Control UI still loaded from before.
    api.session.controls.registerSessionAction({
      id: "svc-restart",
      description: "Restart one service of this session's collection (wtc svc restart).",
      requiredScopes: ["operator.write"],
      handler: (action) => svcAction(action, "restart"),
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
