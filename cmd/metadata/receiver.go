package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	ipfs "github.com/dipdup-io/ipfs-tools"
	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-io/starknet-metadata/internal/types"
	"github.com/dipdup-io/workerpool"
	"github.com/goccy/go-json"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// Receiver -
type Receiver struct {
	ipfsNode     *ipfs.Node
	httpClient   *http.Client
	pool         *workerpool.Pool[storage.TokenMetadata]
	storage      storage.ITokenMetadata
	queue        *types.Queue
	maxAttempts  int
	delay        int
	workersCount int

	wg *sync.WaitGroup
}

// NewReceiver -
func NewReceiver(cfg ReceiverConfig, tm storage.ITokenMetadata, ipfsNode *ipfs.Node) *Receiver {
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

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 100

	r := Receiver{
		ipfsNode: ipfsNode,
		storage:  tm,
		httpClient: &http.Client{
			Transport: t,
		},
		queue:        types.NewQueue(),
		maxAttempts:  maxAttempts,
		delay:        delay,
		workersCount: workersCount,
		wg:           new(sync.WaitGroup),
	}

	r.pool = workerpool.NewPool(r.worker, workersCount)
	return &r
}

// Start -
func (r *Receiver) Start(ctx context.Context) {
	r.pool.Start(ctx)

	r.wg.Add(1)
	go r.work(ctx)
}

func (r *Receiver) work(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.pool.QueueSize() > r.workersCount {
				continue
			}
			tasks, err := r.storage.GetByStatus(ctx, storage.StatusFilled, 100, 0, r.maxAttempts, r.delay)
			if err != nil {
				log.Err(err).Msg("receiving tasks")
				continue
			}

			if len(tasks) == 0 {
				time.Sleep(time.Second)
				continue
			}

			for i := range tasks {
				if r.queue.Contains(tasks[i].Id) {
					continue
				}
				r.queue.Add(tasks[i].Id)
				r.pool.AddTask(tasks[i])
			}
		}
	}
}

func (r *Receiver) worker(ctx context.Context, task storage.TokenMetadata) {
	defer r.queue.Delete(task.Id)

	if task.Uri == nil {
		task.Status = storage.StatusFailed
		task.Attempts = uint(r.maxAttempts)
		err := ErrInvalidUri.Error()
		task.Error = &err
		log.Err(ErrInvalidUri).Str("uri", "nil").Msg("fail to receive metadata")

		if err := r.storage.Update(ctx, &task); err != nil {
			log.Err(err).Msg("saving token metadata in receiver")
		}
		return
	}

	task.Attempts += 1

	log.Info().
		Uint64("id", task.Id).
		Uint("attempt", task.Attempts).
		Hex("contract", task.Contract.Hash).
		Str("uri", *task.Uri).
		Msg("try to receive token metadata")

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var err error
	switch {
	case strings.HasPrefix(*task.Uri, "http://") || strings.HasPrefix(*task.Uri, "https://"):
		err = r.httpRequest(timeoutCtx, &task)
	case strings.HasPrefix(*task.Uri, "ipfs://"):
		err = r.ipfsRequest(timeoutCtx, &task)
	default:
		task.Attempts = uint(r.maxAttempts)
		err = ErrInvalidUri
	}

	if err != nil {
		eStr := err.Error()
		task.Error = &eStr
		log.Err(ErrInvalidUri).Str("uri", *task.Uri).Uint("attempt", task.Attempts).Msg("fail to receive metadata")

		if task.Attempts == uint(r.maxAttempts) {
			task.Status = storage.StatusFailed
		}
	}

	if err := r.storage.Update(ctx, &task); err != nil {
		log.Err(err).Msg("saving token metadata in receiver")
	}
}

func (r *Receiver) httpRequest(ctx context.Context, task *storage.TokenMetadata) error {
	link := *task.Uri
	parsed, err := url.ParseRequestURI(link)
	if err != nil {
		task.Attempts = uint(r.maxAttempts)
		return ErrInvalidUri
	}

	if err := ValidateURL(parsed); err != nil {
		task.Attempts = uint(r.maxAttempts)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("invald status code: %d", resp.StatusCode)
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}

	// 20 MB limit for metadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, 20971520)).Decode(&task.Metadata); err != nil {
		task.Attempts = uint(r.maxAttempts)
		return err
	}

	task.Error = nil
	task.Status = storage.StatusSuccess

	return nil
}

func (r *Receiver) ipfsRequest(ctx context.Context, task *storage.TokenMetadata) error {
	data, err := r.ipfsNode.Get(ctx, strings.TrimPrefix(*task.Uri, "ipfs://"))
	if err != nil {
		return err
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}
	if err := json.Unmarshal(data.Raw, &task.Metadata); err != nil {
		task.Attempts = uint(r.maxAttempts)
		return err
	}

	task.Error = nil
	task.Status = storage.StatusSuccess
	return nil
}

// Close -
func (r *Receiver) Close() error {
	r.wg.Wait()

	if err := r.pool.Close(); err != nil {
		return err
	}

	return nil
}

// ValidateURL -
func ValidateURL(link *url.URL) error {
	host := link.Host
	if strings.Contains(host, ":") {
		newHost, _, err := net.SplitHostPort(link.Host)
		if err != nil {
			return err
		}
		host = newHost
	}
	if host == "localhost" || host == "127.0.0.1" {
		return errors.Wrap(ErrInvalidUri, fmt.Sprintf("invalid host: %s", host))
	}

	for _, mask := range []string{
		"10.0.0.0/8",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4",
	} {
		_, cidr, err := net.ParseCIDR(mask)
		if err != nil {
			return err
		}

		ip := net.ParseIP(host)
		if ip != nil && cidr.Contains(ip) {
			return errors.Wrap(ErrInvalidUri, fmt.Sprintf("restricted subnet: %s", mask))
		}
	}
	return nil
}
