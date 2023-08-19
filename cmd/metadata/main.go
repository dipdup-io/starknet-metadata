package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	ipfs "github.com/dipdup-io/ipfs-tools"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc"
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-io/starknet-metadata/internal/storage/postgres"
	"github.com/dipdup-net/go-lib/hasura"
	"github.com/dipdup-net/indexer-sdk/pkg/modules"
	grpcSDK "github.com/dipdup-net/indexer-sdk/pkg/modules/grpc"
	"github.com/dipdup-net/indexer-sdk/pkg/modules/printer"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "metadata",
		Short: "DipDup metadata indexer for Starknet",
	}
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "2006-01-02 15:04:05",
	}).Level(zerolog.InfoLevel)

	configPath := rootCmd.PersistentFlags().StringP("config", "c", "dipdup.yml", "path to YAML config file")
	if err := rootCmd.Execute(); err != nil {
		log.Panic().Err(err).Msg("command line execute")
		return
	}
	if err := rootCmd.MarkFlagRequired("config"); err != nil {
		log.Panic().Err(err).Msg("config command line arg is required")
		return
	}

	cfg, err := Load(*configPath)
	if err != nil {
		log.Err(err).Msg("")
		return
	}
	runtime.GOMAXPROCS(cfg.Metadata.MaxCPU)

	if cfg.LogLevel == "" {
		cfg.LogLevel = zerolog.LevelInfoValue
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Panic().Err(err).Msg("parsing log level")
		return
	}
	zerolog.SetGlobalLevel(logLevel)
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
		return file + ":" + strconv.Itoa(line)
	}
	log.Logger = log.Logger.With().Caller().Logger()

	ctx, cancel := context.WithCancel(context.Background())

	pg, err := postgres.Create(ctx, cfg.Database)
	if err != nil {
		log.Panic().Err(err).Msg("database creation")
		return
	}

	views, err := createViews(ctx, pg)
	if err != nil {
		log.Panic().Err(err).Msg("create views")
		return
	}

	if cfg.Hasura != nil {
		if err := hasura.Create(ctx, hasura.GenerateArgs{
			Config:         cfg.Hasura,
			DatabaseConfig: cfg.Database,
			Views:          views,
			Models:         []any{new(storage.State), new(storage.Address), new(storage.TokenMetadata)},
		}); err != nil {
			log.Err(err).Msg("hasura.Create")
		}
	}

	ipfsNode, err := ipfs.NewNode(ctx, cfg.Metadata.IPFS.Dir, 1024*1024, cfg.Metadata.IPFS.Blacklist, cfg.Metadata.IPFS.Providers)
	if err != nil {
		log.Err(err).Msg("ipfs.NewNode")
		return
	}

	if err := ipfsNode.Start(ctx); err != nil {
		log.Err(err).Msg("ipfs.Start")
		return
	}

	client := grpc.NewClient(*cfg.GRPC)
	indexer, err := NewIndexer(cfg.Metadata, cfg.DataSources, pg, client, ipfsNode)
	if err != nil {
		log.Err(err).Msg("create indexer")
		return
	}

	if err := modules.Connect(client, indexer, grpc.OutputMessages, printer.InputName); err != nil {
		log.Panic().Err(err).Msg("module connect")
		return
	}

	log.Info().Msg("connecting to gRPC...")
	if err := client.Connect(ctx,
		grpcSDK.WaitServer(),
		grpcSDK.WithUserAgent("starknet-metadata"),
	); err != nil {
		log.Panic().Err(err).Msg("grpc connect")
		return
	}
	log.Info().Msg("connected")

	indexer.Start(ctx)

	if err := indexer.Subscribe(ctx, cfg.GRPC.Subscriptions); err != nil {
		log.Panic().Err(err).Msg("subscribe")
		return
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	<-signals

	if err := indexer.Unsubscribe(ctx); err != nil {
		log.Panic().Err(err).Msg("unsubscribe")
		return
	}

	cancel()

	if err := indexer.Close(); err != nil {
		log.Panic().Err(err).Msg("closing indexer")
	}

	if err := client.Close(); err != nil {
		log.Panic().Err(err).Msg("closing grpc server")
	}
	if err := pg.Storage.Close(); err != nil {
		log.Panic().Err(err).Msg("closing database connection")
	}
	if err := ipfsNode.Close(); err != nil {
		log.Err(err).Msgf("ipfsNode.Close()")
	}

	close(signals)
}
