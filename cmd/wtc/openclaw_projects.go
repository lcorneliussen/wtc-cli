package main

import (
	"fmt"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/openclaw"
	"github.com/spf13/cobra"
)

// gatewayProject is a wtc project on a gateway: a configured harness, whose
// registry names the repositories a collection in it can work with.
type gatewayProject struct {
	Name    string               `json:"name"`
	Harness string               `json:"harness"`
	Ref     string               `json:"ref"`
	Default bool                 `json:"default,omitempty"`
	Repos   []gatewayProjectRepo `json:"repos"`
	Error   string               `json:"error,omitempty"`
}

type gatewayProjectRepo struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
	Role   string `json:"role,omitempty"`
}

// gatewayProjects lists the gateway's wtc projects with their repositories,
// read from the gateway's clone of each harness.
func gatewayProjects() ([]gatewayProject, error) {
	cfg, err := gatewayConfig()
	if err != nil {
		return nil, err
	}
	projects := []gatewayProject{}
	for _, h := range cfg.Harnesses {
		p := gatewayProject{Name: h.Name, Harness: h.Remote, Ref: h.Ref, Default: h.Name == cfg.DefaultHarness, Repos: []gatewayProjectRepo{}}
		reg := readHarnessRegistry(h, "")
		if reg.err != nil {
			p.Error = reg.err.Error()
		}
		for _, r := range reg.repos {
			p.Repos = append(p.Repos, gatewayProjectRepo{Name: r.Name, Remote: r.Remote, Role: r.Role})
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func addProjectsCommand(root *cobra.Command, asJSON *bool) {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Under OpenClaw: list the gateway's wtc projects and their repositories",
		Long: `A wtc project is a named set of repositories: a harness the gateway is
configured with, whose .harness-repos.yml lists them. A collection works in
one wtc project, with worktrees of the repositories a task needs, or in an
ad-hoc one, which takes any repository.

projects lists them, so that what the person asks to work on can be matched to
a project and its repositories before wtc add-repo starts a collection.`,
		Args: cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if h := currentHost(); h != openclaw.HostOpenClaw {
			return fmt.Errorf("wtc projects lists an OpenClaw gateway's wtc projects, and this is a %s session: a harness's .harness-repos.yml lists its repositories", h)
		}
		projects, err := gatewayProjects()
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: map[string]any{"projects": projects}, Summary: projectsSummary(projects)}, *asJSON)
	}
	root.AddCommand(cmd)
}

func projectsSummary(projects []gatewayProject) string {
	var b strings.Builder
	if len(projects) == 0 {
		b.WriteString("This gateway configures no wtc projects: every collection is ad hoc.\n")
	}
	for _, p := range projects {
		def := ""
		if p.Default {
			def = " (default)"
		}
		fmt.Fprintf(&b, "%s%s: harness %s @ %s\n", p.Name, def, p.Harness, p.Ref)
		if p.Error != "" {
			fmt.Fprintf(&b, "  %s\n", p.Error)
		}
		for _, r := range p.Repos {
			role := ""
			if r.Role != "" {
				role = " — " + r.Role
			}
			fmt.Fprintf(&b, "  %s  %s%s\n", r.Name, r.Remote, role)
		}
	}
	b.WriteString("Any other repository: ad hoc, wtc add-repo <url> --project none, or beside a project's with --ad-hoc.\n")
	b.WriteString("Start a collection: wtc add-repo <repo> [...] --worktree <name>; the project whose repositories these are is picked.")
	return b.String()
}
