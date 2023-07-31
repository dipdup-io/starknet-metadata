package storage

import (
	"github.com/dipdup-net/indexer-sdk/pkg/storage"
)

// IAddress -
type IAddress interface {
	storage.Table[*Address]
}

// Address -
type Address struct {
	// nolint
	tableName struct{} `pg:"address" comment:"Table with starknet addresses."`

	ID   uint64 `pg:"id,type:bigint,pk,notnull" comment:"Unique internal identity"`
	Hash []byte `pg:",unique:address_hash" comment:"Address hash."`
}

// TableName -
func (Address) TableName() string {
	return "address"
}
