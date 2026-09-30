package main

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

func TestBlockHeaderKnownProofOfWorkCompatibility(t *testing.T) {
	for _, params := range []*chaincfg.Params{&chaincfg.MainNetParams, &chaincfg.RegressionNetParams} {
		t.Run(params.Name, func(t *testing.T) {
			hdr := params.GenesisBlock.Header
			var canonical bytes.Buffer
			if err := hdr.Serialize(&canonical); err != nil {
				t.Fatal(err)
			}
			ntime := uint32ToHex8Lower(uint32(hdr.Timestamp.Unix()))
			nonce := uint32ToHex8Lower(hdr.Nonce)
			bits := uint32ToHex8Lower(hdr.Bits)
			job := &Job{}
			if err := decodeHexToFixedBytes(job.prevHashBytes[:], hdr.PrevBlock.String()); err != nil {
				t.Fatal(err)
			}
			if err := decodeHex8To4(&job.bitsBytes, bits); err != nil {
				t.Fatal(err)
			}
			builders := map[string]func() ([]byte, error){
				"from_hex": func() ([]byte, error) {
					return buildBlockHeaderFromHex(hdr.Version, hdr.PrevBlock.String(), hdr.MerkleRoot[:], ntime, bits, nonce)
				},
				"job_hex": func() ([]byte, error) {
					return job.buildBlockHeader(hdr.MerkleRoot[:], ntime, nonce, hdr.Version)
				},
				"job_u32": func() ([]byte, error) {
					return job.buildBlockHeaderU32(hdr.MerkleRoot[:], uint32(hdr.Timestamp.Unix()), hdr.Nonce, hdr.Version)
				},
			}
			for name, build := range builders {
				t.Run(name, func(t *testing.T) {
					raw, err := build()
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(raw, canonical.Bytes()) {
						t.Fatalf("header differs from pogolo's wire serialization: got %s want %s", hex.EncodeToString(raw), hex.EncodeToString(canonical.Bytes()))
					}
					var decoded wire.BlockHeader
					if err := decoded.Deserialize(bytes.NewReader(raw)); err != nil {
						t.Fatal(err)
					}
					if err := blockchain.CheckProofOfWork(btcutil.NewBlock(&wire.MsgBlock{Header: decoded}), params.PowLimit); err != nil {
						t.Fatalf("known valid PoW rejected: %v", err)
					}
					target, err := targetFromBits(bits)
					if err != nil {
						t.Fatal(err)
					}
					hash := doubleSHA256Array(raw)
					hashBE := hash
					reverseBytes32(&hashBE)
					if !uint256BELessOrEqual(hashBE, uint256BEFromBigInt(target)) {
						t.Fatal("goPool rejected known valid block target")
					}
					if difficultyFromHash(hash[:]) < difficultyFromBits(hdr.Bits) {
						t.Fatal("goPool difficulty disagrees with known valid PoW")
					}
				})
			}
		})
	}
}

// TestBlockHeaderPoWCompatWithBtcd builds a simple block using goPool's
// header construction path and verifies that btcd's CheckProofOfWork agrees
// with our difficultyFromHash / target comparison.
func TestBlockHeaderPoWCompatWithBtcd(t *testing.T) {
	// Construct a minimal job similar to the ones used in block_test.go.
	job := &Job{
		JobID: "pow-compat-test",
		Template: GetBlockTemplateResult{
			Height:        200,
			CurTime:       1700000300,
			Mintime:       0,
			Bits:          "1d00ffff",
			Previous:      "0000000000000000000000000000000000000000000000000000000000000000",
			CoinbaseValue: 50 * 1e8,
		},
		Extranonce2Size:         4,
		TemplateExtraNonce2Size: 8,
		PayoutScript:            []byte{0x51}, // OP_TRUE
		WitnessCommitment:       "",
		CoinbaseMsg:             "goPool-pow-test",
		ScriptTime:              0,
		Transactions:            nil,
		MerkleBranches:          nil,
		CoinbaseValue:           50 * 1e8,
	}

	ex1 := []byte{0x01, 0x02, 0x03, 0x04}
	ex2 := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	ntimeHex := "5f5e1000" // arbitrary ntime; PoW need not be valid mainnet block
	nonceHex := "00000001"
	version := int32(1)

	blockHex, _, headerBytes, _, err := buildBlock(job, ex1, ex2, ntimeHex, nonceHex, version)
	if err != nil {
		t.Fatalf("buildBlock error: %v", err)
	}
	if len(headerBytes) != 80 {
		t.Fatalf("expected 80-byte header, got %d", len(headerBytes))
	}

	// Check the exact header we built, including its previous hash, merkle
	// root, time, and nonce. A different synthetic header is not a PoW oracle.
	var hdr wire.BlockHeader
	if err := hdr.Deserialize(bytes.NewReader(headerBytes)); err != nil {
		t.Fatalf("deserialize header: %v", err)
	}

	// Hash the header bytes for goPool's difficulty calculation.
	headerHashArray := doubleSHA256Array(headerBytes)
	headerHash := headerHashArray[:]
	btcdHash := hdr.BlockHash()
	if !bytes.Equal(headerHash, btcdHash[:]) {
		t.Fatalf("header hash differs: goPool=%x btcd=%x", headerHash, btcdHash)
	}
	shareDiff := difficultyFromHash(headerHash)

	// btcd's CheckProofOfWork expects a btcutil.Block.
	msgBlock := &wire.MsgBlock{Header: hdr}
	block := btcutil.NewBlock(msgBlock)

	err = blockchain.CheckProofOfWork(block, chaincfg.MainNetParams.PowLimit)
	btcdAccepts := err == nil

	// If btcd considers the block header valid PoW, our computed difficulty
	// must be at least 1 (diff1Target / target >= 1). If btcd rejects it as
	// high hash, our difficulty must be below 1.
	if btcdAccepts && shareDiff < 1 {
		t.Fatalf("btcd accepts header PoW but goPool difficultyFromHash=%.8f < 1", shareDiff)
	}
	if !btcdAccepts && shareDiff >= 1 {
		t.Fatalf("btcd rejects header PoW but goPool difficultyFromHash=%.8f >= 1", shareDiff)
	}

	_ = blockHex // ensures blockHex is used for compile; reserved for future assertions
}
