package caller

import (
	"context"
	"time"

	"github.com/dipdup-io/starknet-go-api/pkg/data"
	rpc "github.com/dipdup-io/starknet-go-api/pkg/rpc"
	"github.com/dipdup-net/go-lib/config"
)

// NodeRpcConfig -
type NodeRpcConfig struct {
	RpcUrl  string `yaml:"rpc_url" validate:"omitempty,url"`
	Rps     int    `yaml:"rps" validate:"omitempty,min=1"`
	Timeout uint64 `yaml:"timeout" validate:"min=1"`
}

// SequencerCaller -
type NodeRpcCaller struct {
	api     rpc.API
	timeout time.Duration
}

// NewNodeRpcCaller -
func NewNodeRpcCaller(cfg config.DataSource, rps int) *NodeRpcCaller {
	var (
		timeout = time.Second * 10
		opts    = make([]rpc.ApiOption, 0)
	)

	if rps > 0 {
		opts = append(opts, rpc.WithRateLimit(rps))
	}

	if cfg.Timeout > 0 {
		timeout = time.Second * time.Duration(cfg.Timeout)
	}
	return &NodeRpcCaller{
		api:     rpc.NewAPI(cfg.URL, opts...),
		timeout: timeout,
	}
}

// Call -
func (nrc *NodeRpcCaller) Call(ctx context.Context, contract, entrypoint string, calldata []string) ([]data.Felt, error) {
	reqCtx, cancelReq := context.WithTimeout(ctx, nrc.timeout)
	defer cancelReq()

	response, err := nrc.api.Call(
		reqCtx,
		rpc.CallRequest{
			ContractAddress:    contract,
			EntrypointSelector: entrypoint,
			Calldata:           calldata,
		},
		data.BlockID{
			String: data.Latest,
		},
	)
	if err != nil {
		return nil, err
	}

	result := make([]data.Felt, len(response.Result))
	for i := range response.Result {
		result[i] = data.Felt(response.Result[i])
	}

	return result, nil
}
