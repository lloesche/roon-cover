package cli

import (
	"context"
	"errors"
	"github.com/spf13/cobra"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
	"strings"
	"time"
)

func openCoreSession(ctx context.Context, cmd *cobra.Command, client *roon.Client, status func(display.Status)) (*roon.Session, error) {
	core, err := resolveCore(ctx, client, strings.TrimSpace(configFor(cmd).GetString("roon.core")))
	if err != nil {
		return nil, err
	}
	if status != nil {
		status(display.Status{Title: "found " + core.Name, Hold: 700 * time.Millisecond, AppendInline: true})
		status(display.Status{Title: "Connecting to " + core.Name + "…", Hold: 300 * time.Millisecond})
	}
	// Diagnostics fail promptly when authorization is required. The kiosk waits.
	sessionCtx, cancel := context.WithCancel(ctx)
	needApproval := false
	onAuthorization := func() {
		if status == nil {
			needApproval = true
			cancel()
			return
		}
		status(display.Status{Title: "Connect to " + core.Name, Detail: "In Roon, open Settings → Extensions", Hint: "Enable roon-cover to continue."})
	}
	session, err := client.OpenSession(sessionCtx, core, onAuthorization)
	if err != nil {
		cancel()
		if needApproval {
			return nil, errors.New("roon is not authorized; open roon-cover to enable it in Roon")
		}
		return nil, err
	}
	go func() { <-session.Done; cancel() }()
	return session, nil
}
