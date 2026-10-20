// The gateway side of the plugin, where it needs no gateway.
import { test } from "node:test";
import assert from "node:assert/strict";
import { versionWarning } from "../index.js";

test("the panel warns when the gateway's wtc is not the release the plugin came from", () => {
  assert.equal(versionWarning("0.1.42", "wtc version 0.1.42\n"), null);
  assert.match(versionWarning("0.1.42", "wtc version 0.1.43\n"), /the wtc 0\.1\.42 plugin.*runs wtc 0\.1\.43.*wtc openclaw install/);
  assert.match(versionWarning("0.1.0", "wtc version v0.1.43"), /0\.1\.43/);
  // Dev builds, a missing wtc and an unknown plugin version say nothing.
  assert.equal(versionWarning("0.1.42", "wtc version dev"), null);
  assert.equal(versionWarning("0.1.42", "wtc version 0.1.43 (dev build, in place of 0.1.42)"), null);
  assert.equal(versionWarning("0.1.42", ""), null);
  assert.equal(versionWarning(null, "wtc version 0.1.43"), null);
});
