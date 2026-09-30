package main

import (
	"encoding/binary"
	"testing"
)

func TestDuplicateShareSetRepeatedEviction(t *testing.T) {
	var cache duplicateShareSet
	keyFor := func(seq uint64) duplicateShareKey {
		var key duplicateShareKey
		key.n = 8
		binary.LittleEndian.PutUint64(key.buf[:8], seq)
		return key
	}
	evictCount := max(duplicateShareHistory/10, 1)
	next := uint64(0)
	for range 20 {
		// Fill to capacity, including the free space from the last eviction.
		for len(cache.order) < duplicateShareHistory {
			if cache.seenOrAdd(keyFor(next)) {
				t.Fatalf("new share %d treated as duplicate", next)
			}
			next++
		}
		oldest := next - uint64(duplicateShareHistory)
		if !cache.seenOrAdd(keyFor(oldest)) {
			t.Fatalf("oldest retained share %d missing", oldest)
		}
		if cache.seenOrAdd(keyFor(next)) {
			t.Fatalf("new share %d treated as duplicate at capacity", next)
		}
		next++
		wantLen := duplicateShareHistory - evictCount + 1
		if len(cache.order) != wantLen || len(cache.m) != wantLen {
			t.Fatalf("cache sizes = (%d, %d), want %d", len(cache.order), len(cache.m), wantLen)
		}
		for i := range uint64(evictCount) {
			if _, ok := cache.m[keyFor(oldest+i)]; ok {
				t.Fatalf("old share %d survived eviction", oldest+i)
			}
		}
		for seq := oldest + uint64(evictCount); seq < next; seq++ {
			if !cache.seenOrAdd(keyFor(seq)) {
				t.Fatalf("recent share %d lost during eviction", seq)
			}
		}
	}
}
