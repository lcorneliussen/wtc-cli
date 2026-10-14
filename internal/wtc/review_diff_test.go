package wtc

import (
	"fmt"
	"testing"
)

func TestReviewDiffRightSideHunks(t *testing.T) {
	diff := []byte(`diff --git a/old name.go b/new name.go
similarity index 90%
rename from old name.go
rename to new name.go
--- a/old name.go
+++ b/new name.go
@@ -3,2 +3,3 @@
 context
-old
+new
+++ b/not-a-header
@@ -50 +51 @@
-old
+new
diff --git a/deleted.go b/deleted.go
--- a/deleted.go
+++ /dev/null
@@ -1 +0,0 @@
-removed
diff --git a/new.go b/new.go
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+new
diff --git "a/caf\303\251.go" "b/caf\303\251.go"
--- "a/caf\303\251.go"
+++ "b/caf\303\251.go"
@@ -1 +1 @@
-old
+new
diff --git a/image.png b/image.png
Binary files a/image.png and b/image.png differ
`)
	lines, err := parseReviewDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		line int
		want bool
	}{
		{"new name.go", 2, false}, {"new name.go", 3, true}, {"new name.go", 5, true},
		{"new name.go", 6, false}, {"new name.go", 50, false}, {"new name.go", 51, true},
		{"old name.go", 3, false}, {"not-a-header", 3, false}, {"pr.md", 50, false},
		{"deleted.go", 1, false}, {"new.go", 1, true}, {"new.go", 2, false},
		{"café.go", 1, true}, {"image.png", 1, false},
	} {
		if got := lines.contains(tc.path, tc.line); got != tc.want {
			t.Errorf("%s:%d = %t, want %t", tc.path, tc.line, got, tc.want)
		}
	}
	for _, invalid := range []string{"@@ malformed", "+++ not-a-git-path", "+++ \"unterminated", "+++ b/a\n@@ -1 +0,1 @@"} {
		if _, err := parseReviewDiff([]byte(invalid)); err == nil {
			t.Fatalf("accepted invalid diff: %q", invalid)
		}
	}
}

func TestReviewPosition422IsNotEveryValidationFailure(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{422, `{"errors":["Pull request review thread line must be part of the diff"]}`, true},
		{422, `{"errors":[{"field":"line","code":"invalid"}]}`, true},
		{422, `{"message":"Line could not be resolved"}`, true},
		{422, `{"errors":[{"field":"body","code":"blank"}]}`, false},
		{422, `{"message":"Validation Failed, or the endpoint has been spammed"}`, false},
		{403, `{"message":"Line could not be resolved"}`, false},
		{503, `{"message":"Service unavailable"}`, false},
	} {
		if got := reviewUnresolvablePosition([]byte(tc.body), fmt.Errorf("gh: rejected (HTTP %d)", tc.status)); got != tc.want {
			t.Errorf("HTTP %d %s: skip %t, want %t", tc.status, tc.body, got, tc.want)
		}
	}
	if reviewUnresolvablePosition([]byte(`{"message":"Line could not be resolved"}`), fmt.Errorf("network failure for PR 422")) {
		t.Fatal("a PR number was mistaken for an HTTP status")
	}
}
