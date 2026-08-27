package adapter

import (
	"context"
	"sync/atomic"

	rds "github.com/redis/go-redis/v9"
)

func pubSubNumSub(ctx context.Context, client rds.UniversalClient, sharded bool, channel string) (int64, error) {
	var count atomic.Int64
	visit := func(ctx context.Context, client *rds.Client) error {
		value, err := subscriberCount(ctx, client, sharded, channel)
		if err == nil {
			count.Add(value)
		}
		return err
	}

	var err error
	switch client := client.(type) {
	case *rds.ClusterClient:
		if sharded {
			master, resolveErr := client.MasterForKey(ctx, channel)
			if resolveErr != nil {
				return 0, resolveErr
			}
			return subscriberCount(ctx, master, true, channel)
		}
		// Normal Pub/Sub subscriptions may live on any cluster node.
		err = client.ForEachShard(ctx, visit)
	default:
		return subscriberCount(ctx, client, sharded, channel)
	}
	return count.Load(), err
}

func subscriberCount(ctx context.Context, client rds.Cmdable, sharded bool, channel string) (int64, error) {
	if sharded {
		result, err := client.PubSubShardNumSub(ctx, channel).Result()
		return result[channel], err
	}
	result, err := client.PubSubNumSub(ctx, channel).Result()
	return result[channel], err
}
