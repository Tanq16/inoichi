package cmd

import (
	"cmp"
	"os"
	"path/filepath"
	"strconv"

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
	Run: func(cmd *cobra.Command, args []string) {
		if serveFlags.dataDir == "" {
			log.Fatal().Msg("cannot resolve a home directory, pass --data-dir or set INOICHI_DATA_DIR")
		}
		store, err := storage.New(serveFlags.dataDir)
		if err != nil {
			log.Fatal().Err(err).Str("dir", serveFlags.dataDir).Msg("failed to open the data directory")
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
		if err := srv.Run(); err != nil {
			log.Fatal().Err(err).Msg("server stopped")
		}
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveFlags.host, "host", cmp.Or(os.Getenv("INOICHI_HOST"), "127.0.0.1"), "address to bind")
	serveCmd.Flags().IntVar(&serveFlags.port, "port", envPort(), "port to listen on")
	serveCmd.Flags().StringVar(&serveFlags.dataDir, "data-dir", cmp.Or(os.Getenv("INOICHI_DATA_DIR"), defaultDataDir()), "directory holding the map files")
	serveCmd.Flags().BoolVar(&serveFlags.noSeed, "no-sample", false, "skip writing the sample map into an empty data directory")
	rootCmd.AddCommand(serveCmd)
}

func envPort() int {
	if v, err := strconv.Atoi(os.Getenv("INOICHI_PORT")); err == nil && v > 0 && v < 65536 {
		return v
	}
	return 8080
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "inoichi")
}
