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
func NewTokenMetadata(db *database.PgGo) *TokenMetadata {
	return &TokenMetadata{
		Table: postgres.NewTable[*storage.TokenMetadata](db),
	}
}

// GetByHash -
func (tm *TokenMetadata) GetByHash(ctx context.Context, hash []byte) (tokenMetadata storage.TokenMetadata, err error) {
	err = tm.DB().ModelContext(ctx, &tokenMetadata).Where("hash = ?", hash).First()
	return
}

// GetByStatus -
func (tm *TokenMetadata) GetByStatus(ctx context.Context, status storage.Status, limit, offset, attempts, delay int) (response []storage.TokenMetadata, err error) {
	if delay < 0 {
		delay = 0
	}
	query := tm.DB().ModelContext(ctx, (*storage.TokenMetadata)(nil)).
		Where("status = ?", status).
		Where("created_at < (extract(epoch from current_timestamp) - ? * attempts)", delay).
		Relation("Contract").
		OrderExpr("attempts asc, updated_at desc")

	if limit < 1 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	if attempts > 0 {
		query.Where("attempts < ?", attempts)
	}
	err = query.Limit(limit).Offset(offset).Select(&response)
	return
}
