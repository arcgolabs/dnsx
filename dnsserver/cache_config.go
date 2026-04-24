package dnsserver

type CacheAlgorithm string

const (
	CacheLRU      CacheAlgorithm = "lru"
	CacheLFU      CacheAlgorithm = "lfu"
	CacheTinyLFU  CacheAlgorithm = "tinylfu"
	CacheWTinyLFU CacheAlgorithm = "wtinylfu"
	CacheTwoQueue CacheAlgorithm = "2q"
	CacheARC      CacheAlgorithm = "arc"
	CacheFIFO     CacheAlgorithm = "fifo"
	CacheSIEVE    CacheAlgorithm = "sieve"
)

type CacheConfig struct {
	Capacity         int
	Algorithm        CacheAlgorithm
	Janitor          bool
	MissingCapacity  int
	MissingAlgorithm CacheAlgorithm
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Capacity:  1024,
		Algorithm: CacheLRU,
	}
}
