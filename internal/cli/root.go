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

	PprofAddr      string
	DownloadToTemp bool
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

			cmd.SetContext(withLogger(ctx, logger()))
			if flags.PprofAddr != "" {
				if err := startPprof(flags.PprofAddr, logger()); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is provided:
			// resolve core, pair/auth, then subscribe to the configured zone.
			return runDefault(cmd)
		},
	}

	cmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "config file path (optional)")
	cmd.PersistentFlags().StringVar(&flags.LogFormat, "log-format", "text", "log format: text|json")
	cmd.PersistentFlags().StringVar(&flags.LogLevel, "log-level", "info", "log level: debug|info|warn|error")
	cmd.PersistentFlags().StringVar(&flags.CoreName, "roon-core", "", "Roon Core name (or set via ROON_COVER_ROON_CORE)")
	cmd.PersistentFlags().StringVar(&flags.ZoneName, "roon-zone", "", "target zone name (or set via ROON_COVER_ROON_ZONE)")
	cmd.PersistentFlags().StringVar(&flags.PprofAddr, "pprof-addr", "", "start pprof server on addr (e.g. 127.0.0.1:6060)")
	cmd.PersistentFlags().BoolVar(&flags.DownloadToTemp, "download-to-temp", false, "download cover art into the OS temp directory and log the file path")

	// Current config keys:
	// - roon.core
	// - roon.zone
	_ = viper.BindPFlag("roon.core", cmd.PersistentFlags().Lookup("roon-core"))
	_ = viper.BindPFlag("roon.zone", cmd.PersistentFlags().Lookup("roon-zone"))
	_ = viper.BindPFlag("download.to_temp", cmd.PersistentFlags().Lookup("download-to-temp"))

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newRoonCmd())
	cmd.AddCommand(newDisplayCmd())

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

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
