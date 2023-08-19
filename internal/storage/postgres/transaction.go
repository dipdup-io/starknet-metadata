package postgres

import (
	"context"

	models "github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/indexer-sdk/pkg/storage"
)

// Transaction -
type Transaction struct {
	storage.Transaction
}

// BeginTransaction -
func BeginTransaction(ctx context.Context, tx storage.Transactable) (Transaction, error) {
	t, err := tx.BeginTransaction(ctx)
	return Transaction{t}, err
}

// SaveAddress -
func (t Transaction) SaveAddress(ctx context.Context, address *models.Address) error {
	_, err := t.Tx().NewInsert().Model(address).On("CONFLICT (id) DO NOTHING").Exec(ctx)
	return err
}
