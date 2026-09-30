package wtc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type statusBuildRequest struct {
	Collection string `json:"collection"`
	Repo       string `json:"repo"`
	Worktree   string `json:"worktree"`
	Slug       string `json:"slug"`
	Branch     string `json:"branch"`
	Tier       string `json:"tier"`
}

type statusLimitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *statusLimitedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.limit {
		b.overflow = true
		return 0, fmt.Errorf("output too large")
	}
	return b.Buffer.Write(data)
}

func (c *Context) statusBuildHookPath() (string, error) {
	path := filepath.Join(c.Harness, "hooks", "wtc", "status.build.sh")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("status build hook must be executable: %s", path)
	}
	return path, nil
}

func (c *Context) statusBuildFromHook(path string, request statusBuildRequest) (*StatusBuild, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path)
	command.Dir = c.Collection
	command.Env = append(os.Environ(), "WTC_COLLECTION="+filepath.Base(c.Collection), "WTC_CONFIG_ROOT="+c.ConfigRoot)
	command.Stdin = bytes.NewReader(payload)
	stdout := statusLimitedBuffer{limit: 1 << 20}
	command.Stdout = &stdout
	stderr := statusLimitedBuffer{limit: 4096}
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if stdout.overflow || stderr.overflow {
			return nil, fmt.Errorf("status build hook output too large")
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("status build hook timed out")
		}
		return nil, fmt.Errorf("status build hook: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if strings.TrimSpace(stdout.String()) == "" || strings.TrimSpace(stdout.String()) == "null" {
		return nil, nil
	}
	var build StatusBuild
	if err := json.Unmarshal(stdout.Bytes(), &build); err != nil {
		return nil, fmt.Errorf("status build hook JSON: %w", err)
	}
	if build.Branch == "" {
		build.Branch = request.Branch
	}
	if build.Branch != request.Branch {
		return nil, fmt.Errorf("status build hook returned a different branch")
	}
	if build.Checks != nil {
		switch *build.Checks {
		case "SUCCESS", "FAILURE", "PENDING", "NONE", "ERROR", "EXPECTED":
		default:
			return nil, fmt.Errorf("status build hook returned an unknown check state")
		}
	}
	if build.URL != nil && *build.URL != "" {
		parsed, err := url.Parse(*build.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return nil, fmt.Errorf("status build hook returned an invalid build URL")
		}
	}
	if build.Checks == nil && build.Build == nil {
		return nil, nil
	}
	return &build, nil
}

func (c *Context) statusBuildFacts(snapshot *StatusSnapshot) error {
	path, err := c.statusBuildHookPath()
	if err != nil || path == "" {
		return err
	}
	for i := range snapshot.Repos {
		row := &snapshot.Repos[i]
		repo, err := c.Repository(row.Repo)
		if err != nil {
			return err
		}
		tip := strings.TrimPrefix(repo.DefaultRef, "origin/")
		if tip == "" {
			tip = "main"
		}
		slug, _ := c.statusRecordForge(PRRecord{Repo: row.Repo}, snapshot.Repos)
		request := statusBuildRequest{Collection: filepath.Base(c.Collection), Repo: row.Repo,
			Worktree: row.Worktree, Slug: slug, Branch: tip, Tier: "tip"}
		row.Tip, err = c.statusBuildFromHook(path, request)
		if err != nil {
			return fmt.Errorf("%s tip: %w", row.Repo, err)
		}
		prod := strings.TrimPrefix(repo.ProductionRef, "origin/")
		if prod != "" && prod != tip {
			request.Branch, request.Tier = prod, "prod"
			row.Prod, err = c.statusBuildFromHook(path, request)
			if err != nil {
				return fmt.Errorf("%s prod: %w", row.Repo, err)
			}
		}
	}
	return nil
}
