package storage

import (
	"context"
	"time"

	"github.com/dipdup-io/starknet-metadata/internal/types"
	"github.com/dipdup-net/indexer-sdk/pkg/storage"
	"github.com/shopspring/decimal"
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
	// nolint
	tableName struct{} `pg:"token_metadata" comment:"Table contains token metadata"`

	Id         uint64          `pg:"id,notnull,type:bigint,pk" comment:"Unique internal identity"`
	CreatedAt  int64           `comment:"Time when row was created"`
	UpdatedAt  int64           `comment:"Time when row was last updated"`
	UpdateID   int64           `json:"-" pg:",use_zero,notnull" comment:"Update counter, increments on each and any token metadata update"`
	ContractID uint64          `comment:"Token contract id"`
	TokenId    decimal.Decimal `pg:",type:numeric,use_zero" comment:"Token id"`
	Type       TokenType       `pg:",type:token_type" comment:"Token type"`
	Status     Status          `pg:",type:status" comment:"Status of resolving metadata"`
	Uri        *string         `comment:"Metadata URI"`
	Metadata   map[string]any  `pg:",type:jsonb" comment:"Token metadata as JSON"`
	Attempts   uint            `pg:",type:SMALLINT,use_zero" comment:"Attempts count of receiving metadata from third-party sources"`
	Error      *string         `comment:"If metadata is failed this field contains error string"`

	Contract Address `pg:"rel:has-one" hasura:"table:address,field:contract_id,remote_field:id,type:oto,name:contract"`
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
