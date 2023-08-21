package caller

import (
	"context"
	"time"

	"github.com/dipdup-io/starknet-go-api/pkg/data"
	"github.com/dipdup-io/starknet-go-api/pkg/sequencer"
	"github.com/dipdup-net/go-lib/config"
)

// SequencerCaller -
type SequencerCaller struct {
	api     sequencer.API
	timeout time.Duration
}

// NewSequencerCaller -
func NewSequencerCaller(cfg config.DataSource) *SequencerCaller {
	var (
		timeout = time.Second * 10
		opts    = make([]sequencer.ApiOption, 0)
	)

	if cfg.RequestsPerSecond > 0 {
		opts = append(opts, sequencer.WithRateLimit(cfg.RequestsPerSecond))
	}

	if cfg.Timeout > 0 {
		timeout = time.Second * time.Duration(cfg.Timeout)
	}
	return &SequencerCaller{
		api:     sequencer.NewAPI("", cfg.URL, opts...),
		timeout: timeout,
	}
}

// Call -
func (sc *SequencerCaller) Call(ctx context.Context, contract, entrypoint string, calldata []string) ([]data.Felt, error) {
	reqCtx, cancelReq := context.WithTimeout(ctx, sc.timeout)
	defer cancelReq()

	response, err := sc.api.CallContract(
		reqCtx,
		data.BlockID{
			String: data.Latest,
		},
		contract,
		entrypoint,
		calldata,
	)
	if err != nil {
		return nil, err
	}

	return response.Result, nil
}
