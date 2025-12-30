package cleanup

import (
	"context"
	"sync"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"go.uber.org/zap"
)

type Manager struct {
	logger     *zap.Logger
	db         *database.DB
	maxSize    int64
	slackRatio float64
	trigger    chan struct{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewManager(logger *zap.Logger, db *database.DB, maxSize int64, slackRatio float64) *Manager {
	if slackRatio <= 0 {
		slackRatio = 0.2 // Default 20%
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		logger:     logger,
		db:         db,
		maxSize:    maxSize,
		slackRatio: slackRatio,
		trigger:    make(chan struct{}, 1),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (m *Manager) Start() {
	m.wg.Add(1)
	go m.run()
}

func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) NotifyWrite() {
	select {
	case m.trigger <- struct{}{}:
	default:
		// Already triggered
	}
}

func (m *Manager) run() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.trigger:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	currentSize, err := m.db.GetCacheSize(m.ctx)
	if err != nil {
		m.logger.Error("failed to get cache size", zap.Error(err))
		return
	}

	if currentSize > m.maxSize {
		targetSize := int64(float64(m.maxSize) * (1.0 - m.slackRatio))
		toFree := currentSize - targetSize
		if toFree > 0 {
			freed, err := m.db.PruneCache(m.ctx, toFree)
			if err != nil {
				m.logger.Error("failed to prune cache", zap.Error(err))
			} else {
				m.logger.Info("pruned cache", zap.Int64("freed_bytes", freed))
			}
		}
	}
}
