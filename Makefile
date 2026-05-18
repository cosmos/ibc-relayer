export PATH:=$(shell pwd)/tools/bin:$(PATH)
SHELL := env PATH='$(PATH)' /bin/sh
GO_DEPS=go.mod go.sum

# sonic/loader references runtime.lastmoduledatap via //go:linkname, which
# Go 1.25+ rejects for modules declaring go <1.25. Disable the check so
# the relayer can stay on go 1.24.x while being built with a newer toolchain.
RELAYER_LDFLAGS := -ldflags="-checklinkname=0"

.PHONY: relayer-local
relayer-local:
	POSTGRES_USER=relayer POSTGRES_PASSWORD=relayer go run $(RELAYER_LDFLAGS) ./cmd/relayer/main.go --config ./config/local/config.yml
.PHONY: transfer
transfer:
	go build $(RELAYER_LDFLAGS) -o ./bin/transfer ./cmd/transfer

.PHONY: relay
relay:
	go build $(RELAYER_LDFLAGS) -o ./bin/relay ./cmd/relay

#
# Developer Tools
#
.PHONY: tools

tools:
	make -C tools local

#
# Code Generation
#
.PHONY: mock-gen proto-gen

mock-gen: tools
	mockery

proto-gen: tools
	./scripts/proto-gen.sh

#
# Helpful Developer Commands
#
.PHONY: migrate-up migrate-down postgres-login tidy test

migrate-up:
	./scripts/migrate.sh up 1

migrate-down:
	./scripts/migrate.sh down 1

postgres-login:
	docker compose exec -it postgres psql -U relayer -d relayer

.PHONY: tidy deps
tidy:
	go mod tidy

deps:
	go env
	go mod download

test:
	go clean -testcache
	docker compose up -d
	go test $(RELAYER_LDFLAGS) -p 1 --tags=test -v -race $(shell go list ./... | grep -v /scripts/)
	docker compose down -v
