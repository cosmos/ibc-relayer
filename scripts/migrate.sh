#!/bin/sh

set -x
docker run \
    -it \
    --rm \
    -v ./db/migrations:/migrations \
    --network skip-relayer_relayer \
    migrate/migrate \
    -path=/migrations/ \
    -database postgres://relayer:relayer@postgres:5432/relayer?sslmode=disable \
    "$@"
