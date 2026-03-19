package evmrpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/ethclient"
	ethereumrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/cosmos/ibc-relayer/shared/config"
)

type EVMRPCClientManager interface {
	GetClient(ctx context.Context, chainID string) (EVMChainRPC, error)
}

type EVMRPCClientManagerImpl struct {
	clients map[string]EVMChainRPC
	m       sync.Mutex
}

func NewEVMRPCClientManager() EVMRPCClientManager {
	m := &EVMRPCClientManagerImpl{
		clients: make(map[string]EVMChainRPC),
	}
	return m
}

func (m *EVMRPCClientManagerImpl) GetClient(ctx context.Context, chainID string) (EVMChainRPC, error) {
	m.m.Lock()
	defer m.m.Unlock()
	if _, ok := m.clients[chainID]; !ok {
		rpc, err := config.GetConfigReader(ctx).GetRPCEndpoint(chainID)
		if err != nil {
			return nil, err
		}

		basicAuth, err := config.GetConfigReader(ctx).GetBasicAuth(chainID)
		if err != nil {
			return nil, err
		}

		conn, err := ethereumrpc.DialContext(ctx, rpc)
		if err != nil {
			return nil, err
		}
		if basicAuth != nil {
			conn.SetHeader("Authorization", fmt.Sprintf("Basic %s", *basicAuth))
		}

		client := NewEVMChainRPC(ethclient.NewClient(conn))
		m.clients[chainID] = client
	}

	return m.clients[chainID], nil
}
