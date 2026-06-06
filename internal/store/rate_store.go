package store

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"walkietalk-go/internal/config"
	"walkietalk-go/internal/util"
)

const redisRateLua = `
local key    = KEYS[1]
local cutoff = ARGV[1]
local member = ARGV[2]
local score  = ARGV[3]
local limit  = tonumber(ARGV[4])
local ttl    = tonumber(ARGV[5])
redis.call('zremrangebyscore', key, '-inf', cutoff)
local count = redis.call('zcard', key)
if count >= limit then return 0 end
redis.call('zadd', key, score, member)
redis.call('expire', key, ttl)
return 1
`

type RateStore struct {
	cfg config.Config
	log *log.Logger
	rdb *redis.Client

	mu                sync.Mutex
	local             map[string][]time.Time
	redisFailures     int
	redisCircuitUntil time.Time
}

func NewRateStore(ctx context.Context, cfg config.Config, logger *log.Logger) *RateStore {
	rs := &RateStore{cfg: cfg, log: logger, local: make(map[string][]time.Time)}
	if cfg.RedisEnabled && cfg.RedisURL != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			logger.Printf("redis url invalid: %v", err)
			return rs
		}
		opt.DialTimeout = 5 * time.Second
		opt.ReadTimeout = 5 * time.Second
		opt.WriteTimeout = 5 * time.Second
		rdb := redis.NewClient(opt)
		if err := rdb.Ping(ctx).Err(); err != nil {
			logger.Printf("redis unavailable (%v) - local rate mode", err)
			_ = rdb.Close()
			return rs
		}
		rs.rdb = rdb
		logger.Printf("redis connected url=%s", util.RedactURL(cfg.RedisURL))
	}
	return rs
}

func (s *RateStore) Close() {
	if s.rdb != nil {
		_ = s.rdb.Close()
	}
}

func (s *RateStore) RedisOK(ctx context.Context) bool {
	if s.rdb == nil || s.redisCircuitOpen() {
		return false
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		s.markRedisFailure(err)
		return false
	}
	s.markRedisSuccess()
	return true
}

func (s *RateStore) Check(ctx context.Context, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return false
	}
	if s.rdb != nil && !s.redisCircuitOpen() {
		now := float64(time.Now().UnixNano()) / 1e9
		ttl := int(window.Seconds() * 2)
		member := util.RandomID("r_")
		res, err := s.rdb.Eval(ctx, redisRateLua, []string{"wt:rate:" + key}, now-window.Seconds(), member, now, limit, ttl).Int()
		if err == nil {
			s.markRedisSuccess()
			return res == 1
		}
		s.markRedisFailure(err)
	}
	return s.checkLocal(key, limit, window)
}

func (s *RateStore) checkLocal(key string, limit int, window time.Duration) bool {
	now := time.Now()
	cutoff := now.Add(-window)
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.local[key]
	kept := items[:0]
	for _, t := range items {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		s.local[key] = kept
		return false
	}
	kept = append(kept, now)
	s.local[key] = kept
	if len(s.local) > 50000 {
		for k := range s.local {
			delete(s.local, k)
			if len(s.local) <= 45000 {
				break
			}
		}
	}
	return true
}

func (s *RateStore) redisCircuitOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.redisCircuitOpenLocked(time.Now())
}

func (s *RateStore) redisCircuitOpenLocked(now time.Time) bool {
	return !s.redisCircuitUntil.IsZero() && now.Before(s.redisCircuitUntil)
}

func (s *RateStore) markRedisSuccess() {
	s.mu.Lock()
	s.redisFailures = 0
	s.redisCircuitUntil = time.Time{}
	s.mu.Unlock()
}

func (s *RateStore) markRedisFailure(err error) {
	now := time.Now()
	shouldLog := false
	failures := 0
	until := time.Time{}
	s.mu.Lock()
	s.redisFailures++
	failures = s.redisFailures
	if s.redisFailures >= s.cfg.RedisFailureThreshold {
		s.redisCircuitUntil = now.Add(s.cfg.RedisCircuitOpenSecs)
		until = s.redisCircuitUntil
		shouldLog = true
	}
	s.mu.Unlock()
	if shouldLog {
		s.log.Printf("redis circuit opened until %s after %d failures: %v", until.Format(time.RFC3339), failures, err)
	}
}

func (s *RateStore) Stats() map[string]any {
	s.mu.Lock()
	localSize := len(s.local)
	failures := s.redisFailures
	circuitOpen := s.redisCircuitOpenLocked(time.Now())
	s.mu.Unlock()
	return map[string]any{
		"redis_enabled":              s.rdb != nil,
		"redis_circuit_open":         circuitOpen,
		"redis_consecutive_failures": failures,
		"local_rate_keys":            localSize,
	}
}
