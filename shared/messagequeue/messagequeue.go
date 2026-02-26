package messagequeue

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/shared/lmt"
)

type MessageQueue struct {
	redis    *redis.Client
	stream   string
	group    string
	consumer string
}

func NewMsgQueue(
	ctx context.Context,
	redisClient *redis.Client,
	stream string,
	group string,
	consumer string,
) (*MessageQueue, error) {
	_, err := redisClient.XGroupCreateMkStream(ctx, stream, group, "$").Result()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, err
	}

	return &MessageQueue{
		redis:    redisClient,
		stream:   stream,
		group:    group,
		consumer: consumer,
	}, nil
}

func (mq *MessageQueue) Push(
	ctx context.Context,
	values map[string]interface{},
) (string, error) {
	id, err := mq.redis.XAdd(ctx, &redis.XAddArgs{Stream: mq.stream, Values: values}).Result()
	if err != nil {
		lmt.Logger(ctx).Error("Unable to push message to queue", zap.Error(err))
		return "", err
	}
	return id, nil
}

func (mq *MessageQueue) Pop(
	ctx context.Context,
	count int64,
	timeout time.Duration,
) ([]redis.XMessage, error) {
	claimResult, err := mq.Claim(ctx, count, timeout)
	if err != nil {
		return nil, err
	}
	messages := claimResult.Messages

	if len(messages) >= int(count) {
		return messages, nil
	}

	readResult, err := mq.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    mq.group,
		Consumer: mq.consumer,
		Streams:  []string{mq.stream, ">"},
		Block:    5 * time.Second,
		Count:    count - int64(len(messages)),
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		lmt.Logger(ctx).Error("Unable to read message from queue", zap.Error(err))
		return nil, err
	}

	messages = append(messages, readResult[0].Messages...)

	return messages, nil
}

func (mq *MessageQueue) Claim(
	ctx context.Context,
	count int64,
	timeout time.Duration,
) (redis.XStream, error) {
	result, _, err := mq.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   mq.stream,
		Group:    mq.group,
		Consumer: mq.consumer,
		MinIdle:  timeout,
		Start:    "0",
		Count:    count,
	}).Result()
	if err != nil {
		lmt.Logger(ctx).Error("Unable to claim message from queue", zap.Error(err))
		return redis.XStream{}, err
	}

	return redis.XStream{
		Stream:   mq.stream,
		Messages: result,
	}, nil
}

func (mq *MessageQueue) Ack(ctx context.Context, ids []string) error {
	_, err := mq.redis.XAck(ctx, mq.stream, mq.group, ids...).Result()
	if err != nil {
		lmt.Logger(ctx).Error("Unable to ack message in queue", zap.Error(err))
	}
	return err
}
