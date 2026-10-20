package openclaw

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Git runs here as the gateway's user, in checkouts an agent can write to,
// including their repository's config. Settings in that config can make git
// run programs: a filter driver while status compares contents, a signature
// program, fsmonitor, hooks, an ssh command, an upload-pack. So every command
// gets the overrides below, and refuses a checkout whose repository or
// worktree config sets anything beyond a short list of keys that only
// describe the checkout. The gateway's own global and system config stay
// trusted: they are the admin's.
var trustedArgs = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=/dev/null",
	"-c", "log.showSignature=false",
	"-c", "commit.gpgSign=false",
	"-c", "tag.gpgSign=false",
	"-c", "gpg.program=false",
	"-c", "gpg.ssh.program=false",
	"-c", "gpg.x509.program=false",
}

// The repository and worktree config keys wtc runs git with. The ones the
// overrides above neutralise are listed too: a hooks path set by a JavaScript
// toolchain is common and harmless once hooks are off.
var allowedRepoKey = regexp.MustCompile(`^(` + strings.Join([]string{
	`core\.(repositoryformatversion|filemode|bare|logallrefupdates|ignorecase|precomposeunicode|symlinks|autocrlf|eol|safecrlf|sparsecheckout|sparsecheckoutcone|hookspath|fsmonitor)`,
	`extensions\.[a-z]+`,
	`remote\..+\.(url|pushurl|fetch|tagopt|prune|promisor|partialclonefilter|mirror)`,
	`branch\..+\.(remote|pushremote|merge|rebase|description|` + baseKey + `)`,
	`submodule\..+\.(url|active|branch)`,
	`user\.(name|email)`,
	`(log\.showsignature|commit\.gpgsign|tag\.gpgsign|gpg\.format|gpg\.program|gpg\.ssh\.program|gpg\.x509\.program)`,
	`lfs\.repositoryformatversion`,
	`(fetch\.prune|pull\.rebase|pull\.ff|merge\.conflictstyle|rerere\.enabled|init\.defaultbranch|maintenance\.auto|maintenance\.strategy|gc\.auto)`,
	`(advice|color|index|pack|feature)\..+`,
}, "|") + `)$`)

// UntrustedConfigError is a checkout wtc runs no git in.
type UntrustedConfigError struct{ msg string }

func (e *UntrustedConfigError) Error() string { return e.msg }

// untrustedConfig says which key, if any, of dir's repository or worktree
// config wtc will not run git with. Listing config runs nothing. It is read
// before every command, as an agent can change it between two.
func untrustedConfig(dir string) error {
	var result error
	out, err := exec.Command("git", "-C", dir, "config", "--list", "--show-scope", "-z").Output()
	if err == nil {
		// -z prints scope NUL key LF value NUL per entry.
		fields := strings.Split(string(out), "\x00")
		for i := 0; i+1 < len(fields); i += 2 {
			scope := fields[i]
			if scope != "local" && scope != "worktree" {
				continue
			}
			key, _, _ := strings.Cut(fields[i+1], "\n")
			if !allowedRepoKey.MatchString(strings.ToLower(key)) {
				flag := ""
				if scope == "worktree" {
					flag = " --worktree"
				}
				result = &UntrustedConfigError{fmt.Sprintf("%s: its %s git config sets %s, which can make git run a program, so wtc runs no git there. Remove it with `git config%s --unset-all %s` if you did not set it on purpose", dir, scope, key, flag, key)}
				break
			}
		}
	}
	return result
}

// gitCommand is the git command wtc runs in dir: refused when dir's config
// is not one to trust, and with the overrides otherwise.
func gitCommand(ctx context.Context, dir string, args ...string) (*exec.Cmd, error) {
	if err := untrustedConfig(dir); err != nil {
		return nil, err
	}
	full := append(append([]string{"-C", dir}, trustedArgs...), args...)
	if ctx == nil {
		return exec.Command("git", full...), nil
	}
	return exec.CommandContext(ctx, "git", full...), nil
}
