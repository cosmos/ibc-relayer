package tmrpc

import (
	"context"
	"sync"
)

type TendermintRPCClientManager interface {
	GetClient(ctx context.Context, chainID string) (TendermintRPCQuerier, error)
}

type tendermintRPCClientManagerImpl struct {
	clients map[string]TendermintRPCQuerier
	m       sync.Mutex
}

func NewTendermintRPCClientManager() TendermintRPCClientManager {
	m := &tendermintRPCClientManagerImpl{
		clients: make(map[string]TendermintRPCQuerier),
	}
	return m
}

func (m *tendermintRPCClientManagerImpl) GetClient(ctx context.Context, chainID string) (TendermintRPCQuerier, error) {
	m.m.Lock()
	defer m.m.Unlock()
	if _, ok := m.clients[chainID]; !ok {
		q, err := DefaultTendermintRPCQuerier(ctx, chainID)
		if err != nil {
			return nil, err
		}
		m.clients[chainID] = q
	}
	return m.clients[chainID], nil
}
