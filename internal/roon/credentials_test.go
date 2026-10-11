package roon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileCredentialStoreCreatesDirectoryOnlyWhenSaving(t *testing.T) {
	dir := t.TempDir()
	// os.UserConfigDir uses different variables on Windows, Linux, and macOS.
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	store, err := NewFileCredentialStore("roon-cover")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	core := Core{ID: "test-core"}
	if _, found, err := store.Load(ctx, core); err != nil || found {
		t.Fatalf("loading an absent store: found=%t, error=%v", found, err)
	}
	path := store.(*fileCredentialStore).Path()
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("creating or loading the store created its directory: %v", err)
	}
	if err := store.Save(ctx, core, Credentials{RegistryToken: "token", PairedCoreID: core.ID}); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load(ctx, core)
	if err != nil || !found || got.RegistryToken != "token" || got.CoreKey != string(core.ID) || got.CreatedAt.IsZero() {
		t.Fatalf("first save did not create a complete store: found=%t, error=%v", found, err)
	}
}
