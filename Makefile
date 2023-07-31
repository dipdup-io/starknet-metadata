-include .env
export $(shell sed 's/=.*//' .env)

up:
	docker-compose -f docker-compose.yml up -d --build

down:
	docker-compose -f docker-compose.yml down -v

metadata:
	cd cmd/metadata && go run . -c ../../build/dipdup.yml

lint:
	golangci-lint run

test:
	go test ./...
