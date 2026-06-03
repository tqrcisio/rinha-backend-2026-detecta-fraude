package index

import "testing"

type xrng struct{ s uint64 }

func (r *xrng) next() uint64 {
	r.s ^= r.s << 13
	r.s ^= r.s >> 7
	r.s ^= r.s << 17
	return r.s
}

// build refs over a tiny domain so the partition key dims AND the distance dims collide constantly,
// producing dense exact-tie boundaries that stress every prune/stop comparison.
func randRefs(r *xrng, n int, dom int16) *Refs {
	refs := &Refs{N: n, Vec: make([]int16, n*Stride), Fraud: make([]bool, n)}
	for i := 0; i < n; i++ {
		base := i * Stride
		for d := 0; d < RawDims; d++ {
			switch d {
			case 9, 10, 11:
				if r.next()&1 == 0 {
					refs.Vec[base+d] = 10000
				}
			case 5, 6:
				if r.next()&1 == 0 {
					refs.Vec[base+d] = sentinel
				} else {
					refs.Vec[base+d] = int16(r.next() % uint64(dom))
				}
			default:
				refs.Vec[base+d] = int16(r.next() % uint64(dom))
			}
		}
		refs.Fraud[i] = r.next()&1 == 0
	}
	return refs
}

func randQuery(r *xrng, dom int16) [Stride]int16 {
	var q [Stride]int16
	for d := 0; d < RawDims; d++ {
		switch d {
		case 9, 10, 11:
			if r.next()&1 == 0 {
				q[d] = 10000
			}
		case 5, 6:
			if r.next()&1 == 0 {
				q[d] = sentinel
			} else {
				q[d] = int16(r.next() % uint64(dom))
			}
		default:
			q[d] = int16(r.next() % uint64(dom))
		}
	}
	// keep dims 5,6 sentinel-consistent: if one is sentinel both are (matches data invariant)
	if q[5] == sentinel || q[6] == sentinel {
		q[5], q[6] = sentinel, sentinel
	}
	return q
}

func TestKDExactDifferentialFuzz(t *testing.T) {
	rng := &xrng{s: 0x9e3779b97f4a7c15}
	configs := []struct {
		refs, queries int
		dom           int16
	}{
		{300, 2000, 4},     // brutal ties
		{500, 2000, 8},
		{2000, 1000, 20},   // forces internal KD nodes (>64/leaf)
		{5000, 500, 100},
		{1000, 1000, 10000},
	}
	totalStopMiss, totalAllMiss := 0, 0
	for ci, cfg := range configs {
		refs := randRefs(rng, cfg.refs, cfg.dom)
		kd := BuildKD(refs)
		for qi := 0; qi < cfg.queries; qi++ {
			q := randQuery(rng, cfg.dom)
			want := bruteInt(&q, refs)
			gotStop := kd.Neighbors5(&q)
			gotAll := neighborsAll(kd, &q)
			if gotAll != want {
				totalAllMiss++
				if totalAllMiss <= 5 {
					t.Errorf("cfg%d q%d ALL-PROBE mismatch: got %v want %v q=%v", ci, qi, gotAll, want, q)
				}
			}
			if gotStop != want {
				totalStopMiss++
				if totalStopMiss <= 5 {
					t.Errorf("cfg%d q%d STOP mismatch: got %v want %v q=%v", ci, qi, gotStop, want, q)
				}
			}
		}
	}
	t.Logf("stop mismatches=%d all-probe mismatches=%d", totalStopMiss, totalAllMiss)
	if totalStopMiss > 0 || totalAllMiss > 0 {
		t.Fatalf("KD diverges from exact int brute: stop=%d all=%d", totalStopMiss, totalAllMiss)
	}
}
