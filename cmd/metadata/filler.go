package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dipdup-io/starknet-go-api/pkg/abi"
	"github.com/dipdup-io/starknet-go-api/pkg/data"
	"github.com/dipdup-io/starknet-go-api/pkg/encoding"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc/pb"
	"github.com/dipdup-io/starknet-metadata/internal/caller"
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-io/starknet-metadata/internal/types"
	"github.com/dipdup-io/workerpool"
	"github.com/dipdup-net/go-lib/config"
	"github.com/goccy/go-json"
	"github.com/karlseguin/ccache/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const (
	entrypointName              = "name"
	entrypointSymbol            = "symbol"
	entrypointDecimals          = "decimals"
	entrypointTokenUri          = "tokenURI"
	entrypointTokenUriSmall     = "tokenUri"
	entrypointUri               = "uri"
	entrypointGetImplementation = "get_implementation"
)

// errors
var (
	ErrInvalidUri         = errors.New("invalid URI")
	ErrViewExecution      = errors.New("view execution")
	ErrInvalidTokenId     = errors.New("invalid token id")
	ErrEntrypointNotFound = errors.New("entrypoint is not found")
	ErrTooBig             = errors.New("metadata file too big")
)

var (
	multicallEntrypoint = encoding.GetSelectorFromName("execute_calls")
)

func newCaller(name string, datasources map[string]config.DataSource, rps int) (caller.Caller, error) {
	if cfg, ok := datasources[name]; ok {
		switch name {
		case "sequencer":
			return caller.NewSequencerCaller(cfg, rps), nil
		case "node":
			return caller.NewNodeRpcCaller(cfg, rps), nil
		}
	}
	return nil, errors.Errorf("unknown caller type: %s", name)
}

// Filler - fills metadata entity with data from node
type Filler struct {
	caller       caller.Caller
	storage      storage.ITokenMetadata
	client       *grpc.Client
	pool         *workerpool.Pool[storage.TokenMetadata]
	cache        *ccache.Cache
	workersCount int
	delay        int
	queue        *types.Queue
	maxAttempts  uint
	multicall    string
	wg           *sync.WaitGroup
}

// NewFiller -
func NewFiller(cfg FillerConfig, datasources map[string]config.DataSource, tm storage.ITokenMetadata, client *grpc.Client) (Filler, error) {
	var (
		workersCount = 10
		maxAttempts  = 5
		delay        = 10
	)

	if cfg.WorkersCount > 0 {
		workersCount = cfg.WorkersCount
	}
	if cfg.MaxAttempts > 0 {
		maxAttempts = cfg.MaxAttempts
	}
	if cfg.Delay > 0 {
		delay = cfg.Delay
	}

	caller, err := newCaller(cfg.Datasource, datasources, cfg.Rps)
	if err != nil {
		return Filler{}, err
	}

	f := Filler{
		caller:       caller,
		storage:      tm,
		client:       client,
		cache:        ccache.New(ccache.Configure().MaxSize(1000)),
		workersCount: workersCount,
		delay:        delay,
		queue:        types.NewQueue(),
		maxAttempts:  uint(maxAttempts),
		multicall:    cfg.Multicall,
		wg:           new(sync.WaitGroup),
	}

	f.pool = workerpool.NewPool(f.worker, workersCount)
	return f, nil
}

// Start -
func (f Filler) Start(ctx context.Context) {
	f.pool.Start(ctx)

	f.wg.Add(1)
	go f.work(ctx)
}

func (f Filler) work(ctx context.Context) {
	defer f.wg.Done()

	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if f.pool.QueueSize() > f.workersCount {
				continue
			}
			tasks, err := f.storage.GetByStatus(ctx, storage.StatusNew, 100, 0, int(f.maxAttempts), f.delay)
			if err != nil {
				log.Err(err).Msg("receiving filler tasks")
				continue
			}

			if len(tasks) == 0 {
				time.Sleep(time.Second)
				continue
			}

			for i := range tasks {
				if f.queue.Contains(tasks[i].Id) {
					continue
				}
				f.queue.Add(tasks[i].Id)
				f.pool.AddTask(tasks[i])
			}
		}
	}
}

// Close -
func (f Filler) Close() error {
	f.wg.Wait()

	if err := f.pool.Close(); err != nil {
		return err
	}

	return nil
}

func (f Filler) worker(ctx context.Context, task storage.TokenMetadata) {
	var (
		err       error
		tokenType = task.Type
	)

	task.Attempts += 1

	log.Info().
		Uint64("id", task.Id).
		Uint("attempt", task.Attempts).
		Hex("contract", task.Contract.Hash).
		Str("token_type", string(tokenType)).
		Msg("try to fill metadata")

	schema, err := f.getJsonSchema(ctx, task.Contract.Hash)
	if err != nil {
		log.Err(err).Hex("contract", task.Contract.Hash).Msg("can't receive json schema")
		return
	}

	switch tokenType {
	case storage.TokenTypeERC20:
		err = f.handleErc20(ctx, schema, &task)
	case storage.TokenTypeERC721:
		err = f.handleErc721(ctx, schema, &task)
	case storage.TokenTypeERC1155:
		err = f.handleErc1155(ctx, schema, &task)
	}
	if err != nil {
		if len(task.Metadata) == 0 {
			task.Metadata = nil
		}
		taskErr := err.Error()
		task.Error = &taskErr

		switch {
		case errors.Is(err, ErrInvalidUri) || errors.Is(err, ErrViewExecution) || errors.Is(err, ErrEntrypointNotFound):
			task.Status = storage.StatusFailed
			task.Attempts = f.maxAttempts
		case task.Attempts >= f.maxAttempts:
			task.Status = storage.StatusFailed
		}

		log.Err(err).Uint64("id", task.Id).Msg("Filler.worker")
	} else {
		task.Attempts = 0
	}

	if err := f.storage.Update(ctx, &task); err != nil {
		log.Err(err).Uint64("id", task.Id).Msg("saving token metadata")
	}
}

func (f Filler) handleErc20(ctx context.Context, schema abi.JsonSchema, task *storage.TokenMetadata) error {
	address := data.NewFeltFromBytes(task.Contract.Hash)
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}

	nameSelector, _, err := f.getSelectorByName(schema, entrypointName)
	if err != nil {
		return err
	}
	symbolSelector, _, err := f.getSelectorByName(schema, entrypointSymbol)
	if err != nil {
		return err
	}
	decimalsSelector, _, err := f.getSelectorByName(schema, entrypointDecimals)
	if err != nil {
		return err
	}

	if f.multicall != "" {
		if err := handlerFillerError(f.multicallErc20(ctx, address, nameSelector, symbolSelector, decimalsSelector, task)); err != nil {
			return err
		}
	} else {
		if err := handlerFillerError(f.getName(ctx, address, nameSelector, task)); err != nil {
			return err
		}

		if err := handlerFillerError(f.getSymbol(ctx, address, symbolSelector, task)); err != nil {
			return err
		}

		if err := handlerFillerError(f.getDecimals(ctx, address, decimalsSelector, task)); err != nil {
			return err
		}
	}

	task.Status = storage.StatusSuccess
	return nil
}

func (f Filler) handleErc721(ctx context.Context, schema abi.JsonSchema, task *storage.TokenMetadata) error {
	address := data.NewFeltFromBytes(task.Contract.Hash)
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}

	nameSelector, _, err := f.getSelectorByName(schema, entrypointName)
	if err != nil {
		return err
	}
	symbolSelector, _, err := f.getSelectorByName(schema, entrypointSymbol)
	if err != nil {
		return err
	}
	uriSelector, funcSchema, err := f.getUriSelector(schema)
	if err != nil {
		return err
	}

	if f.multicall != "" {
		if err := handlerFillerError(f.multicallErc721(ctx, address, nameSelector, symbolSelector, uriSelector, funcSchema, task)); err != nil {
			return err
		}
	} else {
		if err := handlerFillerError(f.getName(ctx, address, nameSelector, task)); err != nil {
			return err
		}

		if err := handlerFillerError(f.getSymbol(ctx, address, symbolSelector, task)); err != nil {
			return err
		}

		if err := handlerFillerError(f.getTokenUri(ctx, address, uriSelector, funcSchema, task)); err != nil {
			return err
		}
	}

	task.Status = storage.StatusFilled
	return nil
}

func (f Filler) handleErc1155(ctx context.Context, schema abi.JsonSchema, task *storage.TokenMetadata) error {
	address := data.NewFeltFromBytes(task.Contract.Hash)

	selector, funcSchema, err := f.getUriSelector(schema)
	if err != nil {
		return err
	}

	if err := handlerFillerError(f.getTokenUri(ctx, address, selector, funcSchema, task)); err != nil {
		return err
	}

	task.Status = storage.StatusFilled
	return nil
}

func handlerFillerError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.Canceled):
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return errors.Wrap(ErrViewExecution, err.Error())
	}
}

func (f Filler) getJsonSchema(ctx context.Context, hash []byte) (abi.JsonSchema, error) {
	cacheKey := fmt.Sprintf("%x:schema", hash)
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		var intSchema abi.JsonSchema
		schemaBytes, err := f.client.JsonSchemaForContract(ctx, &pb.Bytes{
			Data: hash,
		})
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(schemaBytes.Data, &intSchema); err != nil {
			return nil, err
		}

		if !f.isProxy(intSchema) {
			return intSchema, nil
		}

		var (
			impl []byte
			typ  = -1
		)

		proxy, err := f.client.GetProxy(ctx, hash, nil)
		if err != nil {
			if name, ok := f.findName(intSchema, entrypointGetImplementation, "getImplementation", "implementation", "getImplementationHash"); ok {
				impl, err = f.getImplementation(ctx, data.NewFeltFromBytes(hash), name)
				if err != nil {
					return nil, err
				}
			}
		} else {
			impl = proxy.GetHash()
			typ = int(proxy.GetType())
		}

		if len(impl) > 0 {
			var schemaBytes *pb.Bytes
			switch typ {
			case 0:
				schemaBytes, err = f.client.JsonSchemaForClass(ctx, &pb.Bytes{
					Data: impl,
				})
			case 1:
				schemaBytes, err = f.client.JsonSchemaForContract(ctx, &pb.Bytes{
					Data: impl,
				})
			case -1:
				schemaBytes, err = f.client.JsonSchemaForContract(ctx, &pb.Bytes{
					Data: impl,
				})
				if err != nil {
					schemaBytes, err = f.client.JsonSchemaForClass(ctx, &pb.Bytes{
						Data: impl,
					})
				}
			}
			if err != nil {
				return nil, err
			}

			if err := json.Unmarshal(schemaBytes.Data, &intSchema); err != nil {
				return nil, err
			}
		}

		return intSchema, nil
	})
	if err != nil {
		return abi.JsonSchema{}, err
	}

	return item.Value().(abi.JsonSchema), nil
}

func (f Filler) isProxy(schema abi.JsonSchema) bool {
	if _, has := schema.Functions["__default__"]; has {
		return true
	}
	_, has := schema.L1Handlers["__l1_default__"]
	return has
}

func (f Filler) findName(schema abi.JsonSchema, names ...string) (string, bool) {
	for i := range names {
		if _, ok := schema.Functions[names[i]]; ok {
			return names[i], ok
		}
	}

	return "", false
}

func (f Filler) getSelectorByName(schema abi.JsonSchema, name string) (string, abi.JsonSchemaFunction, error) {
	var entrypoint string
	for funcName := range schema.Functions {
		if strings.EqualFold(funcName, name) {
			entrypoint = funcName
			break
		}
	}

	if entrypoint == "" {
		return "", abi.JsonSchemaFunction{}, errors.Wrap(ErrEntrypointNotFound, name)
	}

	return encoding.GetSelectorWithPrefixFromName(entrypoint), schema.Functions[entrypoint], nil
}

func (f Filler) multicallErc20(ctx context.Context, address data.Felt, selectorName, selectorSymbol, selectorDecimals string, task *storage.TokenMetadata) error {
	response, err := f.caller.Call(ctx, f.multicall, multicallEntrypoint, []string{
		"0x3",
		address.String(),
		selectorName,
		"0x0",
		address.String(),
		selectorSymbol,
		"0x0",
		address.String(),
		selectorDecimals,
		"0x0",
		"0x0",
	})
	if err != nil {
		return err
	}
	if len(response) != 4 {
		return errors.Wrapf(ErrViewExecution, "invalid multicall response: %v", response)
	}
	task.Metadata[entrypointName] = response[1].ToAsciiString()
	task.Metadata[entrypointSymbol] = response[2].ToAsciiString()
	task.Metadata[entrypointDecimals] = response[3].ToAsciiString()
	return nil
}

func (f Filler) multicallErc721(ctx context.Context, address data.Felt, selectorName, selectorSymbol, selectorUri string, uriSchema abi.JsonSchemaFunction, task *storage.TokenMetadata) error {
	tokenId, err := data.NewUint256FromString(task.TokenId.String())
	if err != nil {
		return errors.Wrap(ErrInvalidTokenId, err.Error())
	}
	tokenCalldata := tokenId.Calldata()
	lenCallData := fmt.Sprintf("%d", len(tokenCalldata))
	calldata := []string{
		"0x3",
		address.String(),
		selectorName,
		"0x0",
		address.String(),
		selectorSymbol,
		"0x0",
		address.String(),
		selectorUri,
	}
	calldata = append(calldata, lenCallData, lenCallData)
	calldata = append(calldata, tokenCalldata...)

	response, err := f.caller.Call(ctx, f.multicall, multicallEntrypoint, calldata)
	if err != nil {
		return err
	}
	if len(response) < 4 {
		return errors.Wrapf(ErrViewExecution, "invalid multicall response: %v", response)
	}
	task.Metadata[entrypointName] = response[1].ToAsciiString()
	task.Metadata[entrypointSymbol] = response[2].ToAsciiString()
	uri := parseUri(uriSchema, response[3:])
	if url, err := url.ParseRequestURI(uri); err != nil {
		return errors.Wrap(ErrInvalidUri, uri)
	} else if err := ValidateURL(url); err != nil {
		return errors.Wrap(ErrInvalidUri, uri)
	}
	task.Uri = &uri
	return nil
}

func (f Filler) getName(ctx context.Context, address data.Felt, selector string, task *storage.TokenMetadata) error {
	cacheKey := fmt.Sprintf("%x:%s", address.Bytes(), entrypointName)
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		response, err := f.caller.Call(ctx, address.String(), selector, []string{})
		if err != nil {
			return nil, err
		}
		if len(response) < 1 {
			return nil, errors.Wrapf(ErrViewExecution, "invalid response for name: %v", response)
		}

		return response[0].ToAsciiString(), nil
	})
	if err != nil {
		return err
	}

	task.Metadata[entrypointName] = item.Value().(string)
	return nil
}

func (f Filler) getSymbol(ctx context.Context, address data.Felt, selector string, task *storage.TokenMetadata) error {
	cacheKey := fmt.Sprintf("%x:%s", address.Bytes(), entrypointSymbol)
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		response, err := f.caller.Call(ctx, address.String(), selector, []string{})
		if err != nil {
			return nil, err
		}
		if len(response) < 1 {
			return nil, errors.Wrapf(ErrViewExecution, "invalid response for symbol: %v", response)
		}

		return response[0].ToAsciiString(), nil
	})
	if err != nil {
		return err
	}

	task.Metadata[entrypointSymbol] = item.Value().(string)
	return nil
}

func (f Filler) getDecimals(ctx context.Context, address data.Felt, selector string, task *storage.TokenMetadata) error {
	cacheKey := fmt.Sprintf("%x:%s", address.Bytes(), entrypointDecimals)
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		response, err := f.caller.Call(ctx, address.String(), selector, []string{})
		if err != nil {
			return nil, err
		}
		if len(response) < 1 {
			return nil, errors.Wrapf(ErrViewExecution, "invalid response for decimals: %v", response)
		}

		return response[0].Uint64()
	})
	if err != nil {
		return err
	}

	task.Metadata[entrypointDecimals] = item.Value().(uint64)
	return nil
}

func (f Filler) getUriSelector(schema abi.JsonSchema) (selector string, funcSchema abi.JsonSchemaFunction, err error) {
	for _, e := range []string{
		entrypointTokenUri, entrypointUri, entrypointTokenUriSmall,
	} {
		selector, funcSchema, err = f.getSelectorByName(schema, e)
		if err == nil {
			return
		}
	}
	return
}

func (f Filler) getTokenUri(ctx context.Context, address data.Felt, selector string, funcSchema abi.JsonSchemaFunction, task *storage.TokenMetadata) error {
	cacheKey := fmt.Sprintf("%x:uri:%s", address.Bytes(), task.TokenId.String())
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		tokenId, err := data.NewUint256FromString(task.TokenId.String())
		if err != nil {
			return nil, errors.Wrap(ErrInvalidTokenId, err.Error())
		}

		response, err := f.caller.Call(ctx, address.String(), selector, tokenId.Calldata())
		if err != nil {
			return nil, err
		}
		if len(response) < 1 {
			return nil, errors.Wrapf(ErrViewExecution, "invalid response for token uri: %v", response)
		}

		uri := parseUri(funcSchema, response)
		if url, err := url.ParseRequestURI(uri); err != nil {
			return nil, errors.Wrap(ErrInvalidUri, uri)
		} else if err := ValidateURL(url); err != nil {
			return nil, errors.Wrap(ErrInvalidUri, uri)
		}

		return uri, nil
	})
	if err != nil {
		return err
	}

	uri := item.Value().(string)
	task.Uri = &uri
	return nil
}

func (f Filler) getImplementation(ctx context.Context, address data.Felt, name string) ([]byte, error) {
	cacheKey := fmt.Sprintf("%x:%s", address.Bytes(), entrypointGetImplementation)
	item, err := f.cache.Fetch(cacheKey, time.Hour, func() (interface{}, error) {
		selector := encoding.GetSelectorWithPrefixFromName(name)
		response, err := f.caller.Call(ctx, address.String(), selector, []string{})
		if err != nil {
			return nil, err
		}
		if len(response) < 1 {
			return nil, errors.Wrapf(ErrViewExecution, "invalid response for decimals: %v", response)
		}

		return response[0].Bytes(), nil
	})
	if err != nil {
		return nil, err
	}

	return item.Value().([]byte), nil
}

func parseUri(funcSchema abi.JsonSchemaFunction, response []data.Felt) string {
	var isArray bool
	for name := range funcSchema.Output.Properties {
		if strings.HasSuffix(name, "_len") {
			isArray = true
			break
		}
	}

	if isArray {
		response = response[1:]
	}

	var uri string
	for i := range response {
		if response[i] == "0x0" {
			break
		}
		uri += response[i].ToAsciiString()
	}

	return uri
}
