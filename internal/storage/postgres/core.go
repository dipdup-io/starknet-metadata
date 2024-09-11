package postgres

import (
	"context"
	"database/sql"

	"github.com/dipdup-io/starknet-metadata/internal/storage/postgres/migrations"
	"github.com/uptrace/bun/migrate"

	models "github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/config"
	"github.com/dipdup-net/go-lib/database"
	"github.com/dipdup-net/indexer-sdk/pkg/storage/postgres"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
)

// Storage -
type Storage struct {
	*postgres.Storage

	Address       models.IAddress
	TokenMetadata models.ITokenMetadata
	State         models.IState
	Tx            Transaction
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

func initDatabase(ctx context.Context, conn *database.Bun) error {
	if err := createTypes(ctx, conn); err != nil {
		return err
	}

	for _, data := range models.Models {
		if _, err := conn.DB().NewCreateTable().IfNotExists().Model(data).Exec(ctx); err != nil {
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

	if err := applyMigrations(ctx, conn); err != nil {
		return err
	}

	if err := setTokenMetadataLastUpdateID(ctx, conn); err != nil {
		return err
	}

	return createIndices(ctx, conn)
}

func createIndices(ctx context.Context, conn *database.Bun) error {
	log.Info().Msg("creating indexes...")
	return conn.DB().RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewCreateIndex().
			IfNotExists().
			Model((*models.TokenMetadata)(nil)).
			Index("tm_created_at_idx").
			Column("created_at").
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewCreateIndex().
			IfNotExists().
			Model((*models.TokenMetadata)(nil)).
			Index("tm_updated_at_idx").
			Column("updated_at").
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewCreateIndex().
			IfNotExists().
			Model((*models.TokenMetadata)(nil)).
			Index("tm_attempts_idx").
			Column("attempts").
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewCreateIndex().
			IfNotExists().
			Model((*models.TokenMetadata)(nil)).
			Index("tm_status_idx").
			Column("status").
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewCreateIndex().
			IfNotExists().
			Model((*models.TokenMetadata)(nil)).
			Index("tm_contract_id_idx").
			Column("contract_id").
			Exec(ctx); err != nil {
			return err
		}

		return nil
	})
}

func createTypes(ctx context.Context, conn *database.Bun) error {
	log.Info().Msg("creating custom types...")
	return conn.DB().RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
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

func applyMigrations(ctx context.Context, conn *database.Bun) error {
	migrator := migrate.NewMigrator(conn.DB(), migrations.DbMigrations)
	if err := migrator.Init(ctx); err != nil {
		return err
	}
	_, err := migrator.Migrate(ctx)
	return err
}

func setTokenMetadataLastUpdateID(ctx context.Context, conn *database.Bun) error {
	tokenMetadata := new(models.TokenMetadata)
	err := conn.DB().NewSelect().
		Model(tokenMetadata).
		Order("update_id desc").
		Limit(1).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	models.SetLastUpdateID(tokenMetadata.UpdateID)
	return nil
}
