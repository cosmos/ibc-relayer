export PATH:=$(shell pwd)/tools/bin:$(PATH)
SHELL := env PATH='$(PATH)' /bin/sh
GO_DEPS=go.mod go.sum

# sonic/loader references runtime.lastmoduledatap via //go:linkname, which
# Go 1.25+ rejects for modules declaring go <1.25. Disable the check so
# the relayer can stay on go 1.24.x while being built with a newer toolchain.
RELAYER_LDFLAGS := -ldflags="-checklinkname=0"

.PHONY: relayer-local
relayer-local:
	POSTGRES_USER=relayer POSTGRES_PASSWORD=relayer go run $(RELAYER_LDFLAGS) ./cmd/relayer --config ./config/local/config.yml

.PHONY: build
build:
	CGO_ENABLED=0 go build $(RELAYER_LDFLAGS) -o ./bin/relayer ./cmd/relayer

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
.PHONY: mock-gen check-mocks proto-gen

mock-gen: tools
	mockery

check-mocks: mock-gen
	@if ! git diff --exit-code mocks/; then \
		echo "mocks/ has drift from mockery output; run 'make mock-gen' locally and commit the result"; \
		exit 1; \
	fi

proto-gen: tools
	./scripts/proto-gen.sh

#
# Helpful Developer Commands
#
.PHONY: postgres-login tidy test

postgres-login:
	docker compose exec -it postgres psql -U relayer -d relayer

.PHONY: tidy deps
tidy:
	go mod tidy

deps:
	go env
	go mod download

.PHONY: lint lint-fix
lint:
	go tool -modfile=../../go.mod golangci-lint run

lint-fix:
	go tool -modfile=../../go.mod golangci-lint run --fix

test: build check-mocks
	go clean -testcache
	docker compose up -d --wait
	POSTGRES_USER=relayer POSTGRES_PASSWORD=relayer ./bin/relayer migrate --config ./config/local/config.yml
	go test $(RELAYER_LDFLAGS) -p 1 --tags=test -v -race $(shell go list ./... | grep -v /scripts/)
	docker compose down -v --remove-orphans
