package index

import (
	"os"
	"testing"
)

// The bit3 gap of 2e8 assumes a sentinel flip forces BOTH dims 5 and 6 to differ by the full
// 1e8 sentinel gap. That holds only because dims 5 and 6 are always sentinel-together in the
// references (verified: 0 violations across all 3M refs). If that invariant ever broke, gapBits
// would OVER-estimate and the stopping rule would prune partitions holding true neighbors.
// This test pins the dependency: a hand-built violating ref makes gapBits exceed the real distance.
func TestGapBoundOverestimatesWhenSentinelSplit(t *testing.T) {
	// query in a present partition: dim5 present(0), dim6 present(10000), dim9 present, dim10 present.
	var q [Stride]int16
	q[6] = 10000
	q[9] = 10000
	q[10] = 10000
	key := partKey16(q[:]) // bits 0,1,3 -> 11
	if key != 11 {
		t.Fatalf("key=%d want 11", key)
	}

	// a ref in partition 5 (bits 0,2): dim9 present, dim11 present, dim5 sentinel.
	// INVARIANT-VIOLATING: dim5 sentinel but dim6 present(10000) == query's dim6 -> dim6 diff 0.
	rv := make([]int16, Stride)
	rv[5] = sentinel
	rv[6] = 10000 // <- violates dims-5,6-together; matches query dim6
	rv[9] = 10000
	rv[11] = 10000
	if partKey16(rv) != 5 {
		t.Fatalf("ref key=%d want 5", partKey16(rv))
	}

	var realDist int64
	for d := 0; d < Stride; d++ {
		dd := int64(q[d]) - int64(rv[d])
		realDist += dd * dd
	}
	gap := gapBits(key, 5)

	// dim10 flip (1e8) + dim11 flip (1e8) + dim5 sentinel (1e8) + dim6 match (0) = 3e8.
	// gapBits claims 4e8 (1e8+1e8+2e8). Over-estimate => unsafe IF this data could exist.
	if realDist != 300_000_000 {
		t.Fatalf("realDist=%d want 3e8", realDist)
	}
	if gap <= realDist {
		t.Fatalf("expected gap to over-estimate the violating ref; gap=%d realDist=%d", gap, realDist)
	}
	t.Logf("gap=%d > realDist=%d: bit3 gap is safe ONLY under the sentinel-together invariant", gap, realDist)
}

func sentinelInvariantHolds(r *Refs) bool {
	for i := 0; i < r.N; i++ {
		base := i * Stride
		if (r.Vec[base+5] == sentinel) != (r.Vec[base+6] == sentinel) {
			return false
		}
	}
	return true
}

// Guard the production references: dims 5,6 must be sentinel-together, otherwise the bit3 gap of
// 2e8 over-estimates and the stopping rule silently drops true neighbors. Skips if the bin isn't built.
func TestRealRefsSentinelInvariant(t *testing.T) {
	bin := os.Getenv("RINHA_BIN")
	if bin == "" {
		bin = "/tmp/rinha-refs.bin"
	}
	r, err := LoadBin(bin)
	if err != nil {
		t.Skipf("no bin at %s: %v", bin, err)
	}
	if !sentinelInvariantHolds(r) {
		t.Fatalf("references VIOLATE dims-5,6 sentinel-together: bit3 gap bound is unsafe")
	}
	t.Logf("invariant holds across N=%d refs", r.N)
}
