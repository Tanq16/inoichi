package cmd

import (
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var AppVersion = "dev-build"

var debugFlag bool

var rootCmd = &cobra.Command{
	Use:               "inoichi",
	Short:             "A local-first mind mapping editor served from one binary",
	Version:           AppVersion,
	CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
}

func Execute() {
	rootCmd.SetVersionTemplate("inoichi {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "log at debug level instead of info")
	cobra.OnInitialize(setupLogs)
}

func setupLogs() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	var out io.Writer = os.Stdout
	if term.IsTerminal(os.Stdout.Fd()) {
		out = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.DateTime}
	}
	log.Logger = zerolog.New(out).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debugFlag {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}
