GOTOOLS=$(shell go list -e -f '{{ join .Imports " "}}' ./tools.go)

# Pin the Go toolchain that compiles the dev tools. mockery's output (in
# particular import-block formatting) is sensitive to the Go std library it
# was built with, so building it with a moving target produces mock drift
# between local and CI. Keep this in sync with the `go` directive in go.mod.
GOTOOLCHAIN ?= go1.24.0

.PHONY: local
local: tidy
	GOBIN=$(shell pwd)/bin GOTOOLCHAIN=$(GOTOOLCHAIN) go install ${GOTOOLS}

.PHONY: global
global: tidy
	GOTOOLCHAIN=$(GOTOOLCHAIN) go install ${GOTOOLS}

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: all
all: local global
