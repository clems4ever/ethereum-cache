package exporter_test

import (
	"context"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/exporter"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExporter(t *testing.T) {
	// 1. Setup DB
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Insert Data
	ctx := context.Background()
	// Item 1: 9 bytes + 64 overhead = 73 bytes
	err = db.SetCachedRPCResult(ctx, "key1", "method1", []byte("response1"))
	require.NoError(t, err)
	// Item 2: 9 bytes + 64 overhead = 73 bytes
	err = db.SetCachedRPCResult(ctx, "key2", "method1", []byte("response2"))
	require.NoError(t, err)

	// Total expected size: 146 bytes
	// Total expected count: 2

	// 3. Start Exporter
	exp := exporter.New(zap.NewNop(), db, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go exp.Start(ctx)

	// 4. Verify Metrics
	require.Eventually(t, func() bool {
		count := getMetricValue("ethereum_cache_items_count")
		size := getMetricValue("ethereum_cache_size_bytes")
		return count == 2 && size == 146
	}, 2*time.Second, 50*time.Millisecond, "Metrics did not reach expected values")
}

func getMetricValue(name string) float64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return -1
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			if len(mf.GetMetric()) > 0 {
				return mf.GetMetric()[0].GetGauge().GetValue()
			}
		}
	}
	return -1
}
