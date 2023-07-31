package postgres

import (
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/database"
	"github.com/dipdup-net/indexer-sdk/pkg/storage/postgres"
)

// Address -
type Address struct {
	*postgres.Table[*storage.Address]
}

// NewAddress -
func NewAddress(db *database.PgGo) *Address {
	return &Address{
		Table: postgres.NewTable[*storage.Address](db),
	}
}
