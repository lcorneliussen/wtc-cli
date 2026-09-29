package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

type envResult struct {
	Collection string `json:"collection"`
	Changed    bool   `json:"changed"`
	DryRun     bool   `json:"dry_run"`
	Env        string `json:"env"`
	Mise       string `json:"mise"`
}

type envSweepResult struct {
	envResult
	Error string `json:"error,omitempty"`
}

func refreshEnvCollection(c *wtc.Context, dryRun, invokeHooks bool) (envResult, error) {
	data, err := c.RenderEnv()
	if err != nil {
		return envResult{}, err
	}
	mise, err := c.RenderMise()
	if err != nil {
		return envResult{}, err
	}
	old, _ := os.ReadFile(filepath.Join(c.Collection, ".env.collection"))
	oldMise, _ := os.ReadFile(filepath.Join(c.Collection, "mise.toml"))
	changed := !bytes.Equal(old, data) || !bytes.Equal(oldMise, mise)
	if !dryRun {
		if err := c.ValidateEnvSupport(); err != nil {
			return envResult{}, err
		}
		if invokeHooks {
			if err := c.RunHook("env.pre", nil); err != nil {
				return envResult{}, err
			}
		}
		if changed {
			if err := c.WriteEnv(data); err != nil {
				return envResult{}, err
			}
		}
		if err := c.EnsureEnvSupport(); err != nil {
			return envResult{}, err
		}
		if invokeHooks {
			if err := c.TrustMise(); err != nil {
				return envResult{}, err
			}
		}
		if invokeHooks {
			if err := c.RunHook("env.post", nil); err != nil {
				return envResult{}, err
			}
		}
	}
	return envResult{Collection: c.Collection, Changed: changed, DryRun: dryRun, Env: string(data), Mise: string(mise)}, nil
}

func envSummary(result envResult) string {
	state := "already current"
	if result.Changed {
		if result.DryRun {
			state = "would update"
		} else {
			state = "updated"
		}
	}
	return filepath.Base(result.Collection) + ": " + state
}

func printEnvPreview(result envResult) {
	fmt.Print(result.Env)
	fmt.Print("\n# mise.toml\n")
	fmt.Print(result.Mise)
}
