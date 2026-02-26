export PATH:=$(shell pwd)/tools/bin:$(PATH)
SHELL := env PATH='$(PATH)' /bin/sh
GO_DEPS=go.mod go.sum

.PHONY: relayer-local
relayer-local:
	POSTGRES_USER=relayer POSTGRES_PASSWORD=relayer go run ./cmd/relayer/main.go --config ./config/local/config.yml
.PHONY: transfer
transfer:
	go build -o ./bin/transfer ./cmd/transfer

.PHONY: relay
relay:
	go build -o ./bin/relay ./cmd/relay

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
.PHONY: migrate-up migrate-down postgres-login redis-login tidy test

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
	go test -p 1 --tags=test -v -race $(shell go list ./... | grep -v /scripts/)
	docker compose down -v

#
# Build / Deploy
#
REGION=us-east-2
REGISTRY=494494944992.dkr.ecr.us-east-2.amazonaws.com
REPO=skip-mev/solve-relayer
PLATFORM=linux/amd64
COMMIT:=$(shell git rev-parse --short HEAD)

ENVIRONMENT=dev
LEVANT_VAR_FILE:=$(shell mktemp -d)/levant.yaml

RELAYER_IMAGE=${REGISTRY}/${REPO}:${COMMIT}
MIGRATE_IMAGE=${REGISTRY}/${REPO}-migrate:${COMMIT}

build-relayer:
	aws ecr get-login-password --region ${REGION} | docker login --username AWS --password-stdin ${REGISTRY}
	aws ecr describe-repositories --region ${REGION} --repository-names ${REPO} \
		|| aws ecr create-repository --repository-name ${REPO} --region ${REGION}
	docker buildx build \
		--platform ${PLATFORM} \
		-t ${MIGRATE_IMAGE} \
		-t ${REGISTRY}/${REPO}-migrate:latest \
		-f ./docker/migrate.dockerfile \
		--push \
		.
	docker buildx build \
		--platform ${PLATFORM} \
		-t ${RELAYER_IMAGE} \
		-t ${REGISTRY}/${REPO}:latest \
		-f ./docker/relayer.dockerfile \
		--push \
		.

deploy-relayer:
	touch ${LEVANT_VAR_FILE}
	yq e -i '.env |= "${ENVIRONMENT}"' ${LEVANT_VAR_FILE}
	yq e -i '.image |= "${RELAYER_IMAGE}"' ${LEVANT_VAR_FILE}
	yq e -i '.migrate_image |= "${MIGRATE_IMAGE}"' ${LEVANT_VAR_FILE}
	levant deploy ${LEVANT_FLAGS} -force -force-count -var-file=${LEVANT_VAR_FILE} ./nomad/relayer.nomad
