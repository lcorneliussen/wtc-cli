package wtc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lcorneliussen/wtc-cli/internal/envcompat"
	"golang.org/x/sys/unix"
)

// Status talks to Bitbucket and GitHub from every open status pane. Those
// forges bill the calls to one user. A shared cache, one in-flight fetch per
// record, and a shared allowance keep the panes from spending the hour on
// the first few minutes. A rate-limit response pauses that forge and status
// keeps showing the last saved record.

const (
	statusForgeCoolDefault = 15 * time.Minute
	statusBitbucketBurst   = 8
	statusBitbucketEvery   = 8 * time.Second
	statusGitHubBurst      = 16
	statusGitHubEvery      = 2 * time.Second
)

var (
	errForgeCooling = errors.New("forge rate limit cooling down")
	errForgeBudget  = errors.New("forge request budget spent")
)

var statusRateLimitedText = regexp.MustCompile(`(?i)(?:^|[^0-9])429(?:[^0-9]|$)|too many requests|rate limit exceeded|rate limit for this resource|secondary rate limit`)
var statusRetryAfterText = regexp.MustCompile(`(?i)retry[- ]after:?\s*(\d+)`)

func statusRateLimited(err error) bool {
	return err != nil && statusRateLimitedText.MatchString(err.Error())
}

func statusRetryAfter(err error) time.Duration {
	if err == nil {
		return statusForgeCoolDefault
	}
	match := statusRetryAfterText.FindStringSubmatch(err.Error())
	if match == nil {
		return statusForgeCoolDefault
	}
	seconds, err := strconv.Atoi(match[1])
	if err != nil || seconds < 1 {
		return statusForgeCoolDefault
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

// statusForgeAllowance is the shared burst and the gap between later calls.
// WTC_FORGE_MIN_INTERVAL=0 turns the allowance off. A positive number of
// seconds allows one call, then waits that long.
func statusForgeAllowance(forge string) (burst int, every time.Duration) {
	if raw := envcompat.Getenv("WTC_FORGE_MIN_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return 1 << 30, time.Second
		}
		return 1, time.Duration(seconds) * time.Second
	}
	if forge == "bitbucket.org" {
		return statusBitbucketBurst, statusBitbucketEvery
	}
	return statusGitHubBurst, statusGitHubEvery
}

func statusForgeStatePath(prefix, forge string) string {
	return statusForgeEntryPath(prefix, forge, "", "")
}

func statusForgeCooling(forge string) bool {
	raw, err := os.ReadFile(statusForgeStatePath("cool", forge))
	if err != nil {
		return false
	}
	until, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	return err == nil && time.Now().Before(time.Unix(0, until))
}

func statusForgeCool(forge string, retry time.Duration) {
	if retry <= 0 {
		retry = statusForgeCoolDefault
	}
	path := statusForgeStatePath("cool", forge)
	if !statusSecureCacheDir(statusForgeCacheDir()) {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Add(retry).UnixNano(), 10)), 0600)
}

func statusTakeForgeToken(forge string) bool {
	burst, every := statusForgeAllowance(forge)
	if burst >= 1<<30 {
		return true
	}
	path := statusForgeStatePath("budget", forge)
	unlock, err := statusLockCache(path)
	if err != nil {
		return true
	}
	defer unlock()
	now := time.Now()
	tokens, updated := burst, now
	if raw, err := os.ReadFile(path); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) == 2 {
			if parsed, err := strconv.Atoi(fields[0]); err == nil {
				tokens = parsed
			}
			if parsed, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				updated = time.Unix(0, parsed)
			}
		}
	}
	if now.After(updated) {
		gained := int(now.Sub(updated) / every)
		if gained > 0 {
			tokens += gained
			updated = updated.Add(time.Duration(gained) * every)
		}
	}
	if tokens > burst {
		tokens = burst
		updated = now
	}
	if tokens < 1 {
		return false
	}
	tokens--
	return os.WriteFile(path, []byte(fmt.Sprintf("%d %d\n", tokens, updated.UnixNano())), 0600) == nil
}

func statusLockCache(path string) (func(), error) {
	if !statusSecureCacheDir(statusForgeCacheDir()) {
		return nil, errors.New("forge cache")
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		file.Close()
	}, nil
}

// statusWithForgeBudget runs fetch when this forge still has allowance.
// A rate-limit response pauses the forge. A saved body is returned instead
// of calling again while the pause or the allowance says to wait.
func statusWithForgeBudget(forge string, stale []byte, fetch func() ([]byte, error)) ([]byte, error) {
	if statusForgeCooling(forge) {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, errForgeCooling
	}
	if !statusTakeForgeToken(forge) {
		if len(stale) > 0 {
			return stale, nil
		}
		return nil, errForgeBudget
	}
	raw, err := fetch()
	if err != nil {
		if statusRateLimited(err) {
			statusForgeCool(forge, statusRetryAfter(err))
			if len(stale) > 0 {
				return stale, nil
			}
		}
		return nil, err
	}
	return raw, nil
}

// statusSingleFlight returns the fresh cached body, or fetches it once while
// other status processes wait. The allowance is shared across every record.
func statusSingleFlight(kind, forge, slug, key string, fetch func() ([]byte, error)) ([]byte, error) {
	if data, ok := statusReadForgeEntry(kind, forge, slug, key); ok {
		return data, nil
	}
	stale, _ := statusReadForgeEntryAny(kind, forge, slug, key)
	unlock, lockErr := statusLockCache(statusForgeEntryPath(kind, forge, slug, key))
	if lockErr == nil {
		defer unlock()
		if data, ok := statusReadForgeEntry(kind, forge, slug, key); ok {
			return data, nil
		}
	}
	raw, err := statusWithForgeBudget(forge, stale, fetch)
	if err != nil {
		return nil, err
	}
	if len(stale) > 0 && bytes.Equal(raw, stale) {
		return raw, nil
	}
	statusWriteForgeEntry(kind, forge, slug, key, raw)
	return raw, nil
}
