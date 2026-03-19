package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/messagequeue"
)

var redisConnString = flag.String("redis", "redis://redis:6379/0", "redis connection string")

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lmt.ConfigureLogger()
	ctx = lmt.LoggerContext(ctx)

	redisConfig, err := redis.ParseURL(*redisConnString)
	if err != nil {
		lmt.Logger(ctx).Fatal("Unable to parse redis connection string", zap.Error(err))
	}
	redisClient := redis.NewClient(redisConfig)

	if err := redisClient.Ping(ctx).Err(); err != nil {
		lmt.Logger(ctx).Fatal("Unable to ping redis", zap.Error(err))
	}

	eg, ctx := errgroup.WithContext(ctx)
	for i := range 10 {
		eg.Go(func() error {
			mq, err := messagequeue.NewMsgQueue(ctx, redisClient, "messages", "messages-group", fmt.Sprintf("message-producer-%d", i))
			if err != nil {
				lmt.Logger(ctx).Error("Unable to create message queue", zap.Int("i", i), zap.Error(err))
				return err
			}

			for range 10 {
				_, err = mq.Push(ctx, map[string]interface{}{"message": fmt.Sprintf("message-%d", i), "number": rand.Intn(1000000)})
				if err != nil {
					lmt.Logger(ctx).Error("Unable to push message to queue", zap.Int("i", i), zap.Error(err))
					return err
				}
			}

			lmt.Logger(ctx).Info("Finished pushing messages to queue", zap.Int("i", i))
			return nil
		})
	}

	for i := range 3 {
		eg.Go(func() error {
			mq, err := messagequeue.NewMsgQueue(ctx, redisClient, "messages", "messages-group", fmt.Sprintf("message-consumer-%d", i))
			if err != nil {
				lmt.Logger(ctx).Error("Unable to create message queue", zap.Int("i", i), zap.Error(err))
				return err
			}

			for {
				messages, err := mq.Pop(ctx, 1, 15*time.Millisecond)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					lmt.Logger(ctx).Error("Unable to pop messages from queue", zap.Int("i", i), zap.Error(err))
					return err
				}

				lmt.Logger(ctx).Info("Popped messages from queue", zap.Int("i", i), zap.Int("messages", len(messages)))

				ids := make([]string, 0, len(messages))
				for _, message := range messages {
					// Simulate flakiness
					x := rand.Float64()
					if x < 0.1 {
						lmt.Logger(ctx).Warn("Dropping message", zap.String("id", message.ID))
						continue // drop packet
					} else if x < 0.35 {
						lmt.Logger(ctx).Warn("Delaying message", zap.String("id", message.ID))
						time.Sleep(150 * time.Millisecond) // service disappears then comes back and completes
					}

					ids = append(ids, message.ID)
				}

				if len(ids) > 0 {
					err = mq.Ack(ctx, ids)
					if err != nil {
						lmt.Logger(ctx).Error("Unable to ack messages", zap.Int("i", i), zap.Error(err))
						return err
					}
				}
			}
		})
	}

	if err := eg.Wait(); err != nil {
		lmt.Logger(ctx).Fatal("Error running message queue", zap.Error(err))
	}
}
