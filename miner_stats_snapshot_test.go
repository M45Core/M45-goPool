package main

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitRTTPercentiles(t *testing.T) {
	var full [64]float64
	for i := range full {
		full[i] = float64(len(full) - i)
	}
	tests := []struct {
		name    string
		samples [64]float64
		count   int
		want50  float64
		want95  float64
	}{
		{name: "empty"},
		{name: "negative_count", count: -1},
		{name: "invalid_samples", samples: [64]float64{0, -1, math.NaN()}, count: 3},
		{name: "one_sample", samples: [64]float64{42}, count: 1, want50: 42, want95: 42},
		{name: "unsorted_and_filtered", samples: [64]float64{9, 0, 3, -1, 7, math.NaN(), 1, 5}, count: 8, want50: 5, want95: 7},
		{name: "ignore_unused_slots", samples: [64]float64{3, 1, 2, 100}, count: 3, want50: 2, want95: 2},
		{name: "clamp_count", samples: [64]float64{3, 1, 2}, count: 100, want50: 2, want95: 2},
		{name: "full", samples: full, count: len(full), want50: 32, want95: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.samples
			p50, p95 := submitRTTPercentilesLocked(tt.samples, tt.count)
			if p50 != tt.want50 || p95 != tt.want95 {
				t.Fatalf("percentiles = (%v, %v), want (%v, %v)", p50, p95, tt.want50, tt.want95)
			}
			for i := range before {
				if math.Float64bits(tt.samples[i]) != math.Float64bits(before[i]) {
					t.Fatalf("caller sample %d was changed", i)
				}
			}
		})
	}
}

func TestSnapshotShareInfo_WorkStartShowsLiveElapsedWhileAwaitingFirstShare(t *testing.T) {
	mc := &MinerConn{}
	mc.statsMu.Lock()
	mc.notifySentAt = time.Now().Add(-7 * time.Second)
	mc.notifyAwaitingFirstShare = true
	mc.statsMu.Unlock()

	snap := mc.snapshotShareInfo()
	if snap.NotifyToFirstShareMS < 6500 || snap.NotifyToFirstShareMS > 9000 {
		t.Fatalf("got %.2fms want live elapsed around 7000ms", snap.NotifyToFirstShareMS)
	}
}

func TestRecordShareDropsWhenStatsChannelClosed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	mc := &MinerConn{
		statsUpdates: make(chan statsUpdate),
	}
	mc.closeStatsUpdates()

	mc.recordShare("worker", true, 1, 2, "", "hash", nil, now)

	stats := mc.snapshotStats()
	if stats.Accepted != 0 || stats.WindowAccepted != 0 || stats.WindowSubmissions != 0 || stats.TotalDifficulty != 0 {
		t.Fatalf("recordShare updated stats after closed stats channel: %+v", stats)
	}
}

func TestCleanupCountsLateSharesWithoutRestoringPoolHashrate(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	metrics := NewPoolMetrics()
	mc := &MinerConn{
		metrics:      metrics,
		statsUpdates: make(chan statsUpdate),
	}
	atomic.StoreUint64(&mc.connectionSeq, 42)
	mc.initialEMAWindowDone.Store(true)
	metrics.UpdateConnectionHashrate(42, 12345)

	mc.cleanup()
	if got := metrics.PoolHashrate(); got != 0 {
		t.Fatalf("cleanup left pool hashrate=%v, want 0", got)
	}

	mc.recordShare("worker", true, 1_000_000, 1_000_000, "", "late-1", nil, now)
	mc.recordShare("worker", true, 1_000_000, 1_000_000, "", "late-2", nil, now.Add(time.Millisecond))

	if got := metrics.PoolHashrate(); got != 0 {
		t.Fatalf("late shares restored pool hashrate=%v after cleanup, want 0", got)
	}
	stats := mc.snapshotStats()
	if stats.Accepted != 0 || stats.WindowSubmissions != 0 || stats.TotalDifficulty != 0 {
		t.Fatalf("late shares updated cleaned-up stats: %+v", stats)
	}
	accepted, rejected, _ := metrics.Snapshot()
	if accepted != 2 || rejected != 0 {
		t.Fatalf("late share pool totals = (%d, %d), want (2, 0)", accepted, rejected)
	}
}

func TestEnsureWindowLocked_DoesNotResetByVardiffCadence(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	mc := &MinerConn{
		vardiff: VarDiffConfig{
			AdjustmentWindow: 10 * time.Second,
		},
	}
	mc.stats.WindowStart = now.Add(-3 * time.Minute)
	mc.stats.WindowAccepted = 12
	mc.stats.WindowSubmissions = 15
	mc.stats.WindowDifficulty = 42
	mc.stats.LastShare = now.Add(-5 * time.Second)

	mc.statsMu.Lock()
	mc.ensureWindowLocked(now)
	got := mc.stats
	mc.statsMu.Unlock()

	if got.WindowStart != now.Add(-3*time.Minute) || got.WindowAccepted != 12 || got.WindowSubmissions != 15 || got.WindowDifficulty != 42 {
		t.Fatalf("status window should not reset on vardiff cadence: start=%v accepted=%d submissions=%d difficulty=%v",
			got.WindowStart, got.WindowAccepted, got.WindowSubmissions, got.WindowDifficulty)
	}
}

func TestEnsureWindowLocked_ResetsAfterLongIdle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	mc := &MinerConn{}
	mc.stats.WindowStart = now.Add(-2 * time.Hour)
	mc.stats.WindowAccepted = 12
	mc.stats.WindowSubmissions = 15
	mc.stats.WindowDifficulty = 42
	mc.stats.LastShare = now.Add(-statusWindowIdleReset - time.Minute)

	mc.statsMu.Lock()
	mc.ensureWindowLocked(now)
	got := mc.stats
	mc.statsMu.Unlock()

	if !got.WindowStart.Equal(now) || got.WindowAccepted != 0 || got.WindowSubmissions != 0 || got.WindowDifficulty != 0 {
		t.Fatalf("status window should reset after long idle: start=%v accepted=%d submissions=%d difficulty=%v",
			got.WindowStart, got.WindowAccepted, got.WindowSubmissions, got.WindowDifficulty)
	}
}
