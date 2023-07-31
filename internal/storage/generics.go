package storage

import "github.com/dipdup-net/indexer-sdk/pkg/storage"

// Models - list all models
var Models = []storage.Model{
	&State{},
	&Address{},
	&TokenMetadata{},
}
