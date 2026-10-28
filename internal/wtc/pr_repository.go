package wtc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PR repository names are local identities, not just forge basenames. Only a
// checked-out ext.<name> with matching origin evidence can supply an alias.
type prRepository struct {
	name, remote string
	dirs         []string
}

type prRepositories map[string][]*prRepository

func (repos prRepositories) add(alias string, repo *prRepository) {
	for _, old := range repos[alias] {
		if old == repo {
			return
		}
	}
	repos[alias] = append(repos[alias], repo)
}

func (repos prRepositories) resolve(name string) (*prRepository, error) {
	candidates := repos[name]
	if len(candidates) > 1 || len(candidates) == 1 && len(candidates[0].dirs) > 1 {
		return nil, fmt.Errorf("ambiguous enlisted repository %q; use distinct repository directories", name)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0], nil
}

func (repo *prRepository) dir() string {
	if repo != nil && len(repo.dirs) == 1 {
		return repo.dirs[0]
	}
	return ""
}

func (repos prRepositories) forge(name string) (string, string) {
	if repo, err := repos.resolve(name); err == nil && repo != nil {
		return catchUpForge(repo.remote)
	}
	return "", ""
}

func prForgeIdentity(remote string) string {
	slug, forge := catchUpForge(remote)
	if forge == "" {
		return ""
	}
	for _, part := range strings.Split(slug, "/") {
		if !prRepoName.MatchString(part) || part == "." || part == ".." {
			return ""
		}
	}
	return forge + "/" + slug
}

func (c *Context) prRepositories(names ...string) prRepositories {
	repos := prRepositories{}
	registered := map[string]*prRepository{}
	for _, repo := range c.Registry.Repos {
		identity := &prRepository{name: repo.Name, remote: repo.Remote}
		registered[repo.Name] = identity
		repos.add(repo.Name, identity)
	}
	harness, _ := c.HarnessRepoName()
	if repo := registered[harness]; !c.AdHoc && repo != nil {
		repos.add("harness", repo)
	}
	selected := map[string]bool{}
	for _, name := range names {
		stem := strings.TrimPrefix(name, "ext.")
		selected[stem], selected["ext."+stem] = true, true
		if stem == harness || stem == "harness" {
			selected["harness"], selected["ext.harness"] = true, true
			selected[harness], selected["ext."+harness] = true, true
		}
	}
	// Never let an empty collection path inspect the caller's directory, or a
	// directory without its own .git inherit a containing repository's origin.
	if c.Collection == "" {
		return repos
	}
	root, err := filepath.EvalSymlinks(c.Collection)
	if err != nil {
		return repos
	}
	entries, _ := os.ReadDir(c.Collection)
	for _, entry := range entries {
		name := entry.Name()
		if len(names) != 0 && !selected[name] {
			continue
		}
		if ValidatePRIdentity(name, "1") != nil {
			continue
		}
		path := filepath.Join(c.Collection, name)
		if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Dir(real) != root {
			continue
		}
		top, err := gitOutput("-C", path, "rev-parse", "--show-toplevel")
		if err != nil {
			continue
		}
		if top, err = filepath.EvalSymlinks(top); err != nil || top != real {
			continue
		}
		remote, _ := gitOutput("-C", path, "remote", "get-url", "origin")
		identity := registered[name]
		if name == "harness" && !c.AdHoc {
			identity = registered[harness]
		}
		stem, ext := strings.CutPrefix(name, "ext.")
		forgeID := prForgeIdentity(remote)
		// One ext. prefix is a directory convention, not a general name
		// normalization rule. Registered aliases need the entire forge identity.
		alias := ext && stem != "" && !strings.HasPrefix(stem, "ext.") && forgeID != ""
		if identity == nil && alias {
			if repo := registered[stem]; repo != nil && prForgeIdentity(repo.remote) == forgeID {
				identity = repo
			}
		}
		if identity == nil {
			identity = &prRepository{name: name, remote: remote}
		}
		identity.dirs = append(identity.dirs, name)
		repos.add(name, identity)
		if alias && (registered[stem] == identity || strings.HasSuffix(forgeID, "/"+stem)) {
			repos.add(stem, identity)
		}
	}
	return repos
}

// matchesDir also reports ambiguity involving this directory, so catch-up
// cannot replace an unresolved enlisted identity with a branch-name guess.
func (repos prRepositories) matchesDir(name, dir string) (bool, error) {
	for _, repo := range repos[name] {
		for _, candidate := range repo.dirs {
			if candidate == dir {
				_, err := repos.resolve(name)
				return err == nil, err
			}
		}
	}
	return false, nil
}
