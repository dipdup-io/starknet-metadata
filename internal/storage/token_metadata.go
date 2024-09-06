package storage

import (
	"context"
	"time"

	"github.com/dipdup-io/starknet-metadata/internal/types"
	"github.com/dipdup-net/indexer-sdk/pkg/storage"
	"github.com/shopspring/decimal"
	"github.com/uptrace/bun"
)

// TokenUpdateID - incremental counter
var TokenUpdateID = types.NewCounter(0)

// ITokenMetadata -
type ITokenMetadata interface {
	storage.Table[*TokenMetadata]

	GetByStatus(ctx context.Context, status Status, limit, offset, attempts, delay int) ([]TokenMetadata, error)
}

// TokenMetadata -
type TokenMetadata struct {
	bun.BaseModel `bun:"table:token_metadata" comment:"Table contains token metadata"`

	Id         uint64          `bun:"id,notnull,type:bigint,pk" comment:"Unique internal identity"`
	CreatedAt  int64           `comment:"Time when row was created"`
	UpdatedAt  int64           `comment:"Time when row was last updated"`
	UpdateID   int64           `bun:",notnull" comment:"Update counter, increments on each and any token metadata update"`
	ContractID uint64          `comment:"Token contract id"`
	TokenId    decimal.Decimal `bun:",type:numeric" comment:"Token id"`
	Type       TokenType       `bun:",type:token_type" comment:"Token type"`
	Status     Status          `bun:",type:status" comment:"Status of resolving metadata"`
	Uri        *string         `comment:"Metadata URI"`
	Metadata   map[string]any  `bun:",type:jsonb" comment:"Token metadata as JSON"`
	Attempts   uint            `bun:",type:SMALLINT" comment:"Attempts count of receiving metadata from third-party sources"`
	Error      *string         `comment:"If metadata is failed this field contains error string"`

	Contract Address `bun:"rel:belongs-to,join:contract_id=id" hasura:"table:address,field:contract_id,remote_field:id,type:oto,name:contract"`
}

// TableName -
func (TokenMetadata) TableName() string {
	return "token_metadata"
}

// BeforeInsert -
func (tm *TokenMetadata) BeforeInsert(ctx context.Context) (context.Context, error) {
	tm.UpdatedAt = time.Now().Unix()
	tm.CreatedAt = tm.UpdatedAt
	tm.UpdateID = TokenUpdateID.Increment()
	return ctx, nil
}

// BeforeUpdate -
func (tm *TokenMetadata) BeforeUpdate(ctx context.Context) (context.Context, error) {
	tm.UpdatedAt = time.Now().Unix()
	tm.UpdateID = TokenUpdateID.Increment()
	return ctx, nil
}
