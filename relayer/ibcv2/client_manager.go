package ibcv2

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	tmRPC "github.com/cometbft/cometbft/rpc/client"
	rpcclienthttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	signerservice "github.com/cosmos/ibc-relayer/proto/gen/signer"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	ethereumrpc "github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/metrics"
	"github.com/cosmos/ibc-relayer/shared/signing"
	"github.com/cosmos/ibc-relayer/shared/signing/signer_service"
	"github.com/cosmos/ibc-relayer/shared/utils"
)

type BridgeClientManager interface {
	GetClient(ctx context.Context, chainID string) (ibcv2.BridgeClient, error)
}

type ClientManager struct {
	// clients is a map of chain ids to bridge clients
	clients map[string]ibcv2.BridgeClient
}

func NewClientManagerFromConfig(ctx context.Context, keys map[string]string, signerConn *grpc.ClientConn, signerCosmosWalletID string, signerEVMWalletID string, chains ...config.ChainConfig) (*ClientManager, error) {
	var signerManager *signer_service.SignerManager
	if signerConn != nil {
		signerClient := signerservice.NewSignerServiceClient(signerConn)
		manager := signer_service.NewSignerManager(signerClient)
		signerManager = &manager

		hasCosmosChains := false
		hasEVMChains := false
		for _, chain := range chains {
			switch chain.Type {
			case config.ChainType_COSMOS:
				hasCosmosChains = true
			case config.ChainType_EVM:
				hasEVMChains = true
			}
		}

		healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Only validate if chains are configured and wallet ID is provided
		if hasCosmosChains && signerCosmosWalletID != "" {
			_, err := signerManager.GetWallet(healthCtx, &signerservice.GetWalletRequest{
				Id:         signerCosmosWalletID,
				PubkeyType: signerservice.PubKeyType_Cosmos,
			})
			if err != nil {
				return nil, fmt.Errorf("remote signer health check failed for wallet %s: %w", signerCosmosWalletID, err)
			}
		}

		if hasEVMChains && signerEVMWalletID != "" {
			_, err := signerManager.GetWallet(healthCtx, &signerservice.GetWalletRequest{
				Id:         signerEVMWalletID,
				PubkeyType: signerservice.PubKeyType_Ethereum,
			})
			if err != nil {
				return nil, fmt.Errorf("remote signer health check failed for wallet %s: %w", signerEVMWalletID, err)
			}
		}

		lmt.Logger(ctx).Info("Successfully connected to remote signer service")
	}

	clients := make(map[string]ibcv2.BridgeClient)

	for _, chain := range chains {
		var (
			bridge  ibcv2.BridgeClient
			chainID string
		)

		switch chain.Type {
		case config.ChainType_EVM:
			chainID = chain.ChainID

			signer, err := createEthSigner(ctx, chainID, keys, signerManager, signerEVMWalletID)
			if err != nil {
				return nil, fmt.Errorf("creating eth signer for chain %s: %w", chainID, err)
			}

			client, err := createEthClient(ctx, chainID)
			if err != nil {
				return nil, fmt.Errorf("creating eth client for chain %s: %w", chainID, err)
			}

			bridge, err = ibcv2.NewEVMBridgeClient(
				ctx,
				chainID,
				chain.EVM.Contracts.ICS26RouterAddress,
				chain.EVM.Contracts.ICS20TransferAddress,
				client,
				signer,
				chain.EVM.GasFeeCapMultiplier,
				chain.EVM.GasTipCapMultiplier,
			)
			if err != nil {
				return nil, fmt.Errorf("creating evm bridge client for chain %s: %w", chainID, err)
			}
		case config.ChainType_COSMOS:
			chainID = chain.ChainID
			prefix := chain.Cosmos.AddressPrefix
			gasPrice := chain.Cosmos.GasPrice
			feeDenom := chain.Cosmos.IBCV2TxFeeDenom
			feeAmount := chain.Cosmos.IBCV2TxFeeAmount

			if (gasPrice > 0 || feeAmount > 0) && feeDenom == "" {
				return nil, fmt.Errorf("ibcv2 gas price or ibcv2 fee amount cannot be specified without setting ibcv2 fee denom")
			}
			if feeDenom != "" && (gasPrice == 0 && feeAmount == 0) {
				return nil, fmt.Errorf("ibcv2 gas price and ibcv2 fee amount cannot be unset when a ibcv2 fee denom is set")
			}
			if gasPrice > 0 && feeAmount > 0 {
				return nil, fmt.Errorf("ibcv2 fee denom and ibcv2 gas price cannot both be set")
			}

			signer, err := createCosmosSigner(ctx, chainID, keys, signerManager, signerCosmosWalletID)
			if err != nil {
				return nil, fmt.Errorf("creating cosmos signer for chain %s: %w", chainID, err)
			}

			rpc, err := createCosmosRPC(ctx, chainID)
			if err != nil {
				return nil, fmt.Errorf("creating cosmos rpc client for chain %s: %w", chainID, err)
			}

			conn, err := createCosmosGRPC(ctx, chainID)
			if err != nil {
				return nil, fmt.Errorf("creating cosmos grpc connection for chain %s: %w", chainID, err)
			}

			bridge = ibcv2.NewCosmosBridgeClient(chainID, signer, prefix, gasPrice, feeDenom, feeAmount, conn, rpc)
		case config.ChainType_SVM:
			continue
		default:
			continue
		}
		clients[chainID] = bridge
	}

	return NewClientManager(clients), nil
}

func NewClientManager(clients map[string]ibcv2.BridgeClient) *ClientManager {
	return &ClientManager{clients: clients}
}

func (manager *ClientManager) GetClient(ctx context.Context, chainID string) (ibcv2.BridgeClient, error) {
	client, ok := manager.clients[chainID]
	if !ok {
		return nil, fmt.Errorf("no configured ibcv2 bridge client for chain ID %s", chainID)
	}
	return client, nil
}

func createEthClient(ctx context.Context, chainID string) (*ethclient.Client, error) {
	rpc, err := config.GetConfigReader(ctx).GetRPCEndpoint(chainID)
	if err != nil {
		return nil, err
	}

	basicAuth, err := config.GetConfigReader(ctx).GetBasicAuth(chainID)
	if err != nil {
		return nil, err
	}

	c := &http.Client{Transport: metrics.NewTransportMiddleware(http.DefaultTransport, nil)}
	conn, err := ethereumrpc.DialOptions(ctx, rpc, ethereumrpc.WithHTTPClient(c))
	if err != nil {
		return nil, err
	}
	if basicAuth != nil {
		conn.SetHeader("Authorization", fmt.Sprintf("Basic %s", *basicAuth))
	}

	return ethclient.NewClient(conn), nil
}

func createEthSigner(ctx context.Context, chainID string, keys map[string]string, signerManager *signer_service.SignerManager, signerWalletID string) (signing.Signer, error) {
	if signerManager != nil {
		lmt.Logger(ctx).Info("Using remote signer for EVM chain",
			zap.String("chain_id", chainID),
			zap.String("wallet_id", signerWalletID))

		return NewRemoteEVMSigner(signerManager, signerWalletID, chainID), nil
	}
	lmt.Logger(ctx).Info("Remote signing not configured - using local signer for EVM chain", zap.String("chain_id", chainID))
	privateKeyStr, ok := keys[chainID]
	if !ok {
		return nil, fmt.Errorf("private key not found for chain %s", chainID)
	}
	privateKeyStr = strings.TrimPrefix(privateKeyStr, "0x")

	privateKey, err := crypto.HexToECDSA(string(privateKeyStr))
	if err != nil {
		return nil, fmt.Errorf("converting hex private key to ecdsa: %w", err)
	}

	return signing.NewLocalEthereumSigner(privateKey), nil
}

func createCosmosRPC(ctx context.Context, chainID string) (tmRPC.Client, error) {
	rpc, err := config.GetConfigReader(ctx).GetRPCEndpoint(chainID)
	if err != nil {
		return nil, fmt.Errorf("getting rpc endpoint for chain %s: %w", chainID, err)
	}

	basicAuth, err := config.GetConfigReader(ctx).GetBasicAuth(chainID)
	if err != nil {
		return nil, fmt.Errorf("getting basic auth for chain %s: %w", chainID, err)
	}

	transport := metrics.NewTransportMiddleware(
		utils.NewBasicAuthTransport(basicAuth, http.DefaultTransport),
		nil,
	)
	client, err := rpcclienthttp.NewWithClient(rpc, "/websocket", &http.Client{
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("creating tmrpc client for chain %s: %w", chainID, err)
	}

	return client, nil
}

func createCosmosGRPC(ctx context.Context, chainID string) (grpc.ClientConnInterface, error) {
	addr, tlsEnabled, err := config.GetConfigReader(ctx).GetGRPCEndpoint(chainID)
	if err != nil {
		return nil, fmt.Errorf("getting grpc endpoint for chain %s: %w", chainID, err)
	}

	var opts []grpc.DialOption
	if !tlsEnabled {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // nosemgrep: go.grpc.tls.grpc-client-new-insecure-connection.grpc-client-new-insecure-connection
	} else {
		// InsecureSkipVerify: external chain gRPC endpoints use Tailscale/internal networking where cert validation is not required.
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})))
	}
	opts = append(opts, grpc.WithUnaryInterceptor(metrics.UnaryClientInterceptor))

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating grpc client at address %s with tls %t: %w", addr, tlsEnabled, err)
	}

	return conn, nil
}

func createCosmosSigner(ctx context.Context, chainID string, keys map[string]string, signerManager *signer_service.SignerManager, signerWalletID string) (signing.Signer, error) {
	if signerManager != nil {
		lmt.Logger(ctx).Info("Using remote signer for Cosmos chain", zap.String("chain_id", chainID), zap.String("wallet_id", signerWalletID))
		txConfig := utils.DefaultTxConfig()
		return NewRemoteCosmosSigner(signerManager, signerWalletID, chainID, txConfig), nil

	}
	lmt.Logger(ctx).Info("Remote Signing not configured - using local signer for Cosmos chain", zap.String("chain_id", chainID))
	privateKeyStr, ok := keys[chainID]
	if !ok {
		return nil, fmt.Errorf("private key not found for chain %s", chainID)
	}
	if privateKeyStr[:2] == "0x" {
		privateKeyStr = privateKeyStr[2:]
	}

	privateKeyBytes, err := hex.DecodeString(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("hex decoding private key bytes to string: %w", err)
	}

	var privateKey secp256k1.PrivKey
	if err := privateKey.UnmarshalAmino(privateKeyBytes); err != nil {
		return nil, fmt.Errorf("unmarshaling private key bytes to amino: %w", err)
	}

	return signing.NewLocalCosmosSigner(&privateKey), nil
}
