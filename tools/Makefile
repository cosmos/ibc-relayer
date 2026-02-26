GOTOOLS=$(shell go list -e -f '{{ join .Imports " "}}' ./tools.go)

.PHONY: local 
local: tidy
	GOBIN=$(shell pwd)/bin go install ${GOTOOLS}

.PHONY: global
global: tidy
	go install ${GOTOOLS}

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: all
all: local global
