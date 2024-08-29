# ---------------------------------------------------------------------
#  The first stage container, for building the application
# ---------------------------------------------------------------------
FROM golang:1.22.5-alpine as builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOOS=linux

RUN apk --no-cache add ca-certificates
RUN apk add --update git musl-dev gcc build-base

RUN mkdir -p $GOPATH/src/github.com/dipdup-io/starknet-metadata/

COPY ./go.* $GOPATH/src/github.com/dipdup-io/starknet-metadata/
WORKDIR $GOPATH/src/github.com/dipdup-io/starknet-metadata
RUN go mod download

COPY cmd/metadata cmd/metadata
COPY internal internal

WORKDIR $GOPATH/src/github.com/dipdup-io/starknet-metadata/cmd/metadata/
RUN go build -a -o /go/bin/starknet-metadata .

# ---------------------------------------------------------------------
#  The second stage container, for running the application
# ---------------------------------------------------------------------
FROM scratch

WORKDIR /app/metadata/

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/starknet-metadata /go/bin/starknet-metadata
COPY ./cmd/metadata/graphql ./graphql
COPY ./cmd/metadata/views ./views
COPY ./build/*.yml ./

ENTRYPOINT ["/go/bin/starknet-metadata"]
