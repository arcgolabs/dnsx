package dnsserver

import (
	"time"

	"github.com/samber/hot"
)

type responseCache struct {
	cache *hot.HotCache[queryCacheKey, cachedResponse]
}

func newResponseCache(config CacheConfig) *responseCache {
	if config.Capacity <= 0 {
		config.Capacity = 1
	}
	if config.Algorithm == "" {
		config.Algorithm = CacheLRU
	}

	builder := hot.NewHotCache[queryCacheKey, cachedResponse](
		toHotAlgorithm(config.Algorithm),
		config.Capacity,
	)
	if config.Janitor {
		builder = builder.WithJanitor()
	}
	if config.MissingCapacity > 0 {
		algorithm := config.MissingAlgorithm
		if algorithm == "" {
			algorithm = config.Algorithm
		}
		builder = builder.WithMissingCache(toHotAlgorithm(algorithm), config.MissingCapacity)
	}

	return &responseCache{
		cache: builder.Build(),
	}
}

func (c *responseCache) Get(key queryCacheKey) (cachedResponse, bool) {
	var zero cachedResponse
	if c == nil || c.cache == nil {
		return zero, false
	}

	value, found, err := c.cache.Get(key)
	if err != nil || !found {
		return zero, false
	}

	return value, true
}

func (c *responseCache) Set(key queryCacheKey, value cachedResponse, ttl time.Duration) {
	if c == nil || c.cache == nil || ttl <= 0 {
		return
	}

	c.cache.SetWithTTL(key, value, ttl)
}

func toHotAlgorithm(algorithm CacheAlgorithm) hot.EvictionAlgorithm {
	switch algorithm {
	case CacheLFU:
		return hot.LFU
	case CacheTinyLFU:
		return hot.TinyLFU
	case CacheWTinyLFU:
		return hot.WTinyLFU
	case CacheTwoQueue:
		return hot.TwoQueue
	case CacheARC:
		return hot.ARC
	case CacheFIFO:
		return hot.FIFO
	case CacheSIEVE:
		return hot.SIEVE
	case CacheLRU:
		fallthrough
	default:
		return hot.LRU
	}
}
