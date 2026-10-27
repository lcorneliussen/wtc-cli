package main

import (
	"encoding/json"
	"errors"
	"os"
	"slices"

	"github.com/lcorneliussen/wtc-cli/internal/openclaw"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

// This command is called only by the gateway service with its operator-owned
// allowlist. The sandbox cannot choose these arguments over HTTP.
func sandboxProjectsCommand() *cobra.Command {
	var allowed []string
	cmd := &cobra.Command{Use: "sandbox-projects", Hidden: true, Args: cobra.NoArgs}
	cmd.Flags().StringArrayVar(&allowed, "project", nil, "Operator-allowed project (repeatable)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(allowed) == 0 {
			return errors.New("project discovery is not enabled for this agent")
		}
		projects, err := localGatewayProjects(allowed)
		if err != nil {
			return errors.New("gateway project metadata unavailable")
		}
		result := openclaw.SandboxProjects{Projects: projects}
		if cwd, err := os.Getwd(); err == nil {
			result.Collection = openclaw.CurrentWorktree(cwd)
			if folder := openclaw.CollectionFolder(gatewayStateDir(), result.Collection); result.Collection != "" && folder != "" {
				result.CollectionProject, _, err = wtc.GatewayHarness(folder)
				if err != nil || (result.CollectionProject != "" && result.CollectionProject != openclaw.NoHarness && !slices.Contains(allowed, result.CollectionProject)) {
					return errors.New("gateway collection metadata unavailable")
				}
			}
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	return cmd
}

// harnessProjectContext uses one snapshot per add-repo plan. When a gateway
// URL is configured, unavailable or denied discovery never becomes an empty
// local config or an ad-hoc plan.
func harnessProjectContext(worktree string) (openclaw.GatewayConfig, map[string]harnessRegistry, string, error) {
	registries := map[string]harnessRegistry{}
	gw := openclaw.GatewayFromEnv(os.Getenv)
	if gw == nil {
		cfg, err := gatewayConfig()
		return cfg, registries, "", err
	}
	metadata, err := gw.Projects()
	if err != nil {
		return openclaw.GatewayConfig{}, nil, "", err
	}
	var cfg openclaw.GatewayConfig
	for _, p := range metadata.Projects {
		cfg.Harnesses = append(cfg.Harnesses, openclaw.Harness{Name: p.Name, Remote: p.Harness, Ref: p.Ref})
		if p.Default {
			cfg.DefaultHarness = p.Name
		}
		reg := harnessRegistry{projectID: p.ProjectID, projectIDs: map[string]string{}}
		if p.Error != "" {
			reg.err = errors.New(p.Error)
		}
		for _, r := range p.Repos {
			reg.repos = append(reg.repos, wtc.Repo{Name: r.Name, Remote: r.Remote, Role: r.Role, DefaultRef: r.DefaultRef})
			reg.projectIDs[r.Name] = r.ProjectID
		}
		registries[p.Name] = reg
	}
	recorded := ""
	if metadata.Collection == worktree {
		recorded = metadata.CollectionProject
	}
	return cfg, registries, recorded, nil
}
