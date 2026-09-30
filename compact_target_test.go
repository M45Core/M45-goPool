package main

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
)

func TestTargetFromBitsCompactCompatibility(t *testing.T) {
	for _, bits := range []uint32{0x01010000, 0x02008000, 0x03012345, 0x1d00ffff, 0x207fffff, 0x1d80ffff} {
		hexBits := uint32ToHex8Lower(bits)
		t.Run(hexBits, func(t *testing.T) {
			got, err := targetFromBits(hexBits)
			if err != nil {
				t.Fatal(err)
			}
			want := blockchain.CompactToBig(bits)
			if got.Cmp(want) != 0 {
				t.Fatalf("decoded target = %x, want %x", got, want)
			}
		})
	}
}

func TestValidateBitsRejectsInvalidTarget(t *testing.T) {
	for _, bits := range []string{"1d80ffff", "1d000000", "01000001", "21010000", "ff123456"} {
		t.Run(bits, func(t *testing.T) {
			if _, err := validateBits(bits, ""); err == nil {
				t.Fatal("accepted a negative, zero, or overflowing proof-of-work target")
			}
		})
	}
}
