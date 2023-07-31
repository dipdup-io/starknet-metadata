package storage

import (
	"context"
	"time"

	"github.com/dipdup-net/indexer-sdk/pkg/storage"
)

// IState -
type IState interface {
	storage.Table[*State]

	ByName(ctx context.Context, name string) (State, error)
}

// State -
type State struct {
	// nolint
	tableName struct{} `pg:"state" comment:"Table contains current indexer's state"`

	ID         uint64    `comment:"Unique internal identity"`
	Name       string    `pg:",unique:state_name" comment:"Indexer human-readable name"`
	LastHeight uint64    `pg:",use_zero" comment:"Last block height"`
	LastTime   time.Time `comment:"Time of last block"`
}

// TableName -
func (State) TableName() string {
	return "state"
}
