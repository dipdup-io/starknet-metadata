package storage

import (
	"context"
	"time"

	"github.com/dipdup-net/indexer-sdk/pkg/storage"
	"github.com/uptrace/bun"
)

// IState -
type IState interface {
	storage.Table[*State]

	ByName(ctx context.Context, name string) (State, error)
}

// State -
type State struct {
	bun.BaseModel `bun:"table:state" comment:"Table contains current indexer's state"`

	ID         uint64    `bun:"id,pk,autoincrement" comment:"Unique internal identity"`
	Name       string    `bun:"name,unique:state_name" comment:"Indexer human-readable name"`
	LastHeight uint64    `comment:"Last block height"`
	LastTime   time.Time `comment:"Time of last block"`
}

// TableName -
func (State) TableName() string {
	return "state"
}
