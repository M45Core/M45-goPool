package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStatsCleanupConcurrentSharesPreservesPoolTotals(t *testing.T) {
	for _, tt := range []struct {
		name       string
		queueDepth int
	}{
		{name: "buffered", queueDepth: 8},
		{name: "unbuffered", queueDepth: 0},
		{name: "synchronous", queueDepth: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &PoolMetrics{}
			mc := &MinerConn{metrics: metrics}
			if tt.queueDepth >= 0 {
				mc.statsUpdates = make(chan statsUpdate, tt.queueDepth)
				mc.statsWg.Add(1)
				go mc.statsWorker()
			}
			t.Cleanup(mc.cleanup)
			atomic.StoreUint64(&mc.connectionSeq, 42)
			mc.initialEMAWindowDone.Store(true)
			metrics.UpdateConnectionHashrate(42, 12345)

			const producers = 4
			const sharesPerProducer = 2000
			start := make(chan struct{})
			firstShares := make(chan struct{}, producers)
			var wg sync.WaitGroup
			wg.Add(producers)
			for range producers {
				go func() {
					defer wg.Done()
					<-start
					for i := range sharesPerProducer {
						mc.recordShare("worker", i%2 == 0, 1000, 1000, "lowDiff", "hash", nil, time.Now())
						if i == 0 {
							firstShares <- struct{}{}
						}
					}
				}()
			}
			close(start)
			for range producers {
				<-firstShares
			}
			mc.cleanup()
			wg.Wait()

			// Model already-admitted submissions completing after disconnect.
			mc.recordShare("worker", true, 1000, 1000, "", "late-accepted", nil, time.Now())
			mc.recordShare("worker", false, 0, 0, "lowDiff", "late-rejected", nil, time.Now())
			accepted, rejected, _ := metrics.Snapshot()
			want := uint64(producers*sharesPerProducer/2 + 1)
			if accepted != want || rejected != want {
				t.Fatalf("pool totals = (%d, %d), want (%d, %d)", accepted, rejected, want, want)
			}
			if got := metrics.PoolHashrate(); got != 0 {
				t.Fatalf("disconnected miner restored pool hashrate=%v", got)
			}
			stats := mc.snapshotStats()
			if !stats.WindowStart.IsZero() || stats.WindowSubmissions != 0 || stats.WindowDifficulty != 0 {
				t.Fatalf("disconnected miner restored sampling window: %+v", stats)
			}
		})
	}
}
