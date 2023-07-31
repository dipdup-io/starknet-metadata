package postgres

import (
	"context"

	models "github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/config"
	"github.com/dipdup-net/go-lib/database"
	"github.com/dipdup-net/indexer-sdk/pkg/storage/postgres"
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// Storage -
type Storage struct {
	*postgres.Storage

	Address       models.IAddress
	TokenMetadata models.ITokenMetadata
	State         models.IState
}

// Create -
func Create(ctx context.Context, cfg config.Database) (Storage, error) {
	strg, err := postgres.Create(ctx, cfg, initDatabase)
	if err != nil {
		return Storage{}, err
	}

	s := Storage{
		Storage:       strg,
		Address:       NewAddress(strg.Connection()),
		State:         NewState(strg.Connection()),
		TokenMetadata: NewTokenMetadata(strg.Connection()),
	}

	return s, nil
}

func initDatabase(ctx context.Context, conn *database.PgGo) error {
	if err := createTypes(ctx, conn); err != nil {
		return err
	}

	for _, data := range models.Models {
		if err := conn.DB().WithContext(ctx).Model(data).CreateTable(&orm.CreateTableOptions{
			IfNotExists: true,
		}); err != nil {
			if err := conn.Close(); err != nil {
				return err
			}
			return err
		}
	}

	data := make([]any, len(models.Models))
	for i := range models.Models {
		data[i] = models.Models[i]
	}
	if err := database.MakeComments(ctx, conn, data...); err != nil {
		return errors.Wrap(err, "make comments")
	}

	return createIndices(ctx, conn)
}

func createIndices(ctx context.Context, conn *database.PgGo) error {
	log.Info().Msg("creating indexes...")
	return conn.DB().RunInTransaction(ctx, func(tx *pg.Tx) error {
		return nil
	})
}

func createTypes(ctx context.Context, conn *database.PgGo) error {
	log.Info().Msg("creating custom types...")
	return conn.DB().RunInTransaction(ctx, func(tx *pg.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`DO $$
			BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'token_type') THEN
					CREATE TYPE token_type AS ENUM ('erc20', 'erc721', 'erc1155');
				END IF;
			END$$;`,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`DO $$
			BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'status') THEN
					CREATE TYPE status AS ENUM ('new', 'filled', 'failed', 'success');
				END IF;
			END$$;`,
		); err != nil {
			return err
		}
		return nil
	})
}
