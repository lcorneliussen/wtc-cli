package next

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPredictMergeAtPreservesPathsAndUsesPinnedCommits(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@example.invalid"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	path := " leading\nand trailing .md "
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("base\n")
	git("add", ".")
	git("commit", "-qm", "base")
	root := git("rev-parse", "HEAD")
	write("source\n")
	git("commit", "-qam", "source")
	head := git("rev-parse", "HEAD")
	git("switch", "--detach", root)
	write("destination\n")
	git("commit", "-qam", "destination")
	base := git("rev-parse", "HEAD")
	// HEAD equals the base, so using HEAD rather than the source would be clean.
	if conflicts, ok := PredictMergeAt(dir, head, base); !ok || !reflect.DeepEqual(conflicts, []string{path}) {
		t.Fatalf("lost commit identity or path bytes: %q, %v", conflicts, ok)
	}
	if git("rev-parse", "HEAD") != base || git("status", "--porcelain") != "" {
		t.Fatal("prediction changed checkout")
	}
	if _, ok := PredictMergeAt(dir, "no-such-source", base); ok {
		t.Fatal("missing source accepted")
	}
	git("checkout", "--orphan", "unrelated")
	git("commit", "-qm", "unrelated root")
	if _, ok := PredictMergeAt(dir, head, git("rev-parse", "HEAD")); ok {
		t.Fatal("unrelated histories reported mergeable")
	}
}
