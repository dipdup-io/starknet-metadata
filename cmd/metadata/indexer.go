package main

import (
	"context"
	"sync"
	"time"

	"github.com/dipdup-io/ipfs-tools"
	"github.com/dipdup-io/starknet-go-api/pkg/data"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc/pb"
	models "github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-io/starknet-metadata/internal/storage/postgres"
	"github.com/dipdup-net/go-lib/config"
	"github.com/dipdup-net/indexer-sdk/pkg/modules"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// variables
var (
	ZeroAddress = data.Felt("0x0").Bytes()
)

// input name
const (
	InputName  = "input"
	OutputName = "output"
)

const IndexerName = "Indexer"

// Indexer -
type Indexer struct {
	modules.BaseModule

	client       *grpc.Client
	storage      postgres.Storage
	state        *models.State
	subscription grpc.Subscription
	subId        uint64
	channel      Channel

	filler   Filler
	receiver *Receiver

	wg *sync.WaitGroup
}

// NewIndexer -
func NewIndexer(cfg Metadata, datasources map[string]config.DataSource, pg postgres.Storage, client *grpc.Client, ipfsNode *ipfs.Node) (*Indexer, error) {
	indexer := &Indexer{
		client:     client,
		storage:    pg,
		BaseModule: modules.New(IndexerName),
		state:      new(models.State),
		wg:         new(sync.WaitGroup),
	}
	indexer.channel = NewChannel(pg, indexer.state)
	filler, err := NewFiller(cfg.Filler, datasources, pg.TokenMetadata, client)
	if err != nil {
		return nil, err
	}
	indexer.filler = filler
	indexer.receiver = NewReceiver(cfg.Receiver, pg.TokenMetadata, ipfsNode)

	indexer.CreateInputWithCapacity(InputName, 1024*16)
	indexer.CreateOutput(OutputName)

	return indexer, nil
}

// Start -
func (indexer *Indexer) Start(ctx context.Context) {
	if err := indexer.init(ctx); err != nil {
		log.Err(err).Msg("state initialization")
		return
	}

	indexer.client.Start(ctx)

	indexer.wg.Add(1)
	go indexer.reconnectThread(ctx)

	indexer.wg.Add(1)
	go indexer.listen(ctx)

	indexer.filler.Start(ctx)
	indexer.receiver.Start(ctx)
}

// Subscribe -
func (indexer *Indexer) Subscribe(ctx context.Context, subscriptions map[string]grpc.Subscription) error {
	s, ok := subscriptions["metadata"]
	if !ok {
		return errors.Errorf("can't find subscription 'metadata'")
	}
	indexer.subscription = s

	indexer.channel.Start(ctx)

	if err := indexer.actualFilters(ctx, &s); err != nil {
		return errors.Wrap(err, "filters modifying")
	}

	log.Info().Msg("subscribing...")
	req := s.ToGrpcFilter()
	subId, err := indexer.client.Subscribe(ctx, req)
	if err != nil {
		return errors.Wrap(err, "subscribing error")
	}
	indexer.subId = subId
	return nil
}

func (indexer *Indexer) init(ctx context.Context) error {
	state, err := indexer.storage.State.ByName(ctx, indexer.Name())
	if err != nil {
		if !indexer.storage.State.IsNoRows(err) {
			return err
		}

		indexer.state.Name = indexer.Name()
		if err := indexer.storage.State.Save(ctx, indexer.state); err != nil {
			return err
		}
	} else {
		indexer.state = &state
	}

	indexer.channel.state = indexer.state
	return nil
}

func (indexer *Indexer) listen(ctx context.Context) {
	defer indexer.wg.Done()

	input, err := indexer.Input(InputName)
	if err != nil {
		log.Err(err).Msg("unknown input")
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("close listen thread")
			return

		case msg, ok := <-input.Listen():
			if !ok {
				return
			}

			switch typ := msg.(type) {
			case *pb.Subscription:
				indexer.channel.Add(typ)
			default:
				log.Info().Msgf("unknown message: %T", typ)
			}
		}
	}
}

func (indexer *Indexer) reconnectThread(ctx context.Context) {
	defer indexer.wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("close reconnect thread")
			return
		case subscriptionId, ok := <-indexer.client.Reconnect():
			if !ok {
				continue
			}

			if err := indexer.resubscribe(ctx, subscriptionId); err != nil {
				log.Err(err).Msg("resubscribe")
			}
		}
	}
}

func (indexer *Indexer) resubscribe(ctx context.Context, _ uint64) error {
	for !indexer.channel.IsEmpty() {
		select {
		case <-ctx.Done():
			return nil
		default:
			time.Sleep(time.Second)
		}
	}

	if err := indexer.actualFilters(ctx, &indexer.subscription); err != nil {
		return errors.Wrap(err, "filters modifying")
	}

	log.Info().Msg("resubscribing...")
	req := indexer.subscription.ToGrpcFilter()
	subId, err := indexer.client.Subscribe(ctx, req)
	if err != nil {
		return errors.Wrap(err, "resubscribing error")
	}
	indexer.subId = subId

	return nil
}

func (indexer *Indexer) actualFilters(ctx context.Context, sub *grpc.Subscription) error {
	if sub.TokenFilter != nil {
		lastId, err := indexer.storage.TokenMetadata.LastID(ctx)
		if err != nil {
			if indexer.storage.TokenMetadata.IsNoRows(err) {
				return nil
			}
			return err
		}

		log.Info().Uint64("last_id", lastId).Msg("receiving tokens...")

		for i := range sub.TokenFilter {
			sub.TokenFilter[i].Id = &grpc.IntegerFilter{
				Gt: lastId,
			}
		}
	}

	return nil
}

// Unsubscribe -
func (indexer *Indexer) Unsubscribe(ctx context.Context) error {
	log.Info().Uint64("id", indexer.subId).Msg("unsubscribing...")
	if err := indexer.client.Unsubscribe(ctx, indexer.subId); err != nil {
		return errors.Wrap(err, "unsubscribing")
	}
	return nil
}

// Close - gracefully stops module
func (indexer *Indexer) Close() error {
	indexer.wg.Wait()

	if err := indexer.receiver.Close(); err != nil {
		return err
	}

	if err := indexer.filler.Close(); err != nil {
		return err
	}

	if err := indexer.channel.Close(); err != nil {
		return err
	}

	return nil
}
