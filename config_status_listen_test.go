package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatusListenConfigHonorsExplicitEmpty(t *testing.T) {
	for _, tt := range []struct {
		name string
		toml string
		want string
	}{
		{name: "omitted", toml: "[server]\n", want: defaultStatusAddr},
		{name: "disabled", toml: "[server]\nstatus_listen = \"\"\n", want: ""},
		{name: "custom", toml: "[server]\nstatus_listen = \"127.0.0.1:8080\"\n", want: "127.0.0.1:8080"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			file, ok, err := loadBaseConfigFile(path)
			if err != nil || !ok {
				t.Fatalf("load config: present=%v err=%v", ok, err)
			}
			cfg := defaultConfig()
			applyBaseConfig(&cfg, *file)
			if cfg.StatusAddr != tt.want {
				t.Fatalf("HTTP listener = %q, want %q", cfg.StatusAddr, tt.want)
			}

			// Rewriting the config must preserve an intentionally disabled listener.
			if err := rewriteConfigFile(path, cfg); err != nil {
				t.Fatal(err)
			}
			file, ok, err = loadBaseConfigFile(path)
			if err != nil || !ok {
				t.Fatalf("reload config: present=%v err=%v", ok, err)
			}
			reloaded := defaultConfig()
			applyBaseConfig(&reloaded, *file)
			if reloaded.StatusAddr != tt.want {
				t.Fatalf("rewritten HTTP listener = %q, want %q", reloaded.StatusAddr, tt.want)
			}
		})
	}
}
