package exporter

import (
	"context"
	"log"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/metrics"
)

type Exporter struct {
	db       *database.DB
	interval time.Duration
}

func New(db *database.DB, interval time.Duration) *Exporter {
	return &Exporter{
		db:       db,
		interval: interval,
	}
}

func (e *Exporter) Start(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	// Run immediately
	e.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.collect(ctx)
		}
	}
}

func (e *Exporter) collect(ctx context.Context) {
	size, err := e.db.GetCacheSize(ctx)
	if err != nil {
		log.Printf("failed to get cache size: %v", err)
	} else {
		metrics.CacheSizeBytes.Set(float64(size))
	}

	count, err := e.db.GetCacheItemCount(ctx)
	if err != nil {
		log.Printf("failed to get cache item count: %v", err)
	} else {
		metrics.CacheItemsCount.Set(float64(count))
	}
}
