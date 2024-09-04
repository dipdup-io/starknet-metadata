package migrations

import (
	"context"
	"fmt"
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
)

func init() {
	DbMigrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		var erc20Tokens []storage.TokenMetadata
		err := db.NewSelect().
			Model(&erc20Tokens).
			Where("type = ?", storage.TokenTypeERC20).
			Scan(ctx)

		if err != nil {
			fmt.Println("Error:", err)
		}

		for _, token := range erc20Tokens {
			if token.Metadata == nil {
				continue
			}
			unicodeDecimals, ok := token.Metadata["decimals"].(string)
			if !ok || unicodeDecimals == "" {
				continue
			}
			runeDecimals := []rune(unicodeDecimals)[0]
			token.Metadata["decimals"] = int(runeDecimals)
			_, err = db.NewUpdate().
				Model(&token).
				Column("metadata").
				Where("id = ?", token.Id).
				Exec(ctx)
			if err != nil {
				log.Err(err).Uint64("token id", token.Id).Msg("error updating token")
			}
		}
		log.Info().
			Int("updated erc20 tokens amount", len(erc20Tokens)).
			Msg("migration applied")
		return nil

	}, func(ctx context.Context, db *bun.DB) error {
		return nil
	})
}
