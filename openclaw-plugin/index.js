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
import { existsSync, mkdirSync, readFileSync, readdirSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import { startSandboxGit } from "./sandbox-git.js";

const PLUGIN_ID = "wtc";

// The client each tunnel provider of wtc svc tunnel runs, and the providers
// whose tunnel anyone with the link can open.
const TUNNEL_PROGRAMS = { tailscale: "tailscale", devtunnel: "devtunnel", cloudflare: "cloudflared", ngrok: "ngrok" };
const PUBLIC_TUNNELS = new Set(["cloudflare", "ngrok"]);
// The wtc this plugin came from: wtc openclaw install stamps its version.
const PLUGIN_VERSION = (() => {
  try {
    return JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf8")).version ?? null;
  } catch {
    return null;
  }
})();

// The version wtc --version names, or null.
export const wtcVersionOf = (output) => /^wtc version (\S+)/.exec(String(output ?? "").trim())?.[1] ?? null;

// What to say when the wtc on the gateway's PATH is a release other than the
// one this plugin came from, as after upgrading wtc without reinstalling the
// plugin. A dev build on either side says nothing: a wtc without a release
// version stamps the plugin with a -dev version (describeBuild in cmd/wtc),
// or 0.0.0-dev.
export function versionWarning(pluginVersion, wtcVersionOutput) {
  const m = /^wtc version v?(\d+\.\d+\.\d+)$/.exec(String(wtcVersionOutput ?? "").trim());
  if (!m || !/^\d+\.\d+\.\d+$/.test(String(pluginVersion ?? "")) || m[1] === pluginVersion) return null;
  return `This card is the wtc ${pluginVersion} plugin, but the gateway runs wtc ${m[1]}: run "wtc openclaw install" on the gateway host.`;
}

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

// The folder of the collection called name, as wtc makes it: wtc/<harness
// or ad-hoc>/<name>, or wtc-collections/<name> from before there was one per
// harness.
export function collectionFolder(stateDir, name) {
  const legacy = path.join(stateDir, "wtc-collections", name);
  if (existsSync(legacy)) return legacy;
  let groups = [];
  try {
    groups = readdirSync(path.join(stateDir, "wtc")).sort();
  } catch {}
  for (const group of groups) {
    const dir = path.join(stateDir, "wtc", group, name);
    if (existsSync(dir)) return dir;
  }
  return null;
}

// What the conversation is told about a pull request: the facts of it that
// change, as wtc status reports them.
const prFacts = (pr) => (pr ? {
  number: pr.number, url: pr.url, state: pr.state, draft: !!pr.draft, checks: pr.checks ?? "none",
  failed: pr.failed ?? 0, review: pr.review ?? "", mergeable: pr.mergeable ?? "",
} : null);

// What changed on a repository's pull request between two looks, one line
// each. prev is undefined before the first look, which has nothing to compare
// with, and null while there was no pull request.
export function pullRequestChanges(name, prev, cur) {
  if (prev === undefined || !cur) return [];
  const pr = `#${cur.number} (${cur.url})`;
  if (!prev || prev.number !== cur.number) {
    const what = cur.state === "merged" ? "was opened and merged" : cur.state === "closed" ? "was opened and closed" : cur.draft ? "opened as a draft" : "opened";
    return [`${name}: pull request ${pr} ${what}.`];
  }
  const lines = [];
  if (prev.state !== cur.state) {
    if (cur.state === "merged") lines.push(`${name}: pull request ${pr} was merged.`);
    else if (cur.state === "closed") lines.push(`${name}: pull request ${pr} was closed without merging.`);
    else if (cur.state === "open") lines.push(`${name}: pull request ${pr} was reopened.`);
  }
  if (cur.state !== "open") return lines;
  if (prev.draft !== cur.draft) lines.push(`${name}: pull request ${pr} ${cur.draft ? "went back to draft" : "is ready for review"}.`);
  if (prev.checks !== cur.checks) {
    if (cur.checks === "failure") lines.push(`${name}: checks failed on ${pr}${cur.failed ? ` (${cur.failed} failed)` : ""}.`);
    else if (cur.checks === "success") lines.push(`${name}: checks passed on ${pr}${prev.checks === "failure" ? " again" : ""}.`);
  }
  if (prev.review !== cur.review) {
    if (cur.review === "approved") lines.push(`${name}: pull request ${pr} was approved.`);
    else if (cur.review === "changes_requested") lines.push(`${name}: a review requested changes on ${pr}.`);
  }
  if (prev.mergeable !== cur.mergeable && cur.mergeable === "conflicting") lines.push(`${name}: pull request ${pr} conflicts with its base.`);
  return lines;
}

// The pull requests seen in each collection, by agent and the collection's
// session, kept in the gateway's state so that a restart does not tell the
// same news again. A collection not looked at for a while is forgotten.
const FORGET_MS = 14 * 24 * 3600 * 1000;
export function pullRequestWatch(file, now = Date.now) {
  let seen = {};
  try {
    seen = JSON.parse(readFileSync(file, "utf8")) ?? {};
  } catch {}
  const save = () => {
    for (const [key, c] of Object.entries(seen)) if (now() - (c.at ?? 0) > FORGET_MS) delete seen[key];
    try {
      mkdirSync(path.dirname(file), { recursive: true });
      writeFileSync(`${file}.tmp`, JSON.stringify(seen));
      renameSync(`${file}.tmp`, file);
    } catch {}
  };
  return {
    // Records a look at a collection's repositories; returns what changed.
    observe(agentId, sessionKey, repos) {
      const key = `${agentId} ${sessionKey}`;
      const c = (seen[key] ??= { agentId, sessionKey, repos: {} });
      c.at = now();
      const lines = [];
      for (const r of repos) {
        if (!r.path || r.error) continue;
        const prev = c.repos[r.path];
        const cur = prFacts(r.pullRequest);
        // A pull request does not go away: none after one was seen means
        // the forge could not be asked this time. (wtc status reports both
        // that and no pull request yet as none.)
        if (!cur && prev) continue;
        lines.push(...pullRequestChanges(r.repo ?? r.project ?? path.basename(r.path), prev, cur));
        c.repos[r.path] = cur;
      }
      save();
      return lines;
    },
    // The collections with a pull request still open, to look at again.
    open() {
      return Object.values(seen).filter((c) => Object.values(c.repos).some((pr) => pr?.state === "open"));
    },
    forget(agentId, sessionKey) {
      delete seen[`${agentId} ${sessionKey}`];
      save();
    },
  };
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
    // children; otherwise its parent's when it is a repository's own session
    // (it has a worktree). A new conversation has a parent too (the
    // dashboard's, often the main session) and no repositories yet: it is a
    // collection of its own, still empty.
    const collectionOf = (agentId, sessionKey) => {
      const all = [...api.runtime.agent.session.listSessionEntries({ agentId })];
      const entryOf = new Map(all.map((s) => [s.sessionKey, s.entry]));
      const childrenOf = (key) => all.filter((s) => s.entry?.parentSessionKey === key && worktreeOf(s.entry));
      let parentKey = sessionKey;
      let children = childrenOf(parentKey);
      const parentOfSelf = entryOf.get(sessionKey)?.parentSessionKey;
      if (children.length === 0 && parentOfSelf && worktreeOf(entryOf.get(sessionKey))) {
        parentKey = parentOfSelf;
        children = childrenOf(parentKey);
      }
      return { parentKey, parent: entryOf.get(parentKey), children };
    };

    // wtc --version, asked at most once a minute: requests that overlap
    // share the one that is running.
    let wtcVersion = { at: 0, output: null };
    const wtcVersionOutput = () => {
      if (Date.now() - wtcVersion.at > 60000) {
        wtcVersion = {
          at: Date.now(),
          output: run(["wtc", "--version"], { timeoutMs: 10000 }).then((r) => r.stdout, () => ""),
        };
      }
      return wtcVersion.output;
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
          // Only the URL is read from the clone, whose config an agent can
          // write to; git then runs outside it, with the gateway's config.
          url = (await run(["git", "config", "--file", path.join(cwd, ".git", "config"), "--get", "remote.origin.url"])).stdout.trim();
        }
        if (!/^https:\/\/(github\.com|bitbucket\.org)\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+?(\.git)?$/.test(url)) {
          return { ok: false, error: cwd ? `${project}'s origin is not a GitHub or Bitbucket https URL` : "give a GitHub or Bitbucket https URL" };
        }
        const origin = url;
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
        const r = await run(["git", ...auth, "ls-remote", "--symref", url, "HEAD", "refs/heads/*"], { timeoutMs: 30000, env });
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

    // The pull requests of the collections, and how the conversation hears
    // that one changed, as pullRequestEvents says: queued for its next turn
    // and, with wake (the default), a turn now.
    const watch = pullRequestWatch(path.join(stateDir, "plugins", PLUGIN_ID, "pull-requests.json"));
    const tell = (agentId, sessionKey, lines) => {
      const mode = api.pluginConfig?.pullRequestEvents ?? "wake";
      if (lines.length === 0 || mode === "off") return;
      // A heartbeat turn tends to find nothing to do: say what is.
      const text = [
        "wtc: pull request news for this collection",
        ...lines.map((l) => `- ${l}`),
        "Bring the task's progress up to date with this.",
        ...(lines.some((l) => / was merged\.$|opened and merged\.$/.test(l)) ? ["After a merge, run wtc catch-up in that repository's checkout, so it continues from the merge, and check what is left of the task."] : []),
        "Tell the person only what they need to act on.",
      ].join("\n");
      try {
        api.runtime.system.enqueueSystemEvent(text, { agentId, sessionKey });
        // Now, as the gateway's own system event with --mode now does: an
        // "event" wake waits for the agent's next heartbeat to be due. The
        // turn answers in the conversation, not in the operator's direct
        // messages.
        if (mode === "wake") api.runtime.system.requestHeartbeat?.({ source: "other", intent: "immediate", reason: "wtc-pull-request", agentId, sessionKey, heartbeat: { target: "last" } });
        api.logger?.info?.(`wtc: told ${sessionKey} about ${lines.length} pull request change${lines.length === 1 ? "" : "s"}`);
      } catch (err) {
        api.logger?.warn?.(`wtc: cannot tell ${sessionKey} about its pull requests: ${err?.message ?? err}`);
      }
    };

    // The collection a session is in, with what wtc status says about it.
    const loadCollection = async (agentId, sessionKey) => {
      const { parentKey, parent, children } = collectionOf(agentId, sessionKey);
      const result = {
        schema: "wtc.collection/v1",
        collection: { sessionKey: parentKey, label: parent?.label ?? parent?.displayName ?? null },
        requestedSessionKey: sessionKey,
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
      const versionLine = await wtcVersionOutput();
      result.version = { plugin: PLUGIN_VERSION, wtc: wtcVersionOf(versionLine) };
      const stale = versionWarning(PLUGIN_VERSION, versionLine);
      if (stale) result.warnings.push(stale);
      if (children.length === 0) return result;
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
          // The wtc project it works in: its harness, or ad hoc.
          if (col.harness?.name) result.collection.project = col.harness.name;
          else if (col.folder && path.basename(path.dirname(col.folder)) === "ad-hoc") result.collection.project = "ad-hoc";
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
      tell(agentId, parentKey, watch.observe(agentId, parentKey, result.repos));
      return result;
    };

    api.session.controls.registerSessionAction({
      id: "collection",
      description: "The repositories of this session's worktree collection: branch, pending changes and pull request of each.",
      requiredScopes: ["operator.read"],
      async handler(action) {
        if (!action.sessionKey || !action.agentId) return { ok: false, error: "sessionKey and agentId are required" };
        return { ok: true, result: await loadCollection(action.agentId, action.sessionKey) };
      },
    });

    // Collections with an open pull request are looked at every few minutes
    // also while nobody has their card open, so that a merge, a failed check
    // or a review reaches the conversation.
    let timer;
    let polling = false;
    const poll = async () => {
      if (polling) return;
      polling = true;
      try {
        for (const c of watch.open()) {
          const known = api.runtime.agent.session.listSessionEntries({ agentId: c.agentId }).some((s) => s.sessionKey === c.sessionKey);
          if (!known) watch.forget(c.agentId, c.sessionKey);
          else await loadCollection(c.agentId, c.sessionKey).catch(() => {});
        }
      } finally {
        polling = false;
      }
    };
    api.registerService?.({
      id: "pull-request-watch",
      start() {
        const minutes = Number(api.pluginConfig?.pullRequestPollMinutes ?? 5);
        if (!(minutes > 0) || (api.pluginConfig?.pullRequestEvents ?? "wake") === "off") return;
        timer = setInterval(() => void poll(), minutes * 60000);
        timer.unref?.();
      },
      stop() {
        clearInterval(timer);
        timer = undefined;
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
      const cwd = collectionFolder(stateDir, name);
      if (!cwd) return { ok: false, error: `collection ${name} has no folder yet: wtc status ${name} makes it` };
      const r = await run(argv, { cwd, timeoutMs: 180000, env: { OPENCLAW_SHELL: "exec" } });
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

    // The gateway's git service for sandboxed sessions, when configured.
    const sandboxGit = api.pluginConfig?.sandboxGit;
    if (sandboxGit?.port && api.registerService) {
      let server = null;
      api.registerService({
        id: "sandbox-git",
        start() {
          server = startSandboxGit({
            host: sandboxGit.host ?? "0.0.0.0",
            port: sandboxGit.port,
            run,
            openclaw: onPath("openclaw") ? ["openclaw"] : [process.execPath, path.resolve(process.argv[1] ?? "")],
            listSessionEntries: (params) => api.runtime.agent.session.listSessionEntries(params),
            collectionOf,
            worktreeOf,
            logger: api.logger,
          });
        },
        stop() {
          server?.close();
          server = null;
        },
      });
    }

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
