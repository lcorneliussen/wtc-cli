package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func runtimeContext() (*wtc.Context, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return wtc.Discover(cwd)
}

func addRuntimeCommands(root *cobra.Command, asJSON *bool) {
	runtime := &cobra.Command{Use: "runtime", Short: "Inspect or render the opt-in dekit runtime"}
	runtime.RunE = func(cmd *cobra.Command, args []string) error { return cmd.Help() }
	var dryRun bool
	render := &cobra.Command{Use: "render", Short: "Render repository dekit tasks without starting services", Args: cobra.NoArgs}
	render.Flags().BoolVar(&dryRun, "dry-run", false, "Preview task ownership without writing or requiring dekit")
	render.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := runtimeContext()
		if err != nil {
			return err
		}
		result, err := c.RenderRuntime(dryRun)
		if err != nil {
			return err
		}
		if *asJSON {
			return emit(envelope{OK: true, Data: result, Summary: fmt.Sprintf("runtime: %d task(s)", len(result.Items))}, true)
		}
		fmt.Print(wtc.RuntimeText(result.Items))
		return nil
	}
	status := &cobra.Command{Use: "status", Short: "Query existing runtime state without starting it", Args: cobra.NoArgs}
	status.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := runtimeContext()
		if err != nil {
			return err
		}
		items := c.RuntimeStatus()
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(items)
		}
		fmt.Print(wtc.RuntimeText(items))
		return nil
	}
	teardown := &cobra.Command{Use: "teardown", Short: "Stop the runtime and strictly tear down collection-owned resources", Args: cobra.NoArgs}
	teardown.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := runtimeContext()
		if err != nil {
			return err
		}
		if err := c.RetireRuntime(); err != nil {
			return err
		}
		return emit(envelope{OK: true, Summary: "runtime stopped and resource teardown completed"}, *asJSON)
	}
	provision := &cobra.Command{Use: "provision [repo/task-or-group]", Short: "Run idempotent resource hooks without starting services", Args: cobra.MaximumNArgs(1)}
	provision.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := runtimeContext()
		if err != nil {
			return err
		}
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		result, err := c.RuntimeAction("provision", target, 0)
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: result, Summary: "runtime resources provisioned"}, *asJSON)
	}
	runtime.AddCommand(render, status, provision, teardown)
	root.AddCommand(runtime)
	for _, action := range []string{"up", "down", "restart"} {
		var wait time.Duration
		cmd := &cobra.Command{Use: action + " [repo/task-or-group]", Short: action + " the collection's opt-in dekit runtime", Args: cobra.MaximumNArgs(1)}
		if action != "down" {
			cmd.Flags().DurationVar(&wait, "wait", 30*time.Second, "Wait for selected startup readiness (0 returns immediately)")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if wait < 0 {
				return fmt.Errorf("--wait must be nonnegative")
			}
			c, err := runtimeContext()
			if err != nil {
				return err
			}
			target := ""
			if len(args) != 0 {
				target = args[0]
			}
			result, err := c.RuntimeAction(action, target, wait)
			if *asJSON {
				if emitErr := emit(envelope{OK: err == nil, Data: result, Summary: action + " runtime"}, true); emitErr != nil {
					return emitErr
				}
			}
			if err != nil {
				return err
			}
			if !*asJSON {
				fmt.Print(wtc.RuntimeText(result.Items))
			}
			return nil
		}
		root.AddCommand(cmd)
	}
	var follow bool
	var tail int
	logs := &cobra.Command{Use: "logs <repo/task>", Short: "Read a task's persistent logs without starting the runtime", Args: cobra.ExactArgs(1)}
	logs.Flags().BoolVarP(&follow, "follow", "f", false, "Follow appended log output")
	logs.Flags().IntVar(&tail, "tail", 100, "Number of recent lines")
	logs.RunE = func(cmd *cobra.Command, args []string) error {
		if *asJSON {
			return fmt.Errorf("logs emits plain text; use wtc runtime status --json for state")
		}
		if tail < 0 {
			return fmt.Errorf("--tail must be nonnegative")
		}
		c, err := runtimeContext()
		if err != nil {
			return err
		}
		path, err := c.RuntimeLogPath(args[0])
		if err != nil {
			return err
		}
		argv := []string{"-n", fmt.Sprint(tail)}
		if follow {
			argv = append(argv, "-f")
		}
		argv = append(argv, path)
		reader := exec.CommandContext(cmd.Context(), "tail", argv...)
		reader.Stdout, reader.Stderr = os.Stdout, os.Stderr
		return reader.Run()
	}
	root.AddCommand(logs)
}
