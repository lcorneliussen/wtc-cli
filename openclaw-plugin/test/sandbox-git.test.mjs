// The gateway's git service for sandboxes, without a gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { parseCgiHead, projectionWorktreeId, startSandboxGit, uploadPackConfig } from "../sandbox-git.js";

test("a sandbox's workspace names the managed worktree it projects", () => {
  assert.equal(projectionWorktreeId("/home/node/.openclaw/worktree-projections/ee0ae31d-cf9a/workspace"), "ee0ae31d-cf9a");
  assert.equal(projectionWorktreeId("/home/node/.openclaw/workspaces/dev"), null);
  assert.equal(projectionWorktreeId("/x/worktree-projections/../workspace"), null);
  assert.equal(projectionWorktreeId(undefined), null);
});

test("upload-pack shows origin, tags and the collection's branches, and nothing else", () => {
  const dir = mkdtempSync(path.join(os.tmpdir(), "wtc-sandbox-git-"));
  try {
    const git = (...args) => execFileSync("git", ["-C", dir, ...args], { encoding: "utf8", env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null" } });
    git("init", "-q", "-b", "main");
    git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init");
    for (const ref of ["refs/heads/openclaw/ours", "refs/heads/openclaw/ours--a", "refs/heads/openclaw/theirs", "refs/remotes/origin/main", "refs/tags/v1", "refs/openclaw/snapshots/x"]) {
      git("update-ref", ref, "HEAD");
    }
    const env = { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", ...uploadPackConfig(new Set(["openclaw/ours"])) };
    // What http-backend runs. (A local ls-remote would drop the config.)
    const shown = execFileSync("git", ["upload-pack", "--advertise-refs", "--stateless-rpc", dir], { encoding: "utf8", env })
      .split("\n").map((l) => l.slice(4).split("\0")[0].split(" ")[1]).filter(Boolean).sort();
    assert.deepEqual(shown, ["refs/heads/openclaw/ours", "refs/remotes/origin/main", "refs/tags/v1"]);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("a CGI response's status, headers and body", () => {
  const head = parseCgiHead(Buffer.from("Status: 403 Forbidden\r\nContent-Type: text/plain\r\n\r\nno"));
  assert.deepEqual([head.status, head.headers["Content-Type"]], [403, "text/plain"]);
  assert.equal(parseCgiHead(Buffer.from("Content-Type: x\r\n")), null);
  const plain = parseCgiHead(Buffer.from("Content-Type: x\n\nbody"));
  assert.deepEqual([plain.status, plain.bodyStart], [200, 17]);
});

test("a connection from no sandbox container is refused", async () => {
  const asked = [];
  const server = startSandboxGit({
    host: "127.0.0.1",
    port: 0,
    run: async (argv) => {
      asked.push(argv.join(" "));
      return { code: 0, stdout: JSON.stringify({ containers: [] }), stderr: "" };
    },
    openclaw: ["openclaw"],
    listSessionEntries: () => [],
    collectionOf: () => ({ children: [] }),
    worktreeOf: () => null,
  });
  try {
    await new Promise((resolve) => server.once("listening", resolve));
    const { port } = server.address();
    const r = await fetch(`http://127.0.0.1:${port}/api/info/refs?service=git-upload-pack`);
    assert.equal(r.status, 403);
    assert.match((await r.json()).error, /no sandbox container/);
    assert.ok(asked.every((a) => a === "openclaw sandbox list --json"), asked.join("; "));
  } finally {
    server.close();
  }
});
