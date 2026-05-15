#!/bin/sh

echo "Generating proto code"

cd proto

buf generate --template buf.gen.yaml
buf generate --template buf.ibc.gen.yaml --path ibc/
