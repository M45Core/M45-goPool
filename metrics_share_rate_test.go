package main

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestPoolShareRateSnapshotConcurrentAcceptedShares(t *testing.T) {
	metrics := &PoolMetrics{}
	metrics.RecordShare(true, "")
	const updates = 10000
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for range updates {
			metrics.RecordShare(true, "")
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range updates {
			perSecond, perMinute := metrics.SnapshotShareRates(time.Now())
			if perSecond < 0 || math.IsNaN(perSecond) || math.IsInf(perSecond, 0) || perMinute != perSecond*60 {
				t.Errorf("invalid share rates: perSecond=%v perMinute=%v", perSecond, perMinute)
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	accepted, rejected, _ := metrics.Snapshot()
	if accepted != updates+1 || rejected != 0 {
		t.Fatalf("pool totals = (%d, %d), want (%d, 0)", accepted, rejected, updates+1)
	}
}
