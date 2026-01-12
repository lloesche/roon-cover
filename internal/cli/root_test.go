package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInitConfig_NoConfigPath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := initConfig(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitConfig_LoadsFromEnv(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("ROON_COVER_ROON_ZONE", "Kitchen")

	if err := initConfig(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := viper.GetString("roon.zone"); got != "Kitchen" {
		t.Fatalf("zone mismatch: got=%q want=%q", got, "Kitchen")
	}
}

func TestInitConfig_LoadsFromFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "roon-cover.yaml")
	if err := os.WriteFile(cfgPath, []byte("roon:\n  zone: Living Room\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := initConfig(cfgPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := viper.GetString("roon.zone"); got != "Living Room" {
		t.Fatalf("zone mismatch: got=%q want=%q", got, "Living Room")
	}

	if got := viper.GetString("config_dir"); got != dir {
		t.Fatalf("config_dir mismatch: got=%q want=%q", got, dir)
	}
}
