package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInitConfig_NoConfigPath(t *testing.T) {
	cfg := viper.New()

	if err := initConfig(cfg, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitConfig_LoadsFromEnv(t *testing.T) {
	cfg := viper.New()

	t.Setenv("ROON_COVER_ROON_ZONE", "Kitchen")

	if err := initConfig(cfg, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.GetString("roon.zone"); got != "Kitchen" {
		t.Fatalf("zone mismatch: got=%q want=%q", got, "Kitchen")
	}
}

func TestInitConfig_LoadsFromFile(t *testing.T) {
	cfg := viper.New()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "roon-cover.yaml")
	if err := os.WriteFile(cfgPath, []byte("roon:\n  zone: Living Room\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := initConfig(cfg, cfgPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.GetString("roon.zone"); got != "Living Room" {
		t.Fatalf("zone mismatch: got=%q want=%q", got, "Living Room")
	}

	if got := cfg.GetString("config_dir"); got != dir {
		t.Fatalf("config_dir mismatch: got=%q want=%q", got, dir)
	}
}

func TestCommandConfigurationIsIsolated(t *testing.T) {
	first := newRootCmd(context.Background())
	first.SetOut(io.Discard)
	first.SetArgs([]string{"version", "--roon-zone", "Kitchen"})
	if err := first.Execute(); err != nil {
		t.Fatal(err)
	}
	second := newRootCmd(context.Background())
	second.SetOut(io.Discard)
	second.SetArgs([]string{"version"})
	if err := second.Execute(); err != nil {
		t.Fatal(err)
	}
	if configFor(first).GetString("roon.zone") != "Kitchen" || configFor(second).GetString("roon.zone") != "" {
		t.Fatal("configuration leaked across commands")
	}
}
func TestConfigFontPathAndFlagPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("display:\n  font: fonts/custom.ttf\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd(context.Background())
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"version", "--config", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := configFor(cmd).GetString("display.font"); got != filepath.Join(dir, "fonts/custom.ttf") {
		t.Fatal(got)
	}
	cmd = newRootCmd(context.Background())
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"version", "--config", path, "--font", "flag.ttf"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := configFor(cmd).GetString("display.font"); got != "flag.ttf" {
		t.Fatal(got)
	}
}
