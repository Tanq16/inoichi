package cmd

import (
	"cmp"
	"context"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/inoichi/internal/server"
	"github.com/tanq16/inoichi/internal/storage"
)

var AppVersion = "dev-build"

var flags struct {
	debug   bool
	host    string
	port    int
	dataDir string
	noSeed  bool
}

var rootCmd = &cobra.Command{
	Use:               "inoichi",
	Short:             "A local-first mind mapping editor served from one binary",
	Version:           AppVersion,
	Args:              cobra.NoArgs,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		store, err := storage.New(flags.dataDir)
		if err != nil {
			log.Fatal().Err(err).Str("dir", flags.dataDir).Msg("failed to open the data directory")
		}
		store.OnWriteError = func(id string, err error) {
			log.Error().Err(err).Str("id", id).Msg("failed to write a map to disk, will retry")
		}
		srv := server.New(flags.host, flags.port, AppVersion, store)
		if err := srv.Setup(); err != nil {
			log.Fatal().Err(err).Msg("failed to set up the server")
		}
		if !flags.noSeed {
			if err := srv.SeedSample(); err != nil {
				log.Error().Err(err).Msg("failed to seed the sample map, continuing without it")
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		runErr := srv.Run(ctx)
		if err := store.Close(); err != nil {
			log.Error().Err(err).Msg("failed to write every map to disk on shutdown")
		}
		if runErr != nil {
			log.Fatal().Err(runErr).Msg("server stopped")
		}
	},
}

func Execute() {
	rootCmd.SetVersionTemplate("inoichi {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&flags.debug, "debug", false, "log at debug level instead of info")
	rootCmd.Flags().StringVar(&flags.host, "host", cmp.Or(os.Getenv("INOICHI_HOST"), "127.0.0.1"), "address to bind")
	rootCmd.Flags().IntVarP(&flags.port, "port", "p", envPort(), "port to listen on")
	rootCmd.Flags().StringVarP(&flags.dataDir, "data-dir", "d", cmp.Or(os.Getenv("INOICHI_DATA_DIR"), "data"), "directory holding the map files")
	rootCmd.Flags().BoolVar(&flags.noSeed, "no-sample", false, "skip writing the sample map into an empty data directory")
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
	if flags.debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

func envPort() int {
	if v, err := strconv.Atoi(os.Getenv("INOICHI_PORT")); err == nil && v > 0 && v < 65536 {
		return v
	}
	return 8080
}
