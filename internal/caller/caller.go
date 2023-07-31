package caller

import (
	"context"

	"github.com/dipdup-io/starknet-go-api/pkg/data"
)

// Caller -
type Caller interface {
	Call(ctx context.Context, contract, entrypoint string, calldata []string) ([]data.Felt, error)
}
