package exporter

import (
	"context"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/metrics"
	"go.uber.org/zap"
)

type Exporter struct {
	logger   *zap.Logger
	db       *database.DB
	interval time.Duration
}

func New(logger *zap.Logger, db *database.DB, interval time.Duration) *Exporter {
	return &Exporter{
		logger:   logger,
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
		e.logger.Error("failed to get cache size", zap.Error(err))
	} else {
		metrics.CacheSizeBytes.Set(float64(size))
	}

	count, err := e.db.GetCacheItemCount(ctx)
	if err != nil {
		e.logger.Error("failed to get cache item count", zap.Error(err))
	} else {
		metrics.CacheItemsCount.Set(float64(count))
	}
}
