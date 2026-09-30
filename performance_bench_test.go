package main

import (
	"encoding/binary"
	"fmt"
	"testing"
)

var benchmarkLatencyPercentiles [2]float64

func BenchmarkSubmitRTTPercentiles(b *testing.B) {
	for _, count := range []int{8, 64} {
		b.Run(fmt.Sprintf("%d_samples", count), func(b *testing.B) {
			var samples [64]float64
			for i := range samples {
				samples[i] = float64((i*17)%64 + 1)
			}
			b.ReportAllocs()
			for b.Loop() {
				p50, p95 := submitRTTPercentilesLocked(samples, count)
				benchmarkLatencyPercentiles = [2]float64{p50, p95}
			}
		})
	}
}

func BenchmarkBuildCoinbaseOutputs(b *testing.B) {
	for _, count := range []int{1, 2, 3, maxCoinbasePayoutOutputs} {
		b.Run(fmt.Sprintf("%d_payouts", count), func(b *testing.B) {
			payouts := make([]coinbasePayoutOutput, count)
			for i := range payouts {
				payouts[i] = coinbasePayoutOutput{Script: []byte{0x51}, Value: int64(i + 1)}
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := buildCoinbaseOutputs(nil, payouts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDuplicateShareSetSteadyState(b *testing.B) {
	var cache duplicateShareSet
	var key duplicateShareKey
	key.n = 8
	// Start with a full cache so the benchmark measures repeated evictions.
	for i := range uint64(duplicateShareHistory) {
		binary.LittleEndian.PutUint64(key.buf[:8], i)
		cache.seenOrAdd(key)
	}
	seq := uint64(duplicateShareHistory)
	b.ReportAllocs()
	for b.Loop() {
		binary.LittleEndian.PutUint64(key.buf[:8], seq)
		if cache.seenOrAdd(key) {
			b.Fatal("unique share treated as duplicate")
		}
		seq++
	}
}
