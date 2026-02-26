package transactionqueue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cosmos/platform-relayer/shared/messagequeue"
)

type Transaction struct {
	ChainID string
	TxHash  string
	ID      string
}

type ConsumerTransactionQueue interface {
	Pop(ctx context.Context, count int) ([]*Transaction, error)
	Ack(ctx context.Context, transaction *Transaction) error
}

type ProducerTransactionQueue interface {
	Push(ctx context.Context, txHash, chainID string) error
}

func NewConsumerTransactionQueue(ctx context.Context, redisClient *redis.Client, consumerID string, timeout time.Duration) (ConsumerTransactionQueue, error) {
	messageQueue, err := messagequeue.NewMsgQueue(ctx, redisClient, "transactions", "transactions-group", consumerID) // TODO: stream name should potentially be a config param
	if err != nil {
		return nil, err
	}
	return &consumerTransactionQueueImpl{
		timeout:      timeout,
		messageQueue: messageQueue,
	}, nil
}

type consumerTransactionQueueImpl struct {
	timeout      time.Duration
	messageQueue *messagequeue.MessageQueue
}

func (t *consumerTransactionQueueImpl) Pop(ctx context.Context, count int) ([]*Transaction, error) {
	messages, err := t.messageQueue.Pop(ctx, int64(count), t.timeout)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	var transactions []*Transaction
	for _, message := range messages {
		transactions = append(transactions, &Transaction{
			ChainID: message.Values["chain_id"].(string),
			TxHash:  message.Values["tx_hash"].(string),
			ID:      message.ID,
		})
	}
	return transactions, nil
}

func (t *consumerTransactionQueueImpl) Ack(ctx context.Context, transaction *Transaction) error {
	return t.messageQueue.Ack(ctx, []string{transaction.ID})
}

func NewProducerTransactionQueue(ctx context.Context, redisClient *redis.Client) (ProducerTransactionQueue, error) {
	messageQueue, err := messagequeue.NewMsgQueue(ctx, redisClient, "transactions", "transactions-group", "producer")
	if err != nil {
		return nil, err
	}
	return &producerTransactionQueueImpl{
		messageQueue: messageQueue,
	}, nil
}

type producerTransactionQueueImpl struct {
	messageQueue *messagequeue.MessageQueue
}

func (t *producerTransactionQueueImpl) Push(ctx context.Context, txHash, chainID string) error {
	_, err := t.messageQueue.Push(ctx, map[string]interface{}{
		"tx_hash":  txHash,
		"chain_id": chainID,
	})
	return err
}
