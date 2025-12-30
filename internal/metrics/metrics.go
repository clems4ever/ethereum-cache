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
)
