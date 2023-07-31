package main

import (
	"context"
	"sync"
	"time"

	"github.com/dipdup-io/starknet-indexer/pkg/grpc/pb"
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-io/starknet-metadata/internal/storage/postgres"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Channel -
type Channel struct {
	state   *storage.State
	storage postgres.Storage
	failed  bool
	ch      chan *pb.Subscription
	wg      *sync.WaitGroup
}

// NewChannel -
func NewChannel(pg postgres.Storage, state *storage.State) Channel {
	ch := Channel{
		storage: pg,
		state:   state,
		ch:      make(chan *pb.Subscription, 1024*1024),
		wg:      new(sync.WaitGroup),
	}

	return ch
}

// AddEvent -
func (channel Channel) Add(msg *pb.Subscription) {
	channel.ch <- msg
}

func (channel Channel) Start(ctx context.Context) {
	channel.wg.Add(1)
	go channel.listen(ctx)
}

// IsEmpty -
func (channel Channel) IsEmpty() bool {
	return len(channel.ch) == 0
}

func (channel Channel) listen(ctx context.Context) {
	defer channel.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-channel.ch:
			if channel.failed {
				continue
			}
			response := msg.GetResponse()
			switch {
			case msg.EndOfBlock != nil:
				log.Info().
					Uint64("subscription", response.GetId()).
					Uint64("height", msg.EndOfBlock.Height).
					Msg("end of block")

				channel.state.LastHeight = msg.EndOfBlock.Height
				channel.state.LastTime = time.Now()

				if err := channel.storage.State.Update(ctx, channel.state); err != nil {
					log.Err(err).Msg("state updating")
				}

			case msg.Token != nil:

				if err := channel.parseToken(ctx, msg.Token); err != nil {
					log.Err(err).Msg("token parsing")
				}

				log.Debug().
					Str("token_type", msg.Token.Type).
					Str("token_id", msg.Token.TokenId).
					Uint64("id", msg.Token.Id).
					Uint64("subscription", msg.Response.Id).
					Msg("new token")

			}
		}
	}
}

// Close -
func (channel Channel) Close() error {
	channel.wg.Wait()

	close(channel.ch)
	return nil
}

// State -
func (channel Channel) State() *storage.State {
	return channel.state
}

func (channel Channel) parseToken(ctx context.Context, token *pb.Token) error {
	tokenMetadata := storage.TokenMetadata{
		Id:      token.Id,
		TokenId: decimal.RequireFromString(token.TokenId),
		Status:  storage.StatusNew,
		Type:    storage.TokenType(token.Type),
	}

	if token.Contract != nil {
		tokenMetadata.ContractID = token.Contract.Id
		tokenMetadata.Contract = storage.Address{
			ID:   token.Contract.Id,
			Hash: token.Contract.Hash,
		}
	}

	return channel.saveToken(ctx, tokenMetadata)
}

func (channel Channel) saveToken(ctx context.Context, metadata storage.TokenMetadata) error {
	tx, err := channel.storage.Transactable.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Close(ctx)

	if err := tx.Add(ctx, &metadata); err != nil {
		return tx.HandleError(ctx, err)
	}

	if metadata.ContractID > 0 {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO address (id, hash) VALUES (?,?) ON CONFLICT (id) DO NOTHING`,
			metadata.Contract.ID,
			metadata.Contract.Hash,
		); err != nil {
			return tx.HandleError(ctx, err)

		}
	}

	if err := tx.Flush(ctx); err != nil {
		return tx.HandleError(ctx, err)
	}

	return nil
}
