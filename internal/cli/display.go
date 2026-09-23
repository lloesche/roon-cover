package cli

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

	windowed := viper.GetBool("display.window")
	if windowed {
		disp.Width = 800
		disp.Height = 800
		disp.Fullscreen = false
	} else {
		disp.Fullscreen = true
	}
	disp.DisplayIndex = viper.GetInt("display.index")
	disp.InfoCh = infoCh
	disp.EventCh = eventCh
	disp.FadeMS = viper.GetInt("display.fade_ms")
	disp.Ease = viper.GetString("display.ease")
	disp.FontPath = viper.GetString("display.font")
	disp.FontSize = viper.GetInt("display.font_size")
	disp.FontFadeMS = viper.GetInt("display.font_fade_ms")

	showAll := viper.GetBool("display.show_all")
	disp.ShowTitle = showAll || viper.GetBool("display.show_title")
	disp.ShowArtist = showAll || viper.GetBool("display.show_artist")
	disp.ShowAlbum = showAll || viper.GetBool("display.show_album")
	disp.ShowZone = showAll || viper.GetBool("display.show_zone")

	sleepIdleSec := viper.GetInt("display.sleep_idle_sec")
	if sleepIdleSec < 0 {
		return fmt.Errorf("--display-sleep-idle-sec must be >= 0 (got %d)", sleepIdleSec)
	}
	powerCtl := displaypower.CommandController{
		SleepCmd: viper.GetString("display.sleep_cmd"),
		WakeCmd:  viper.GetString("display.wake_cmd"),
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
	if disp.FontFadeMS < 0 {
		return fmt.Errorf("--font-fade-ms must be >= 0 (got %d)", disp.FontFadeMS)
	}

	client := roon.NewClient(roon.Config{DisplayName: "roon-cover"}, roon.WithLogger(l))
	core, err := ensureCoreAndPaired(cmd, client)
	if err != nil {
		return err
	}
	zones, err := client.GetZones(ctx, core)
	if err != nil {
		return err
	}
	controller := app.Controller{Source: client, Core: core, Log: l, Options: app.Options{Zone: viper.GetString("roon.zone"), SleepAfter: time.Duration(sleepIdleSec) * time.Second}, Power: func(ctx context.Context, sleep bool) error {
		if sleep {
			return powerCtl.Sleep(ctx)
		}
		return powerCtl.Wake(ctx)
	}}
	if viper.GetBool("download.to_temp") {
		controller.Options.SaveArtwork = func(zone string, data []byte, mime string) { writeCoverBytesToTemp(l, zone, data, mime) }
	}
	if err := controller.Initialize(zones); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- controller.Run(ctx, updates, infoCh, eventCh) }()
	renderErr := disp.Run(ctx, updates)
	cancel()
	if renderErr != nil {
		return renderErr
	}
	return <-done
}
