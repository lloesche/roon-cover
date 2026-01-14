package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const envPrefix = "ROON_COVER"

type rootFlags struct {
	ConfigPath string

	LogFormat string
	LogLevel  string

	CoreName string
	ZoneName string

	PprofAddr           string
	DownloadToTemp      bool
	Window              bool
	DisplayIndex        int
	FadeMS              int
	Ease                string
	DisplaySleepCmd     string
	DisplayWakeCmd      string
	DisplaySleepIdleSec int

	ShowTitle  bool
	ShowArtist bool
	ShowAlbum  bool
	ShowZone   bool
	ShowAll    bool

	FontPath   string
	FontSize   int
	FontFadeMS int
}

func newRootCmd(ctx context.Context) *cobra.Command {
	var flags rootFlags

	cmd := &cobra.Command{
		Use:           "roon-cover",
		Short:         "Display Roon cover art for a selected zone",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initConfig(flags.ConfigPath); err != nil {
				return err
			}
			if err := initLogging(flags.LogFormat, flags.LogLevel); err != nil {
				return err
			}

			cmd.SetContext(withLogger(cmd.Context(), logger()))
			if flags.PprofAddr != "" {
				if err := startPprof(cmd.Context(), flags.PprofAddr, logger()); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is provided:
			// run kiosk/display mode for the configured zone.
			return runKiosk(cmd)
		},
	}

	cmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "config file path (optional)")
	cmd.PersistentFlags().StringVar(&flags.LogFormat, "log-format", "text", "log format: text|json")
	cmd.PersistentFlags().StringVar(&flags.LogLevel, "log-level", "info", "log level: debug|info|warn|error")
	cmd.PersistentFlags().StringVar(&flags.CoreName, "roon-core", "", "Roon Core name (or set via ROON_COVER_ROON_CORE)")
	cmd.PersistentFlags().StringVar(&flags.ZoneName, "roon-zone", "", "target zone name (optional; if omitted uses the first available zone)")
	cmd.PersistentFlags().StringVar(&flags.PprofAddr, "pprof-addr", "", "start pprof server on addr (e.g. 127.0.0.1:6060)")
	cmd.PersistentFlags().BoolVar(&flags.DownloadToTemp, "download-to-temp", false, "download cover art into the OS temp directory and log the file path")
	cmd.PersistentFlags().BoolVar(&flags.Window, "window", false, "run windowed (800x800) instead of fullscreen")
	cmd.PersistentFlags().IntVar(&flags.DisplayIndex, "display", 0, "SDL display index to show on (0-based)")
	cmd.PersistentFlags().IntVar(&flags.FadeMS, "fade-ms", 500, "crossfade duration in ms when cover changes (0 disables)")
	cmd.PersistentFlags().StringVar(&flags.Ease, "ease", "in-out-sine", "easing function for fades (e.g. in-sine, out-sine, in-out-sine, in-quad, out-cubic, out-expo, in-circ, out-elastic, out-bounce)")
	cmd.PersistentFlags().StringVar(&flags.DisplaySleepCmd, "display-sleep-cmd", "", "command to run when display should sleep (optional)")
	cmd.PersistentFlags().StringVar(&flags.DisplayWakeCmd, "display-wake-cmd", "", "command to run when display should wake (optional)")
	cmd.PersistentFlags().IntVar(&flags.DisplaySleepIdleSec, "display-sleep-idle-sec", 0, "seconds of no playback before running --display-sleep-cmd (0 disables)")

	cmd.PersistentFlags().BoolVar(&flags.ShowTitle, "show-title", false, "show track title overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowArtist, "show-artist", false, "show artist overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowAlbum, "show-album", false, "show album overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowZone, "show-zone", false, "show zone name overlay (top-left; auto-hides after 5s)")
	cmd.PersistentFlags().BoolVar(&flags.ShowAll, "show-all", false, "show title+artist+album+zone overlays")

	cmd.PersistentFlags().StringVar(&flags.FontPath, "font", "", "path to a .ttf font file (optional; default is OS-specific)")
	cmd.PersistentFlags().IntVar(&flags.FontSize, "font-size", 28, "font size in points for overlays")
	cmd.PersistentFlags().IntVar(&flags.FontFadeMS, "font-fade-ms", 200, "text fade duration in ms for overlays (0 disables; independent of cover fade)")

	// Current config keys:
	// - roon.core
	// - roon.zone
	_ = viper.BindPFlag("roon.core", cmd.PersistentFlags().Lookup("roon-core"))
	_ = viper.BindPFlag("roon.zone", cmd.PersistentFlags().Lookup("roon-zone"))
	_ = viper.BindPFlag("download.to_temp", cmd.PersistentFlags().Lookup("download-to-temp"))
	_ = viper.BindPFlag("display.window", cmd.PersistentFlags().Lookup("window"))
	_ = viper.BindPFlag("display.index", cmd.PersistentFlags().Lookup("display"))
	_ = viper.BindPFlag("display.fade_ms", cmd.PersistentFlags().Lookup("fade-ms"))
	_ = viper.BindPFlag("display.ease", cmd.PersistentFlags().Lookup("ease"))
	_ = viper.BindPFlag("display.sleep_cmd", cmd.PersistentFlags().Lookup("display-sleep-cmd"))
	_ = viper.BindPFlag("display.wake_cmd", cmd.PersistentFlags().Lookup("display-wake-cmd"))
	_ = viper.BindPFlag("display.sleep_idle_sec", cmd.PersistentFlags().Lookup("display-sleep-idle-sec"))
	_ = viper.BindPFlag("display.show_title", cmd.PersistentFlags().Lookup("show-title"))
	_ = viper.BindPFlag("display.show_artist", cmd.PersistentFlags().Lookup("show-artist"))
	_ = viper.BindPFlag("display.show_album", cmd.PersistentFlags().Lookup("show-album"))
	_ = viper.BindPFlag("display.show_zone", cmd.PersistentFlags().Lookup("show-zone"))
	_ = viper.BindPFlag("display.show_all", cmd.PersistentFlags().Lookup("show-all"))
	_ = viper.BindPFlag("display.font", cmd.PersistentFlags().Lookup("font"))
	_ = viper.BindPFlag("display.font_size", cmd.PersistentFlags().Lookup("font-size"))
	_ = viper.BindPFlag("display.font_fade_ms", cmd.PersistentFlags().Lookup("font-fade-ms"))

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newRoonCmd())
	cmd.AddCommand(newDisplayCmd())

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.SetContext(ctx)
	return cmd
}

func initConfig(configPath string) error {
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("roon.core", "")
	viper.SetDefault("roon.zone", "")
	viper.SetDefault("download.to_temp", false)
	viper.SetDefault("display.window", false)
	viper.SetDefault("display.index", 0)
	viper.SetDefault("display.fade_ms", 500)
	viper.SetDefault("display.ease", "in-out-sine")
	viper.SetDefault("display.sleep_cmd", "")
	viper.SetDefault("display.wake_cmd", "")
	viper.SetDefault("display.sleep_idle_sec", 0)
	viper.SetDefault("display.show_title", false)
	viper.SetDefault("display.show_artist", false)
	viper.SetDefault("display.show_album", false)
	viper.SetDefault("display.show_zone", false)
	viper.SetDefault("display.show_all", false)
	viper.SetDefault("display.font", "")
	viper.SetDefault("display.font_size", 28)
	viper.SetDefault("display.font_fade_ms", 200)

	if configPath == "" {
		return nil
	}

	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if info.IsDir() {
		return errors.New("config: path is a directory")
	}

	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Make relative paths in config behave relative to config dir (future-proofing).
	viper.Set("config_dir", filepath.Dir(configPath))

	return nil
}
