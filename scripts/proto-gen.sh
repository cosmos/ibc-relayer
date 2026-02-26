#!/bin/sh

echo "Generating proto code"

cd proto

buf generate --template buf.gen.yaml --exclude-path ibc/
buf generate --template buf.gen.yaml --path signer/
buf generate --template buf.gen.yaml --path relayerapi/
buf generate --template buf.ibc.gen.yaml --path ibc/
