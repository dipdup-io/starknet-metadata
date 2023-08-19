package storage

import (
	"github.com/dipdup-net/indexer-sdk/pkg/storage"
	"github.com/uptrace/bun"
)

// IAddress -
type IAddress interface {
	storage.Table[*Address]
}

// Address -
type Address struct {
	bun.BaseModel `bun:"table:address" comment:"Table with starknet addresses."`

	ID   uint64 `bun:"id,type:bigint,pk,notnull" comment:"Unique internal identity"`
	Hash []byte `bun:",unique:address_hash" comment:"Address hash."`
}

// TableName -
func (Address) TableName() string {
	return "address"
}
