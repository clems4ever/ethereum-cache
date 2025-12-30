package cleanup

import (
	"context"
	"log"
	"sync"

	"github.com/clems4ever/ethereum-cache/internal/database"
)

type Manager struct {
	db         *database.DB
	maxSize    int64
	slackRatio float64
	trigger    chan struct{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewManager(db *database.DB, maxSize int64, slackRatio float64) *Manager {
	if slackRatio <= 0 {
		slackRatio = 0.2 // Default 20%
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
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
		log.Printf("failed to get cache size: %v", err)
		return
	}

	if currentSize > m.maxSize {
		targetSize := int64(float64(m.maxSize) * (1.0 - m.slackRatio))
		toFree := currentSize - targetSize
		if toFree > 0 {
			freed, err := m.db.PruneCache(m.ctx, toFree)
			if err != nil {
				log.Printf("failed to prune cache: %v", err)
			} else {
				log.Printf("pruned cache: freed %d bytes", freed)
			}
		}
	}
}
