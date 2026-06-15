#!/bin/sh

MITM_VERSION=$(git describe --tags)

go build -ldflags "-X main.version=${MITM_VERSION}" -o ./bin/mitm-transformer ./cmd/transformer/main.go

cp bin/mitm-transformer ../../scheduler/mitm_scheduler/bin/.