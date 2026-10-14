package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

// handOverToDevBuild replaces this process with the collection's dev build
// when that build is the same version as this one or newer. It returns only
// when this binary should keep running.
func handOverToDevBuild() {
	if os.Getenv(wtc.DevBuildDelegatedEnv) != "" || os.Getenv(wtc.DevBuildDisableEnv) == "off" {
		return
	}
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	build := wtc.ReadDevBuild(wtc.FindCollectionRoot(dir))
	if build == nil {
		return
	}
	if self, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(self); err == nil && resolved == build.Binary {
			return
		}
	}
	if !wtc.DevBuildTakesOver(version, build) {
		if term.IsTerminal(os.Stderr.Fd()) {
			fmt.Fprintf(os.Stderr, "wtc: dev build %s is older than %s; using %s (wtc dev install to rebuild)\n", build.Version, version, version)
		}
		return
	}
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, wtc.DevBuildDelegatedEnv+"=") {
			env = append(env, entry)
		}
	}
	env = append(env, wtc.DevBuildDelegatedEnv+"="+version)
	// On success Exec does not return; on failure keep running this binary.
	if err := syscall.Exec(build.Binary, append([]string{build.Binary}, os.Args[1:]...), env); err != nil {
		fmt.Fprintf(os.Stderr, "wtc: cannot run dev build %s: %v\n", build.Binary, err)
	}
}

// versionLabel is what --version prints: a dev build standing in for another
// version says so.
func versionLabel() string {
	if from := os.Getenv(wtc.DevBuildDelegatedEnv); from != "" {
		return version + " (dev build, in place of " + from + ")"
	}
	return version
}

func addDevCommands(root *cobra.Command, asJSON *bool) {
	dev := &cobra.Command{Use: "dev", Short: "Run a collection-local build of wtc in place of the pinned release",
		Long: "wtc dev install builds the wtc-cli checkout in this collection into .wtc/dev/wtc.\n" +
			"Any wtc started inside the collection then hands over to that build while it is the same version\n" +
			"or newer, and runs itself again once it is newer than the build. Set WTC_DEV_BUILD=off to skip\n" +
			"the handover for one command.", Args: cobra.NoArgs}
	collectionRoot := func() (string, error) {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root := wtc.FindCollectionRoot(dir)
		if root == "" {
			return "", fmt.Errorf("not inside a worktree collection")
		}
		return root, nil
	}
	var source string
	install := &cobra.Command{Use: "install", Short: "Build the collection's wtc-cli checkout as its dev build", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := collectionRoot()
			if err != nil {
				return err
			}
			from := source
			if from == "" {
				if from, err = wtc.DevBuildSource(root); err != nil {
					return err
				}
			} else if from, err = filepath.Abs(from); err != nil {
				return err
			}
			build, err := wtc.InstallDevBuild(root, from)
			if err != nil {
				return err
			}
			return emit(envelope{OK: true, Data: build, Summary: fmt.Sprintf("installed dev build %s from %s\n%s", build.Version, build.Source, devBuildEffect(build))}, *asJSON)
		}}
	install.Flags().StringVar(&source, "source", "", "wtc-cli checkout to build (default: the collection's wtc-cli sibling)")
	status := &cobra.Command{Use: "status", Short: "Show which wtc runs in this collection", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := collectionRoot()
			if err != nil {
				return err
			}
			build := wtc.ReadDevBuild(root)
			data := map[string]any{"running": version, "delegated_from": os.Getenv(wtc.DevBuildDelegatedEnv), "dev_build": build}
			summary := "no dev build in " + root
			if build != nil {
				summary = fmt.Sprintf("dev build %s from %s\n%s", build.Version, build.Source, devBuildEffect(build))
			}
			return emit(envelope{OK: true, Data: data, Summary: summary}, *asJSON)
		}}
	uninstall := &cobra.Command{Use: "uninstall", Short: "Remove the collection's dev build", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := collectionRoot()
			if err != nil {
				return err
			}
			removed, err := wtc.UninstallDevBuild(root)
			if err != nil {
				return err
			}
			summary := "no dev build to remove"
			if removed {
				summary = "removed the dev build; wtc runs the pinned or ambient release here again"
			}
			return emit(envelope{OK: true, Data: map[string]bool{"removed": removed}, Summary: summary}, *asJSON)
		}}
	dev.AddCommand(install, status, uninstall)
	root.AddCommand(dev)
}

func devBuildEffect(build *wtc.DevBuild) string {
	if from := os.Getenv(wtc.DevBuildDelegatedEnv); from != "" {
		return "in effect: running it in place of " + from
	}
	if version == build.Version {
		return "in effect: this is the dev build"
	}
	if wtc.DevBuildTakesOver(version, build) {
		return "in effect: wtc " + version + " hands over to it"
	}
	if _, err := os.Stat(build.Binary); err == nil && version == "dev" {
		return "this wtc is an unversioned build and does not hand over"
	}
	return "not in effect: wtc " + version + " is newer"
}
