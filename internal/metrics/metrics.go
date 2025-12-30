package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ethereum_cache_hits_total",
		Help: "The total number of cache hits",
	}, []string{"method"})

	CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ethereum_cache_misses_total",
		Help: "The total number of cache misses",
	}, []string{"method"})

	CacheSizeBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ethereum_cache_size_bytes",
		Help: "The current size of the cache in bytes",
	})

	CacheItemsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ethereum_cache_items_count",
		Help: "The current number of items in the cache",
	})
)
