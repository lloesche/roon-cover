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

	FontPath string
	FontSize int
}

func newRootCmd(ctx context.Context) *cobra.Command {
	var flags rootFlags
	cfg := viper.New()

	cmd := &cobra.Command{
		Use:           "roon-cover",
		Short:         "Display Roon cover art for a selected zone",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initConfig(cfg, flags.ConfigPath); err != nil {
				return err
			}
			if err := initLogging(cfg.GetString("log.format"), cfg.GetString("log.level")); err != nil {
				return err
			}

			if font := cfg.GetString("display.font"); font != "" && !filepath.IsAbs(font) && cfg.InConfig("display.font") && !cmd.Root().PersistentFlags().Changed("font") {
				if _, overridden := os.LookupEnv("ROON_COVER_DISPLAY_FONT"); !overridden {
					cfg.Set("display.font", filepath.Join(cfg.GetString("config_dir"), font))
				}
			}
			cmd.SetContext(withLogger(cmd.Context(), logger()))
			if address := cfg.GetString("debug.pprof_addr"); address != "" {
				if err := startPprof(cmd.Context(), address, logger()); err != nil {
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
	cmd.PersistentFlags().StringVar(&flags.CoreName, "roon-core", "", "Roon Core name or host:port (bypasses discovery)")
	cmd.PersistentFlags().StringVar(&flags.ZoneName, "roon-zone", "", "target zone name (optional; at startup prefers the first playing zone, otherwise the first available)")
	cmd.PersistentFlags().StringVar(&flags.PprofAddr, "pprof-addr", "", "start pprof server on addr (e.g. 127.0.0.1:6060)")
	cmd.PersistentFlags().BoolVar(&flags.DownloadToTemp, "download-to-temp", false, "download cover art into the OS temp directory and log the file path")
	cmd.PersistentFlags().BoolVar(&flags.Window, "window", false, "run windowed (800x800) instead of fullscreen")
	cmd.PersistentFlags().IntVar(&flags.DisplayIndex, "display", 0, "monitor index to show on (0-based; console framebuffer mode exposes one monitor)")
	cmd.PersistentFlags().IntVar(&flags.FadeMS, "fade-ms", 500, "cover and text transition duration in ms (0 disables)")
	cmd.PersistentFlags().StringVar(&flags.Ease, "ease", "in-out-sine", "easing function for fades (e.g. in-sine, out-sine, in-out-sine, in-quad, out-cubic, out-expo, in-circ)")
	cmd.PersistentFlags().StringVar(&flags.DisplaySleepCmd, "display-sleep-cmd", "", "command to run when display should sleep (optional)")
	cmd.PersistentFlags().StringVar(&flags.DisplayWakeCmd, "display-wake-cmd", "", "command to run when display should wake (optional)")
	cmd.PersistentFlags().IntVar(&flags.DisplaySleepIdleSec, "display-sleep-idle-sec", 0, "seconds of no playback before running --display-sleep-cmd (0 disables)")

	cmd.PersistentFlags().BoolVar(&flags.ShowTitle, "show-title", false, "show track title overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowArtist, "show-artist", false, "show artist overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowAlbum, "show-album", false, "show album overlay")
	cmd.PersistentFlags().BoolVar(&flags.ShowZone, "show-zone", false, "show zone name overlay (top-left; auto-hides after 5s)")
	cmd.PersistentFlags().BoolVar(&flags.ShowAll, "show-all", false, "show title+artist+album+zone overlays")

	cmd.PersistentFlags().StringVar(&flags.FontPath, "font", "", "path to a .ttf font file (optional; default is OS-specific)")
	cmd.PersistentFlags().IntVar(&flags.FontSize, "font-size", 28, "font size in logical pixels for overlays")

	_ = cfg.BindPFlag("log.format", cmd.PersistentFlags().Lookup("log-format"))
	_ = cfg.BindPFlag("log.level", cmd.PersistentFlags().Lookup("log-level"))
	_ = cfg.BindPFlag("debug.pprof_addr", cmd.PersistentFlags().Lookup("pprof-addr"))
	// Current config keys:
	// - roon.core
	// - roon.zone
	_ = cfg.BindPFlag("roon.core", cmd.PersistentFlags().Lookup("roon-core"))
	_ = cfg.BindPFlag("roon.zone", cmd.PersistentFlags().Lookup("roon-zone"))
	_ = cfg.BindPFlag("download.to_temp", cmd.PersistentFlags().Lookup("download-to-temp"))
	_ = cfg.BindPFlag("display.window", cmd.PersistentFlags().Lookup("window"))
	_ = cfg.BindPFlag("display.index", cmd.PersistentFlags().Lookup("display"))
	_ = cfg.BindPFlag("display.fade_ms", cmd.PersistentFlags().Lookup("fade-ms"))
	_ = cfg.BindPFlag("display.ease", cmd.PersistentFlags().Lookup("ease"))
	_ = cfg.BindPFlag("display.sleep_cmd", cmd.PersistentFlags().Lookup("display-sleep-cmd"))
	_ = cfg.BindPFlag("display.wake_cmd", cmd.PersistentFlags().Lookup("display-wake-cmd"))
	_ = cfg.BindPFlag("display.sleep_idle_sec", cmd.PersistentFlags().Lookup("display-sleep-idle-sec"))
	_ = cfg.BindPFlag("display.show_title", cmd.PersistentFlags().Lookup("show-title"))
	_ = cfg.BindPFlag("display.show_artist", cmd.PersistentFlags().Lookup("show-artist"))
	_ = cfg.BindPFlag("display.show_album", cmd.PersistentFlags().Lookup("show-album"))
	_ = cfg.BindPFlag("display.show_zone", cmd.PersistentFlags().Lookup("show-zone"))
	_ = cfg.BindPFlag("display.show_all", cmd.PersistentFlags().Lookup("show-all"))
	_ = cfg.BindPFlag("display.font", cmd.PersistentFlags().Lookup("font"))
	_ = cfg.BindPFlag("display.font_size", cmd.PersistentFlags().Lookup("font-size"))

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newLicensesCmd())
	cmd.AddCommand(newRoonCmd())
	cmd.AddCommand(newDisplayCmd())

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.SetContext(context.WithValue(ctx, configKey{}, cfg))
	return cmd
}

func initConfig(cfg *viper.Viper, configPath string) error {
	cfg.SetEnvPrefix(envPrefix)
	cfg.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	cfg.AutomaticEnv()

	// Defaults
	cfg.SetDefault("roon.core", "")
	cfg.SetDefault("roon.zone", "")
	cfg.SetDefault("download.to_temp", false)
	cfg.SetDefault("display.window", false)
	cfg.SetDefault("display.index", 0)
	cfg.SetDefault("display.fade_ms", 500)
	cfg.SetDefault("display.ease", "in-out-sine")
	cfg.SetDefault("display.sleep_cmd", "")
	cfg.SetDefault("display.wake_cmd", "")
	cfg.SetDefault("display.sleep_idle_sec", 0)
	cfg.SetDefault("display.show_title", false)
	cfg.SetDefault("display.show_artist", false)
	cfg.SetDefault("display.show_album", false)
	cfg.SetDefault("display.show_zone", false)
	cfg.SetDefault("display.show_all", false)
	cfg.SetDefault("display.font", "")
	cfg.SetDefault("display.font_size", 28)

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

	cfg.SetConfigFile(configPath)
	if err := cfg.ReadInConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Make relative paths in config behave relative to config dir (future-proofing).
	cfg.Set("config_dir", filepath.Dir(configPath))

	return nil
}

type configKey struct{}

func configFor(cmd *cobra.Command) *viper.Viper {
	return cmd.Context().Value(configKey{}).(*viper.Viper)
}
