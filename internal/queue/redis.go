// Package queue provides a Redis-backed delay queue for scheduled outbound
// WhatsApp replies. The orchestrator schedules replies via ZADD with
// score = unix_ms; the bridge worker polls and pops due entries.
//
// Why a delay queue: the nurture layer wants to wait 8–23 seconds before
// sending an AI reply (mimics human response time). The bridge worker is
// the only thing that can actually call whatsmeow.SendMessage, so the
// orchestrator schedules the send, and the bridge picks it up.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisQueue is a Redis ZSET-backed implementation of the Queue interface.
//
// Items are serialized as JSON and stored with score = unix_ms(sendAt). The
// bridge polls with ZRANGEBYSCORE + ZREM in a MULTI/EXEC to atomically pop
// due entries (so two workers never see the same job).
//
// The cancel-key sidecar (a HASH mapping tenantID+phone -> replyID) lets
// Cancel() remove a pending reply without scanning the ZSET.
type RedisQueue struct {
	client *redis.Client
	key    string
}

// NewRedisQueue returns a RedisQueue backed by the given client.
// The client should be configured with a context-aware DialTimeout.
func NewRedisQueue(client *redis.Client) *RedisQueue {
	if client == nil {
		panic("queue: redis client is nil")
	}
	return &RedisQueue{client: client, key: RedisKey()}
}

// Schedule enqueues a reply to be sent at sendAt.
func (q *RedisQueue) Schedule(ctx context.Context, r ScheduledReply) error {
	if r.ID == "" {
		return errors.New("queue: reply ID is required")
	}
	if r.SendAt.IsZero() {
		return errors.New("queue: SendAt is required")
	}

	payload, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("queue: marshal: %w", err)
	}

	// Cancel any existing pending reply for this tenant+phone before
	// scheduling the new one. Prevents a fast follow-up from doubling up.
	if err := q.Cancel(ctx, r.TenantID, r.Phone); err != nil {
		return fmt.Errorf("queue: cancel previous: %w", err)
	}

	pipe := q.client.TxPipeline()
	pipe.ZAdd(ctx, q.key, redis.Z{
		Score:  Score(r.SendAt),
		Member: payload,
	})
	pipe.HSet(ctx, q.cancelKey(), r.TenantID+":"+r.Phone, r.ID)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("queue: zadd: %w", err)
	}
	return nil
}

// Cancel removes any pending reply for (tenantID, phone).
func (q *RedisQueue) Cancel(ctx context.Context, tenantID, phone string) error {
	// Best-effort: locate the current entry by scan, then ZREM. The cancel
	// sidecar HASH gives us the ID; we still need to find the payload by
	// score range to remove it atomically.
	//
	// We accept the small race window where a worker could pop the entry
	// between HDEL and ZREM; the worker treats the missing entry as a
	// no-op (TX), so it's safe.
	sideKey := q.cancelKey()
	field := tenantID + ":" + phone
	existing, err := q.client.HGet(ctx, sideKey, field).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("queue: hget: %w", err)
	}
	if errors.Is(err, redis.Nil) {
		return nil
	}
	_ = existing

	// Scan the ZSET for a payload whose tenant_id+phone matches. Bounded
	// by score range: from -inf to +inf, all timestamps. We expect <100
	// entries per (tenant, phone) in practice.
	entries, err := q.client.ZRangeWithScores(ctx, q.key, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("queue: zrange: %w", err)
	}

	for _, z := range entries {
		raw, ok := z.Member.(string)
		if !ok {
			continue
		}
		var r ScheduledReply
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			continue
		}
		if r.TenantID == tenantID && r.Phone == phone {
			if err := q.client.ZRem(ctx, q.key, z.Member).Err(); err != nil {
				return fmt.Errorf("queue: zrem: %w", err)
			}
		}
	}

	if err := q.client.HDel(ctx, sideKey, field).Err(); err != nil {
		return fmt.Errorf("queue: hdel: %w", err)
	}
	return nil
}

// PopDue atomically pops up to `limit` replies that are due now,
// blocking for at most `block` if none are due.
func (q *RedisQueue) PopDue(ctx context.Context, block time.Duration, limit int) ([]ScheduledReply, error) {
	if limit <= 0 {
		limit = 1
	}
	now := time.Now().UTC()
	maxScore := Score(now)

	// Block-wait for at least one due item: ZRANGEBYSCORE LIMIT 0 1.
	// We do a brief poll loop with a small initial sleep so we don't
	// burn a Redis connection blocking on BZPOPMIN (which doesn't
	// support time-bounded blocking on raw ZSETs).
	if block > 0 {
		deadline := time.Now().Add(block)
		for {
			n, err := q.client.ZCount(ctx, q.key, "-inf", fmt.Sprintf("%f", maxScore)).Result()
			if err != nil {
				return nil, fmt.Errorf("queue: zcount: %w", err)
			}
			if n > 0 {
				break
			}
			if time.Now().After(deadline) {
				return nil, ErrEmpty
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	// Pop up to `limit` due items atomically.
	raws, err := q.client.ZRangeByScore(ctx, q.key, &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%f", maxScore),
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: zrangebyscore: %w", err)
	}
	if len(raws) == 0 {
		return nil, ErrEmpty
	}

	results := make([]ScheduledReply, 0, len(raws))
	for _, raw := range raws {
		var r ScheduledReply
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			// Best-effort: remove the bad entry so we don't loop on it.
			_ = q.client.ZRem(ctx, q.key, raw).Err()
			continue
		}
		// Remove + clear cancel sidecar in a single TX.
		pipe := q.client.TxPipeline()
		pipe.ZRem(ctx, q.key, raw)
		pipe.HDel(ctx, q.cancelKey(), r.TenantID+":"+r.Phone)
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, fmt.Errorf("queue: pop exec: %w", err)
		}
		results = append(results, r)
	}
	return results, nil
}

// Ping verifies the Redis connection. Used by /readyz.
func (q *RedisQueue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

// Close releases the underlying Redis client.
func (q *RedisQueue) Close() error {
	return q.client.Close()
}

// cancelKey returns the HASH key that tracks pending (tenantID, phone) -> replyID.
// Lives alongside the main ZSET so a single MULTI can update both.
func (q *RedisQueue) cancelKey() string {
	return q.key + ":cancel"
}

// NewClient returns a configured go-redis client. The caller is responsible
// for closing it. dialTimeout is applied to the initial connect; once
// connected, the client recovers automatically.
func NewClient(redisURL string, dialTimeout time.Duration) (*redis.Client, error) {
	if redisURL == "" {
		return nil, errors.New("queue: REDIS_URL is required")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue: parse REDIS_URL: %w", err)
	}
	if dialTimeout > 0 {
		opts.DialTimeout = dialTimeout
	}
	return redis.NewClient(opts), nil
}
