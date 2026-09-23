package cli

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"

	"roon-cover/internal/app"
	"roon-cover/internal/display"
	"roon-cover/internal/displaypower"
	"roon-cover/internal/roon"
	"time"
)

func newDisplayCmd() *cobra.Command {
	return &cobra.Command{Use: "display", Short: "Display Roon cover art", RunE: func(cmd *cobra.Command, args []string) error { return runKiosk(cmd) }}
}
func runKiosk(cmd *cobra.Command) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	l := LoggerFromContext(ctx)
	updates := make(chan display.Update, 1)
	infoCh := make(chan display.ScreenInfo, 1)
	eventCh := make(chan display.Event, 8)
	disp := &display.Window{
		Title: "roon-cover",
	}

	windowed := configFor(cmd).GetBool("display.window")
	if windowed {
		disp.Width = 800
		disp.Height = 800
		disp.Fullscreen = false
	} else {
		disp.Fullscreen = true
	}
	disp.DisplayIndex = configFor(cmd).GetInt("display.index")
	disp.InfoCh = infoCh
	disp.EventCh = eventCh
	disp.FadeMS = configFor(cmd).GetInt("display.fade_ms")
	disp.Ease = configFor(cmd).GetString("display.ease")
	disp.FontPath = configFor(cmd).GetString("display.font")
	disp.FontSize = configFor(cmd).GetInt("display.font_size")

	showAll := configFor(cmd).GetBool("display.show_all")
	disp.ShowTitle = showAll || configFor(cmd).GetBool("display.show_title")
	disp.ShowArtist = showAll || configFor(cmd).GetBool("display.show_artist")
	disp.ShowAlbum = showAll || configFor(cmd).GetBool("display.show_album")
	disp.ShowZone = showAll || configFor(cmd).GetBool("display.show_zone")

	sleepIdleSec := configFor(cmd).GetInt("display.sleep_idle_sec")
	if sleepIdleSec < 0 {
		return fmt.Errorf("--display-sleep-idle-sec must be >= 0 (got %d)", sleepIdleSec)
	}
	powerCtl := displaypower.CommandController{
		SleepCmd: configFor(cmd).GetString("display.sleep_cmd"),
		WakeCmd:  configFor(cmd).GetString("display.wake_cmd"),
		Timeout:  10 * time.Second,
	}

	if disp.FadeMS < 0 {
		return fmt.Errorf("--fade-ms must be >= 0 (got %d)", disp.FadeMS)
	}
	if _, err := display.FadeEasingByName(disp.Ease); err != nil {
		return err
	}

	if disp.FontSize < 6 || disp.FontSize > 256 {
		return fmt.Errorf("--font-size must be in [6,256] (got %d)", disp.FontSize)
	}

	done := make(chan error, 1)
	go func() {
		defer close(updates)
		var controller *app.Controller
		err := runStartup(ctx, updates, 5*time.Second, func(status func(display.Status)) error {
			client := roon.NewClient(roon.Config{DisplayName: "roon-cover"}, roon.WithLogger(l))
			core, err := ensureCoreAndPaired(ctx, cmd, client, status)
			if err != nil {
				return err
			}
			zones, err := client.GetZones(ctx, core)
			if err != nil {
				return err
			}
			status(display.Status{Title: "connected", Hold: 700 * time.Millisecond, AppendInline: true})
			status(display.Status{Title: "Choosing listening zone…", Hold: 300 * time.Millisecond})
			controller = &app.Controller{Source: client, Core: core, Log: l, Options: app.Options{Zone: configFor(cmd).GetString("roon.zone"), SleepAfter: time.Duration(sleepIdleSec) * time.Second}, Power: func(ctx context.Context, sleep bool) error {
				if sleep {
					return powerCtl.Sleep(ctx)
				}
				return powerCtl.Wake(ctx)
			}}
			if configFor(cmd).GetBool("download.to_temp") {
				controller.Options.SaveArtwork = func(zone string, data []byte, mime string) { writeCoverBytesToTemp(l, zone, data, mime) }
			}
			if err := controller.Initialize(zones); err != nil {
				return err
			}
			zone := controller.SelectedZone()
			status(display.Status{Title: zone.Name, Hold: 2 * time.Second, AppendInline: true})
			return nil
		})
		if err == nil && ctx.Err() == nil {
			display.Publish(updates, display.Update{})
			err = controller.Run(ctx, updates, infoCh, eventCh)
		}
		done <- err
	}()
	renderErr := disp.Run(ctx, updates)
	cancel()
	controllerErr := <-done
	if renderErr != nil {
		return renderErr
	}
	return controllerErr
}
