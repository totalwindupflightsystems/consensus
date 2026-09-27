package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wojons/consensus/internal/config"
)

func writeConfigWithPort(t *testing.T, path string, port int) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf("server:\n  port: %d\n", port)), 0o600); err != nil {
		t.Fatalf("write config %s: %v", path, err)
	}
}

func executeConfigLoadingCommand(t *testing.T, name, explicitPath string) (int, error) {
	t.Helper()

	previousInit, previousServer := InitFunc, ServerFunc
	t.Cleanup(func() {
		InitFunc, ServerFunc = previousInit, previousServer
	})

	var (
		observedPort int
		loadErr      error
	)
	loadConfig := func() error {
		var cfg config.Config
		cfg, loadErr = config.Load()
		if loadErr == nil {
			observedPort = cfg.Server.Port
		}
		return loadErr
	}
	InitFunc = func(string) error { return loadConfig() }
	ServerFunc = func() { _ = loadConfig() }

	args := []string{name}
	if explicitPath != "" {
		// Match the README quickstart form: the persistent flag follows the
		// subcommand rather than preceding it.
		args = append(args, "--config", explicitPath)
	}
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return 0, err
	}
	return observedPort, loadErr
}

func TestConfigPathDoesNotLeakAcrossCommandReuse(t *testing.T) {
	for _, command := range []string{"init", "serve"} {
		command := command
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			envPath := filepath.Join(dir, "env.yaml")
			explicitPath := filepath.Join(dir, "explicit.yaml")
			writeConfigWithPort(t, envPath, 4202)
			writeConfigWithPort(t, explicitPath, 4303)
			t.Setenv("CONSENSUS_CONFIG", envPath)
			t.Setenv("CONSENSUS_PORT", "")
			config.SetConfigPath("")
			t.Cleanup(func() { config.SetConfigPath("") })

			previousInit, previousServer := InitFunc, ServerFunc
			t.Cleanup(func() {
				InitFunc, ServerFunc = previousInit, previousServer
			})

			var observed []int
			loadConfig := func() error {
				cfg, err := config.Load()
				if err == nil {
					observed = append(observed, cfg.Server.Port)
				}
				return err
			}
			InitFunc = func(string) error { return loadConfig() }
			ServerFunc = func() { _ = loadConfig() }

			cmd := NewRootCommand()
			cmd.SetArgs([]string{command, "--config", explicitPath})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("first %s execution: %v", command, err)
			}
			cmd.SetArgs([]string{command})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("second %s execution: %v", command, err)
			}

			if len(observed) != 2 || observed[0] != 4303 || observed[1] != 4202 {
				t.Fatalf("ports loaded across command reuse = %v, want [4303 4202]", observed)
			}
		})
	}
}

func TestInitAndServeConfigPathPrecedence(t *testing.T) {
	for _, command := range []string{"init", "serve"} {
		command := command
		t.Run(command, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				explicitPath bool
				envPath      bool
				wantPort     int
			}{
				{name: "flag beats env and cwd", explicitPath: true, envPath: true, wantPort: 4303},
				{name: "env beats cwd", envPath: true, wantPort: 4202},
				{name: "cwd file is fallback", wantPort: 4101},
			} {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					dir := t.TempDir()
					cwdPath := filepath.Join(dir, "consensus.yaml")
					envPath := filepath.Join(dir, "env.yaml")
					explicitPath := filepath.Join(dir, "explicit.yaml")
					writeConfigWithPort(t, cwdPath, 4101)
					writeConfigWithPort(t, envPath, 4202)
					writeConfigWithPort(t, explicitPath, 4303)

					t.Chdir(dir)
					t.Setenv("CONSENSUS_PORT", "")
					if tc.envPath {
						t.Setenv("CONSENSUS_CONFIG", envPath)
					} else {
						t.Setenv("CONSENSUS_CONFIG", "")
					}

					// Isolate the process-global config seam before and after every
					// command so one command execution cannot contaminate another.
					config.SetConfigPath("")
					t.Cleanup(func() { config.SetConfigPath("") })

					flagPath := ""
					if tc.explicitPath {
						flagPath = explicitPath
					}
					gotPort, err := executeConfigLoadingCommand(t, command, flagPath)
					if err != nil {
						t.Fatalf("execute consensus %s: %v", command, err)
					}
					if gotPort != tc.wantPort {
						t.Fatalf("config loaded by %s: port=%d, want %d", command, gotPort, tc.wantPort)
					}

					// The CLI override is scoped to the callback. Once it returns,
					// normal env > cwd resolution must be restored.
					if tc.explicitPath {
						cfg, err := config.Load()
						if err != nil {
							t.Fatalf("load after %s: %v", command, err)
						}
						if cfg.Server.Port != 4202 {
							t.Fatalf("config path leaked after %s: port=%d, want env fallback 4202", command, cfg.Server.Port)
						}
					}
				})
			}
		})
	}
}
