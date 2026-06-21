#!/bin/sh

MITM_VERSION=$(git describe --tags)

CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${MITM_VERSION}" -o ./bin/mitm-transformer ./cmd/transformer/main.go

cp bin/mitm-transformer ../../scheduler/mitm_scheduler/bin/.