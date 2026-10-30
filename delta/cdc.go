// Package delta does content-defined chunking (Gear hash, FastCDC-style) so an
// update only ships chunks the edge node does not already have.
//
// Fixed-size blocks break on every insertion (all later blocks shift).
// Content-defined boundaries move with the content, so inserting a byte only
// changes one or two chunks. That is the difference between resending 200 MB
// and 200 KB over a degraded link.
package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var gear [256]uint64

func init() {
	// Deterministic table (splitmix64) so both ends chunk identically.
	x := uint64(0x9E3779B97F4A7C15)
	for i := range gear {
		x += 0x9E3779B97F4A7C15
		z := x
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		gear[i] = z ^ (z >> 31)
	}
}

type Params struct{ Min, Avg, Max int }

var Default = Params{Min: 2 << 10, Avg: 8 << 10, Max: 64 << 10}

type Chunk struct {
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
	Hash   string `json:"sha256"`
}

func maskFor(avg int) uint64 {
	bits := 0
	for (1 << bits) < avg {
		bits++
	}
	return (1 << bits) - 1
}

// Split returns chunk boundaries of data.
func Split(data []byte, p Params) []Chunk {
	mask := maskFor(p.Avg)
	var out []Chunk
	start := 0
	for start < len(data) {
		end := start + p.Max
		if end > len(data) {
			end = len(data)
		}
		cut := end
		var h uint64
		for i := start + p.Min; i < end; i++ {
			h = (h << 1) + gear[data[i]]
			if h&mask == 0 {
				cut = i + 1
				break
			}
		}
		sum := sha256.Sum256(data[start:cut])
		out = append(out, Chunk{start, cut - start, hex.EncodeToString(sum[:])})
		start = cut
	}
	return out
}

// Plan lists chunks of the new version that the receiver (who has `have`) is missing.
func Plan(newChunks []Chunk, have map[string]bool) (need []Chunk, bytes int) {
	seen := map[string]bool{}
	for _, c := range newChunks {
		if !have[c.Hash] && !seen[c.Hash] {
			need = append(need, c)
			bytes += c.Size
			seen[c.Hash] = true
		}
	}
	return
}

// Assemble rebuilds a file from its chunk list and a content-addressed store, verifying each chunk.
func Assemble(chunks []Chunk, store map[string][]byte) ([]byte, error) {
	var out []byte
	for _, c := range chunks {
		b, ok := store[c.Hash]
		if !ok {
			return nil, fmt.Errorf("missing chunk %s", c.Hash[:12])
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != c.Hash {
			return nil, errors.New("chunk hash mismatch")
		}
		out = append(out, b...)
	}
	return out, nil
}
