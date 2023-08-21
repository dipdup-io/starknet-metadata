package main

import (
	ipfs "github.com/dipdup-io/ipfs-tools"
	"github.com/dipdup-io/starknet-indexer/pkg/grpc"
	"github.com/dipdup-net/go-lib/config"
)

// Config -
type Config struct {
	config.Config `yaml:",inline"`
	Metadata      Metadata           `yaml:"metadata"`
	LogLevel      string             `yaml:"log_level" validate:"omitempty,oneof=debug trace info warn error fatal panic"`
	GRPC          *grpc.ClientConfig `yaml:"grpc" validate:"required"`
}

// Substitute -
func (c *Config) Substitute() error {
	if err := c.Config.Substitute(); err != nil {
		return err
	}
	return nil
}

// Load -
func Load(filename string) (cfg Config, err error) {
	err = config.Parse(filename, &cfg)
	return
}

// Metadata -
type Metadata struct {
	IPFS                 IPFS           `yaml:"ipfs"`
	Filler               FillerConfig   `yaml:"filler"`
	Receiver             ReceiverConfig `yaml:"receiver"`
	HTTPTimeout          uint64         `yaml:"http_timeout" validate:"min=1"`
	MaxRetryCountOnError int            `yaml:"max_retry_count_on_error" validate:"min=1"`
	MaxCPU               int            `yaml:"max_cpu,omitempty" validate:"omitempty,min=1"`
}

// Filters -
type Filters struct {
	Accounts   []config.Alias[config.Contract] `yaml:"accounts" validate:"max=50"`
	FirstLevel uint64                          `yaml:"first_level" validate:"min=0"`
	LastLevel  uint64                          `yaml:"last_level" validate:"min=0"`
}

// Addresses -
func (f Filters) Addresses() []string {
	addresses := make([]string, 0)
	for i := range f.Accounts {
		addresses = append(addresses, f.Accounts[i].Struct().Address)
	}
	return addresses
}

// MetadataDataSource -
type MetadataDataSource struct {
	Tzkt config.Alias[config.DataSource] `yaml:"tzkt" validate:"url"`
}

// IPFS -
type IPFS struct {
	Dir       string          `yaml:"dir"`
	Bootstrap []string        `yaml:"bootstrap"`
	Blacklist []string        `yaml:"blacklist"`
	Timeout   uint64          `yaml:"timeout" validate:"min=1"`
	Delay     int             `yaml:"delay" validate:"min=1"`
	Providers []ipfs.Provider `yaml:"providers" validate:"omitempty"`
}

// FillerConfig -
type FillerConfig struct {
	Datasource   string `yaml:"datasource" validate:"required,oneof=sequencer node"`
	WorkersCount int    `yaml:"workers_count" validate:"required,min=1"`
	MaxAttempts  int    `yaml:"max_attempts" validate:"omitempty,min=1"`
	Delay        int    `yaml:"delay" validate:"omitempty,min=1"`
	Multicall    string `yaml:"multicall_contract" validate:"omitempty"`
}

// ReceiverConfig -
type ReceiverConfig struct {
	WorkersCount int `yaml:"workers_count" validate:"required,min=1"`
	MaxAttempts  int `yaml:"max_attempts" validate:"omitempty,min=1"`
	Delay        int `yaml:"delay" validate:"omitempty,min=1"`
}
