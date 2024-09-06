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
	query := tm.DB().NewSelect().
		Model((*storage.TokenMetadata)(nil)).
		Where("status = ?", status).
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
	query = query.Limit(limit).Offset(offset)

	err = tm.DB().NewSelect().
		TableExpr("(?) as token_metadata", query).
		ColumnExpr("token_metadata.*").
		ColumnExpr("address.hash as contract__hash").
		Join("left join address on contract_id = address.id").
		Scan(ctx, &response)

	return
}
