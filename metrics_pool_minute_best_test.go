package main

import (
	"testing"
	"time"
)

func TestPoolMinuteBestDifficultyTracksMaximumWithinMinute(t *testing.T) {
	metrics := &PoolMetrics{}
	minute := time.Unix(1_700_000_040, 0).UTC().Truncate(time.Minute)

	metrics.observePoolMinuteBestDifficulty(1200, minute.Add(2*time.Second))
	metrics.observePoolMinuteBestDifficulty(9900, minute.Add(20*time.Second))
	metrics.observePoolMinuteBestDifficulty(3400, minute.Add(45*time.Second))

	if got, want := metrics.poolMinuteBestDifficultyQ(minute), encodeBestShareSI16(9900); got != want {
		t.Fatalf("minute best q=%d want %d", got, want)
	}
	if got := metrics.poolMinuteBestDifficultyQ(minute.Add(time.Minute)); got != 0 {
		t.Fatalf("empty adjacent minute q=%d want 0", got)
	}
}

func TestTrackBestShareObservesMinuteBeforeLeaderboardPrefilter(t *testing.T) {
	metrics := &PoolMetrics{bestShareCount: defaultBestShareLimit}
	for i := range metrics.bestShares {
		metrics.bestShares[i].Difficulty = 10_000
	}
	minute := time.Unix(1_700_000_040, 0).UTC().Truncate(time.Minute)

	// This cannot rank in the all-time leaderboard, but it must still appear as
	// the best share for its homepage chart minute.
	metrics.TrackBestShare("worker", "hash", 9000, minute.Add(20*time.Second))

	if got, want := metrics.poolMinuteBestDifficultyQ(minute), encodeBestShareSI16(9000); got != want {
		t.Fatalf("minute best q=%d want %d", got, want)
	}
}

func TestPoolMinuteBestDifficultyRejectsStaleRingSlot(t *testing.T) {
	metrics := &PoolMetrics{}
	minute := time.Unix(1_700_000_040, 0).UTC().Truncate(time.Minute)
	later := minute.Add(poolMinuteBestSlots * time.Minute)

	metrics.observePoolMinuteBestDifficulty(1200, minute)
	metrics.observePoolMinuteBestDifficulty(3400, later)

	if got := metrics.poolMinuteBestDifficultyQ(minute); got != 0 {
		t.Fatalf("overwritten minute q=%d want 0", got)
	}
	if got, want := metrics.poolMinuteBestDifficultyQ(later), encodeBestShareSI16(3400); got != want {
		t.Fatalf("new minute q=%d want %d", got, want)
	}
}
