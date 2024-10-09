# Starknet metadata indexer

A DipDup Vertical component intended to indexing starknet tokens metadata

## Features

* Receiving metadata through IPFS and HTTP 
* Receiving data through embedded IPFS node
* Indexing `ERC20`, `ERC721` and `ERC1155` tokens

## Public instances

Public deployments with reasonable rate limits are available for testing and prototyping:
* [Starknet metadata](https://ide.dipdup.io/?resource=starknet-metadata.dipdup.net/v1/graphql) `https://starknet-metadata.dipdup.net/v1/graphql`

## Usage examples

### Receiving ERC20 tokens metadata

```gql
query ERC20_token_metadata {
    token_metadata(limit: 100, where: { type: { _eq: "erc20" } }) {
        created_at
        metadata
        status
        token_id
        uri
        type
        updated_at
        contract {
            hash
        }
    }
}
```

### Receiving certain contract tokens metadata 

```gql
query Contract_token_metadata {
    token_metadata(
        limit: 100
        where: {
            contract: {
                hash: {
                    _eq: "\\x01b10f6d79aa4b556a6c5a067596ca02d8dbf6566f1fbceee136bdaadd9fedc8"
                }
            }
        }
    ) {
        metadata
        contract {
            hash
        }
        uri
        created_at
        token_id
        type
        updated_at
    }
}
```

### Indexer status
To make sure the indexer works fine you can query status table

```gql
query State {
    state {
        last_time
        last_height
        name
    }
}
```

## Self hosted mode

* Create `.env` file relying on [.env.example](https://github.com/dipdup-io/starknet-metadata/blob/master/.env.example)
* Get Starknet node url endpoint for example on [Blast API](https://blastapi.io/public-api/starknet) and fill up `NODE_URL` variable
* Run `docker compose up --build`

## About

DipDup Vertical for Starknet is a federated API including the following services:

- [x] Generic Starknet indexer
- [x] Starknet ID indexer
- [x] Token metadata indexer
- [ ] Starknet search engine (Q3)
- [ ] Chain/dapp/contract analytics
- [ ] Aggregated market data

Project is supported by Starkware and Starknet Foundation via [OnlyDust platform](https://app.onlydust.com/p/dipdup)
