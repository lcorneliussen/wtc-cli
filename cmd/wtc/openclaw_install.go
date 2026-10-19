package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	openclawplugin "github.com/lcorneliussen/wtc-cli/openclaw-plugin"
	"github.com/spf13/cobra"
)

// The setting that lets the gateway load a plugin's Control UI.
const customPluginsKey = "gateway.controlUi.experimental.customPlugins"

func addOpenClawCommands(root *cobra.Command) {
	oc := &cobra.Command{Use: "openclaw", Short: "Set up an OpenClaw gateway for wtc", Args: cobra.NoArgs}
	var dir string
	var noConfig bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Install this wtc's OpenClaw plugin on the gateway: the collection card, the Collection panel and the agent's wtc skill",
		Long: `Run on the gateway host, where wtc and openclaw are on the PATH. The plugin is
embedded in this binary, so it is always the one that matches this wtc.

install turns on custom Control UI plugins (` + customPluginsKey + `,
skip with --no-config) and runs "openclaw plugins install <plugin> --force",
which also replaces an older wtc plugin. Restart the gateway, or reload the
Control UI, afterwards. With --dir it only writes the plugin there, for a
gateway on another host or in an image build.`,
		Args: cobra.NoArgs,
	}
	install.Flags().StringVar(&dir, "dir", "", "Write the plugin into this directory and install nothing")
	install.Flags().BoolVar(&noConfig, "no-config", false, "Leave "+customPluginsKey+" as it is")
	install.RunE = func(cmd *cobra.Command, args []string) error {
		if dir != "" {
			if err := writeOpenClawPlugin(dir, version); err != nil {
				return err
			}
			fmt.Printf("wrote the wtc %s OpenClaw plugin to %s\ninstall it on the gateway with: openclaw plugins install %s --force\n", versionLabel(), dir, dir)
			return nil
		}
		bin, err := exec.LookPath("openclaw")
		if err != nil {
			return errors.New("openclaw is not on the PATH: run this on the gateway host, or write the plugin with --dir and install it there")
		}
		tmp, err := os.MkdirTemp("", "wtc-openclaw-plugin-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		target := filepath.Join(tmp, "wtc")
		if err := writeOpenClawPlugin(target, version); err != nil {
			return err
		}
		run := func(args ...string) error {
			c := exec.Command(bin, args...)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("openclaw %s: %w", strings.Join(args, " "), err)
			}
			return nil
		}
		if !noConfig {
			if err := run("config", "set", customPluginsKey, "true"); err != nil {
				return err
			}
		}
		if err := run("plugins", "install", target, "--force"); err != nil {
			return err
		}
		fmt.Println("installed the wtc OpenClaw plugin. Restart the gateway if it does not show up, then see https://github.com/lcorneliussen/wtc-cli/blob/main/docs/openclaw.md#install for what each person connects")
		return nil
	}
	oc.AddCommand(install)
	root.AddCommand(oc)
}

var manifestVersion = regexp.MustCompile(`("version":\s*)"[^"]*"`)

// writeOpenClawPlugin writes the embedded plugin into dir, stamped with this
// binary's version so that the gateway lists which wtc it came from.
func writeOpenClawPlugin(dir, version string) error {
	stamp := strings.TrimPrefix(version, "v")
	return fs.WalkDir(openclawplugin.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := openclawplugin.Files.ReadFile(path)
		if err != nil {
			return err
		}
		if (path == "package.json" || path == "openclaw.plugin.json") && stamp != "dev" && stamp != "" {
			if at := manifestVersion.FindSubmatchIndex(data); at != nil {
				data = append(append(append([]byte{}, data[:at[3]]...), `"`+stamp+`"`...), data[at[1]:]...)
			}
		}
		return os.WriteFile(target, data, 0o644)
	})
}
