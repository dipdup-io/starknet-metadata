package postgres

import (
	"context"

	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/database"
	"github.com/dipdup-net/indexer-sdk/pkg/storage/postgres"
)

// TokenMetadata -
type TokenMetadata struct {
	*postgres.Table[*storage.TokenMetadata]
}

// NewTokenMetadata -
func NewTokenMetadata(db *database.Bun) *TokenMetadata {
	return &TokenMetadata{
		Table: postgres.NewTable[*storage.TokenMetadata](db),
	}
}

// GetByStatus -
func (tm *TokenMetadata) GetByStatus(ctx context.Context, status storage.Status, limit, offset, attempts, delay int) (response []storage.TokenMetadata, err error) {
	query := tm.DB().NewSelect().Model(&response).
		Where("status = ?", status).
		Relation("Contract").
		Order("attempts asc", "updated_at desc")

	if delay > 0 {
		query = query.
			Where("created_at < (extract(epoch from current_timestamp) - ? * attempts)", delay)
	}

	if limit < 1 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	if attempts > 0 {
		query.Where("attempts < ?", attempts)
	}
	err = query.Limit(limit).Offset(offset).Scan(ctx)
	return
}
