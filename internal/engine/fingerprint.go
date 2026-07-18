package engine

import (
	"bytes"
	"crypto/sha256"
)

// CanonicalMin returns the lexicographically smallest of the candidate
// serializations. Canonicalizers serialize a puzzle under every transform in
// its symmetry group and feed the results here; the winner is the canonical
// form, so all symmetric variants of one puzzle share a fingerprint.
// Panics on an empty candidate list (a programming error, not a data error).
func CanonicalMin(candidates [][]byte) []byte {
	if len(candidates) == 0 {
		panic("engine: CanonicalMin called with no candidates")
	}
	min := candidates[0]
	for _, c := range candidates[1:] {
		if bytes.Compare(c, min) < 0 {
			min = c
		}
	}
	return min
}

// FingerprintBytes hashes a canonical serialization into the fixed-size
// fingerprint used by dedup sets and the corpus.
func FingerprintBytes(canonical []byte) [32]byte {
	return sha256.Sum256(canonical)
}
