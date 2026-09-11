package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	lockKeyPrefix  = "lock:site:"
	fenceKeyPrefix = "fence:site:"
)

// Lua script to release lock only if token matches
const releaseLuaScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`

// Lua script to renew lock only if token matches
const renewLuaScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("pexpire", KEYS[1], ARGV[2])
else
    return 0
end
`

// Acquisition and fence allocation are one Redis operation: a paused acquirer
// must not obtain a newer fence after its separately acquired lease expires.
const acquireLuaScript = `
if redis.call('EXISTS', KEYS[1]) ~= 0 then return 0 end
local previous = redis.call('GET', KEYS[2])
-- Conservative bound below Lua cjson's 14-significant-digit serialization limit.
if previous and tonumber(previous) >= 9999999999999 then return redis.error_reply('fencing counter exhausted') end
local fence = redis.call('INCR', KEYS[2])
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return fence
`

type RedisLockManager struct {
	client *redis.Client
}

func NewRedisLockManager(addr, password string, db int) (*RedisLockManager, error) {
	client := redis.NewClient(&redis.Options{
		Addr:                  addr,
		Password:              password,
		DB:                    db,
		ContextTimeoutEnabled: true,
		DialTimeout:           5 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
		PoolTimeout: 3 * time.Second, PoolSize: 16, MaxRetries: -1,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisLockManager{
		client: client,
	}, nil
}

func (r *RedisLockManager) Acquire(ctx context.Context, siteID string, ttl time.Duration) (string, int64, error) {
	lockKey := lockKeyPrefix + siteID
	fenceKey := fenceKeyPrefix + siteID
	token := uuid.New().String()

	if ttl.Milliseconds() < 1 {
		return "", 0, fmt.Errorf("lock TTL must be positive")
	}
	fencingToken, err := r.client.Eval(ctx, acquireLuaScript, []string{lockKey, fenceKey}, token, ttl.Milliseconds()).Int64()
	if err != nil {
		return "", 0, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if fencingToken == 0 {
		return "", 0, fmt.Errorf("lock already held for site: %s", siteID)
	}

	return token, fencingToken, nil
}

func (r *RedisLockManager) Renew(ctx context.Context, siteID, token string, ttl time.Duration) error {
	if ttl.Milliseconds() < 1 {
		return fmt.Errorf("lock TTL must be positive")
	}
	lockKey := lockKeyPrefix + siteID
	ttlMs := ttl.Milliseconds()

	result, err := r.client.Eval(ctx, renewLuaScript, []string{lockKey}, token, ttlMs).Result()
	if err != nil {
		return fmt.Errorf("failed to renew lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock token mismatch or lock not held")
	}

	return nil
}

func (r *RedisLockManager) Release(ctx context.Context, siteID, token string) error {
	lockKey := lockKeyPrefix + siteID

	result, err := r.client.Eval(ctx, releaseLuaScript, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock token mismatch or lock not held")
	}

	return nil
}

func (r *RedisLockManager) Close() error {
	return r.client.Close()
}
