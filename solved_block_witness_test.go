package main

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestSolvedBlockWitnessCommitment(t *testing.T) {
	for _, withWitnessTx := range []bool{false, true} {
		name := "coinbase_only"
		if withWitnessTx {
			name = "witness_transaction"
		}
		t.Run(name, func(t *testing.T) {
			job := benchmarkSubmitJobForTest(t)
			var transactions []*btcutil.Tx
			if withWitnessTx {
				tx := makeMerkleTestTransactions(1)[0].MsgTx()
				tx.TxIn[0].Witness = wire.TxWitness{[]byte{1, 2, 3}}
				var raw bytes.Buffer
				if err := tx.Serialize(&raw); err != nil {
					t.Fatal(err)
				}
				job.Transactions = []GBTTransaction{{Data: hex.EncodeToString(raw.Bytes())}}
				transactions = []*btcutil.Tx{btcutil.NewTx(tx)}
			}
			// Like pogolo and Bitcoin Core, commit to a zero coinbase wtxid and
			// a 32-byte zero reserved value, independently of the coinbase txid.
			allTxs := append([]*btcutil.Tx{btcutil.NewTx(wire.NewMsgTx(1))}, transactions...)
			witnessRoot := blockchain.CalcMerkleRoot(allTxs, true)
			var preimage [64]byte
			copy(preimage[:32], witnessRoot[:])
			commitment := append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, chainhash.DoubleHashB(preimage[:])...)
			job.WitnessCommitment = hex.EncodeToString(commitment)
			job.MerkleBranches = buildMerkleBranches(transactions)
			ex1, ex2 := []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}
			coinbase, txid, err := serializeCoinbaseTxPredecoded(job.Template.Height, ex1, ex2, job.TemplateExtraNonce2Size, job.PayoutScript, job.CoinbaseValue, commitment, nil, job.CoinbaseMsg, job.ScriptTime)
			if err != nil {
				t.Fatal(err)
			}
			root := computeMerkleRootFromBranches(txid, job.MerkleBranches)
			header, err := job.buildBlockHeaderU32(root, uint32(job.Template.CurTime), 1, job.Template.Version)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"solved", "rebuilt"} {
				t.Run(path, func(t *testing.T) {
					var blockHex string
					var err error
					if path == "solved" {
						blockHex, err = assembleSolvedBlock(job, header, coinbase)
					} else {
						blockHex, _, _, _, err = buildBlockWithScriptTime(job, ex1, ex2, uint32ToHex8Lower(uint32(job.Template.CurTime)), "00000001", job.Template.Version, job.PayoutScript, job.ScriptTime)
					}
					if err != nil {
						t.Fatal(err)
					}
					raw, err := hex.DecodeString(blockHex)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(raw[:80], header) {
						t.Fatal("block assembly changed the solved header")
					}
					var block wire.MsgBlock
					if err := block.Deserialize(bytes.NewReader(raw)); err != nil {
						t.Fatal(err)
					}
					if err := blockchain.ValidateWitnessCommitment(btcutil.NewBlock(&block)); err != nil {
						t.Fatalf("assembled block violates SegWit consensus: %v", err)
					}
					var stripped bytes.Buffer
					if err := block.Transactions[0].SerializeNoWitness(&stripped); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(stripped.Bytes(), coinbase) {
						t.Fatal("adding witness changed the solved coinbase's base serialization")
					}
					gotTxID := block.Transactions[0].TxHash()
					if !bytes.Equal(gotTxID[:], txid) || !bytes.Equal(block.Header.MerkleRoot[:], root) {
						t.Fatal("adding witness changed the solved transaction ID or merkle root")
					}
				})
			}
		})
	}
}

func TestCoinbaseForBlockPreservesExistingWitnessAndLegacyBytes(t *testing.T) {
	job := benchmarkSubmitJobForTest(t)
	legacy, _, err := serializeCoinbaseTxPredecoded(job.Template.Height, []byte{1, 2, 3, 4}, make([]byte, 4), job.TemplateExtraNonce2Size, job.PayoutScript, job.CoinbaseValue, nil, nil, job.CoinbaseMsg, job.ScriptTime)
	if err != nil {
		t.Fatal(err)
	}
	got, err := coinbaseForBlock(legacy)
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("legacy bytes changed: error=%v", err)
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(legacy)); err != nil {
		t.Fatal(err)
	}
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, make([]byte, 32)...)))
	tx.TxIn[0].Witness = wire.TxWitness{bytes.Repeat([]byte{0x7f}, 32)}
	var full bytes.Buffer
	if err := tx.Serialize(&full); err != nil {
		t.Fatal(err)
	}
	got, err = coinbaseForBlock(full.Bytes())
	if err != nil || !bytes.Equal(got, full.Bytes()) {
		t.Fatalf("existing witness bytes changed: error=%v", err)
	}
	for _, raw := range [][]byte{nil, {1}, append(append([]byte(nil), legacy...), 0)} {
		if _, err := coinbaseForBlock(raw); err == nil {
			t.Fatalf("accepted invalid coinbase bytes: %x", raw)
		}
	}
}
