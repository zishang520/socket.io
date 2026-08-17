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
		// Normal Pub/Sub may be placed on any cluster node;
		// sharded subscriptions are explicitly pinned to slot masters.
		if sharded {
			err = client.ForEachMaster(ctx, visit)
		} else {
			err = client.ForEachShard(ctx, visit)
		}
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
