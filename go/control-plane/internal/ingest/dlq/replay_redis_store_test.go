package dlq

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type fakeReplayRedisClient struct {
	values  map[string]string
	getErr  error
	setErr  error
	setTTL  time.Duration
	setKey  string
	setJSON string
}

func newFakeReplayRedisClient() *fakeReplayRedisClient {
	return &fakeReplayRedisClient{values: make(map[string]string)}
}

func (f *fakeReplayRedisClient) Get(_ context.Context, key string) *redis.StringCmd {
	if f.getErr != nil {
		return redis.NewStringResult("", f.getErr)
	}
	value, ok := f.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(value, nil)
}

func (f *fakeReplayRedisClient) Set(_ context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	if f.setErr != nil {
		return redis.NewStatusResult("", f.setErr)
	}
	f.setKey = key
	f.setTTL = expiration
	switch typed := value.(type) {
	case []byte:
		f.setJSON = string(typed)
	default:
		f.setJSON = fmt.Sprint(value)
	}
	f.values[key] = f.setJSON
	return redis.NewStatusResult("OK", nil)
}

func TestRedisReplayIdempotencyStoreRoundTrip(t *testing.T) {
	client := newFakeReplayRedisClient()
	store := newRedisReplayIdempotencyStore(client, "test:dlq:", time.Hour, zap.NewNop())
	result := ReplayResult{
		ReplayID:       "dlq-replay-1",
		Status:         ReplayStatusCompleted,
		TenantID:       "tenant-a",
		IdempotencyKey: "tenant-a:approval-1",
	}

	if err := store.Put(context.Background(), result.IdempotencyKey, result); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if client.setTTL != time.Hour {
		t.Fatalf("ttl=%s want 1h", client.setTTL)
	}
	if client.setKey == "test:dlq:"+result.IdempotencyKey {
		t.Fatalf("redis key should hash the idempotency key, got raw key %q", client.setKey)
	}

	got, ok, err := store.Get(context.Background(), result.IdempotencyKey)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok {
		t.Fatalf("Get should find stored result")
	}
	if got.ReplayID != result.ReplayID || got.Status != result.Status {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestRedisReplayIdempotencyStoreMissAndErrors(t *testing.T) {
	store := newRedisReplayIdempotencyStore(newFakeReplayRedisClient(), "test:dlq:", time.Hour, zap.NewNop())
	_, ok, err := store.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("missing Get returned error: %v", err)
	}
	if ok {
		t.Fatalf("missing key should not be found")
	}

	store = newRedisReplayIdempotencyStore(&fakeReplayRedisClient{getErr: errors.New("redis down")}, "test:dlq:", time.Hour, zap.NewNop())
	_, _, err = store.Get(context.Background(), "key")
	if err == nil {
		t.Fatalf("Get should return redis errors")
	}

	store = newRedisReplayIdempotencyStore(&fakeReplayRedisClient{setErr: errors.New("redis down")}, "test:dlq:", time.Hour, zap.NewNop())
	err = store.Put(context.Background(), "key", ReplayResult{ReplayID: "dlq-replay-1"})
	if err == nil {
		t.Fatalf("Put should return redis errors")
	}
}
