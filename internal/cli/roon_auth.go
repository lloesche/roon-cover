package cli

import (
	"context"
	"errors"
	"strings"

	"roon-cover/internal/display"
	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
)

func ensureCoreAndPaired(ctx context.Context, cmd *cobra.Command, client *roon.Client, status func(display.Status)) (roon.Core, error) {
	l := LoggerFromContext(ctx)

	coreName := strings.TrimSpace(configFor(cmd).GetString("roon.core"))
	core, err := resolveCore(ctx, client, coreName)
	if err != nil {
		return roon.Core{}, err
	}
	if status != nil {
		status(display.Status{Title: "Found " + core.Name, Detail: "Connecting to Roon…"})
	}

	store, err := roon.NewFileCredentialStore("roon-cover")
	if err != nil {
		return roon.Core{}, err
	}
	if ps, ok := store.(interface{ Path() string }); ok {
		l.Debug("credential store path", "path", ps.Path())
	}

	creds, ok, err := store.Load(ctx, core)
	if err != nil {
		return roon.Core{}, err
	}
	l.Debug("credentials loaded", "ok", ok, "core_key", creds.CoreKey, "paired_core_id", creds.PairedCoreID, "has_registry_token", creds.RegistryToken != "")

	needsPair := !ok || creds.PairedCoreID == "" || (core.ID != "" && creds.PairedCoreID != core.ID)
	if !needsPair {
		l.Info("already paired", "core", core.Name)
		return core, nil
	}

	if status == nil {
		return roon.Core{}, errors.New("Roon is not paired; open roon-cover to complete pairing in its window")
	}
	status(display.Status{Title: "Connect to " + core.Name, Detail: "In Roon, open Settings → Extensions", Hint: "Enable roon-cover to continue."})
	newCreds, err := client.Pair(ctx, core)
	if err != nil {
		return roon.Core{}, err
	}
	if newCreds.PairedCoreID == "" {
		return roon.Core{}, errors.New("pairing did not return a paired core id")
	}
	l.Info("pair returned", "core", core.Name, "paired_core_id", newCreds.PairedCoreID)
	if err := store.Save(ctx, core, newCreds); err != nil {
		return roon.Core{}, err
	}
	l.Info("paired and saved credentials", "core", core.Name)
	l.Debug("credentials saved", "core_key", newCreds.CoreKey, "paired_core_id", newCreds.PairedCoreID, "has_registry_token", newCreds.RegistryToken != "")
	return core, nil
}
