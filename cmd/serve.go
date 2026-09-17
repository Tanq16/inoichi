package cmd

import (
	"cmp"
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/tanq16/inoichi/internal/server"
	"github.com/tanq16/inoichi/internal/storage"
)

var serveFlags struct {
	host    string
	port    int
	dataDir string
	noSeed  bool
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the mind mapping editor over HTTP",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := storage.New(serveFlags.dataDir)
		if err != nil {
			log.Fatal().Err(err).Str("dir", serveFlags.dataDir).Msg("failed to open the data directory")
		}
		store.OnWriteError = func(id string, err error) {
			log.Error().Err(err).Str("id", id).Msg("failed to write a map to disk, will retry")
		}
		srv := server.New(serveFlags.host, serveFlags.port, AppVersion, store)
		if err := srv.Setup(); err != nil {
			log.Fatal().Err(err).Msg("failed to set up the server")
		}
		if !serveFlags.noSeed {
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

func init() {
	serveCmd.Flags().StringVar(&serveFlags.host, "host", cmp.Or(os.Getenv("INOICHI_HOST"), "127.0.0.1"), "address to bind")
	serveCmd.Flags().IntVarP(&serveFlags.port, "port", "p", envPort(), "port to listen on")
	serveCmd.Flags().StringVarP(&serveFlags.dataDir, "data-dir", "d", cmp.Or(os.Getenv("INOICHI_DATA_DIR"), "data"), "directory holding the map files")
	serveCmd.Flags().BoolVar(&serveFlags.noSeed, "no-sample", false, "skip writing the sample map into an empty data directory")
	rootCmd.AddCommand(serveCmd)
}

func envPort() int {
	if v, err := strconv.Atoi(os.Getenv("INOICHI_PORT")); err == nil && v > 0 && v < 65536 {
		return v
	}
	return 8080
}
