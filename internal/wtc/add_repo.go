package wtc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AddRepoOptions struct {
	Collection string
	Repos      []string
	Branch     string
}

type AddRepoResult struct {
	Collection string   `json:"collection"`
	Added      []string `json:"added"`
	Skipped    []string `json:"skipped,omitempty"`
}

// AddRepositories extends an existing collection using its own registry and
// lifecycle configuration. Repository init runs after the collection env,
// secrets, skills, and MCP configuration are available.
func (c *Context) AddRepositories(opt AddRepoOptions) (AddRepoResult, error) {
	result := AddRepoResult{Added: []string{}}
	if len(opt.Repos) == 0 {
		return result, fmt.Errorf("at least one repository is required")
	}
	if opt.Branch != "" {
		if _, err := gitOutput("check-ref-format", "--branch", opt.Branch); err != nil {
			return result, fmt.Errorf("invalid explicit branch: %w", err)
		}
	}
	path := c.Collection
	if opt.Collection != "" {
		if !collectionNamePattern.MatchString(opt.Collection) {
			return result, fmt.Errorf("invalid collection name %q", opt.Collection)
		}
		path = filepath.Join(c.Workspace, opt.Collection)
	}
	target, err := OpenCollection(path)
	if err != nil {
		return result, err
	}
	result.Collection = target.Collection
	if _, err := os.Stat(filepath.Join(target.Collection, ".env.collection")); os.IsNotExist(err) {
		target.ConfigRoot = c.ConfigRoot
	} else if err != nil {
		return result, err
	}
	if len(opt.Repos) > 1 && opt.Collection == "" {
		if _, err := target.Repository(opt.Repos[0]); err != nil {
			if info, statErr := os.Stat(filepath.Join(c.Workspace, opt.Repos[0], "harness")); statErr == nil && info.IsDir() {
				return result, fmt.Errorf("%q is a collection, not a repository; use --collection %s", opt.Repos[0], opt.Repos[0])
			}
		}
	}
	harnessRepo, err := target.HarnessRepoName()
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, name := range opt.Repos {
		if name == "harness" || name == harnessRepo {
			result.Skipped = append(result.Skipped, name)
			continue
		}
		if seen[name] {
			return result, fmt.Errorf("repository %q was requested more than once", name)
		}
		seen[name] = true
		if _, err := target.Repository(name); err != nil {
			return result, err
		}
		if _, err := os.Lstat(filepath.Join(target.Collection, name)); err == nil {
			return result, fmt.Errorf("worktree path already exists: %s", filepath.Join(target.Collection, name))
		} else if !os.IsNotExist(err) {
			return result, err
		}
	}
	toAdd := make([]string, 0, len(seen))
	for _, name := range opt.Repos {
		if name != "harness" && name != harnessRepo {
			toAdd = append(toAdd, name)
		}
	}
	values := map[string]string{"repositories": strings.Join(toAdd, ","), "branch": opt.Branch}
	if err := target.RunHook("add-repo.pre", values); err != nil {
		return result, err
	}
	for _, name := range opt.Repos {
		if name == "harness" || name == harnessRepo {
			continue
		}
		if err := target.AddWorktree(name, name, target.Collection, opt.Branch); err != nil {
			return result, fmt.Errorf("add %s: %w", name, err)
		}
		result.Added = append(result.Added, name)
	}
	if _, err := os.Stat(filepath.Join(target.Collection, ".env.collection")); os.IsNotExist(err) {
		env, err := target.RenderEnv()
		if err != nil {
			return result, err
		}
		if err := target.WriteEnv(env); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	}
	if err := target.EnsureEnvSupport(); err != nil {
		return result, err
	}
	if err := target.TrustMise(); err != nil {
		return result, err
	}
	if err := target.RunHook("secrets.link.pre", nil); err != nil {
		return result, err
	}
	for _, name := range result.Added {
		if _, err := target.LinkSecrets(SecretLinkOptions{Repo: name}); err != nil {
			return result, err
		}
	}
	if err := target.RunHook("secrets.link.post", nil); err != nil {
		return result, err
	}
	if _, err := target.RenderSkills(SkillRenderOptions{}); err != nil {
		return result, err
	}
	if _, err := os.Stat(filepath.Join(target.Harness, ".mcp-servers.yml")); err == nil {
		outputs, err := target.RenderMCP()
		if err != nil {
			return result, err
		}
		if _, err := target.WriteMCP(outputs, false); err != nil {
			return result, err
		}
	} else if !os.IsNotExist(err) {
		return result, err
	}
	for _, name := range result.Added {
		RunRepoInit(filepath.Join(target.Collection, name))
	}
	_, _ = target.AgentToolchainPath(true)
	if err := target.RunHook("add-repo.post", values); err != nil {
		return result, err
	}
	return result, nil
}
