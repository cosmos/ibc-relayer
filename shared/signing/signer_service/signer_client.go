package signer_service

import (
	signerservice "github.com/cosmos/ibc-relayer/proto/gen/signer"
)

// SignerClient is the expected interface that a client of the signer service must implement
//
//go:generate mockery --name=SignerClient --filename=mock_signer_client.go --case=underscore
type SignerClient interface {
	signerservice.SignerServiceClient
}
