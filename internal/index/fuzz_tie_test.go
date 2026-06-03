package index

// Adversarial tie fuzz: every non-key dim is in {0,10000} so each squared diff is exactly 0 or 1e8.
// Total distances are integer multiples of 1e8, which collide constantly with partition gaps (1e8,2e8,...)
// and AABB box-face bounds. This maximizes the gap/bound == worstDist alignment the >= hole needs.

import "testing"

func randRefsBinary(r *xrng, n int) *Refs {
	refs := &Refs{N: n, Vec: make([]int16, n*Stride), Fraud: make([]bool, n)}
	for i := 0; i < n; i++ {
		base := i * Stride
		if r.next()&3 == 0 {
			refs.Vec[base+5] = sentinel
			refs.Vec[base+6] = sentinel
		} else {
			if r.next()&1 == 0 {
				refs.Vec[base+5] = 10000
			}
			if r.next()&1 == 0 {
				refs.Vec[base+6] = 10000
			}
		}
		for d := 0; d < RawDims; d++ {
			if d == 5 || d == 6 {
				continue
			}
			if r.next()&1 == 0 {
				refs.Vec[base+d] = 10000
			}
		}
		refs.Fraud[i] = r.next()&1 == 0
	}
	return refs
}

func randQueryBinary(r *xrng) [Stride]int16 {
	var q [Stride]int16
	for d := 0; d < RawDims; d++ {
		if d == 5 || d == 6 {
			continue
		}
		if r.next()&1 == 0 {
			q[d] = 10000
		}
	}
	if r.next()&3 == 0 {
		q[5], q[6] = sentinel, sentinel
	} else {
		if r.next()&1 == 0 {
			q[5] = 10000
		}
		if r.next()&1 == 0 {
			q[6] = 10000
		}
	}
	return q
}

func TestKDTieAlignmentFuzz(t *testing.T) {
	rng := &xrng{s: 0xdeadbeefcafef00d}
	stopMiss, allMiss := 0, 0
	for cfg := 0; cfg < 8; cfg++ {
		n := 80 + cfg*120 // span below and above leafSize, force internal nodes
		refs := randRefsBinary(rng, n)
		kd := BuildKD(refs)
		for qi := 0; qi < 5000; qi++ {
			q := randQueryBinary(rng)
			want := bruteInt(&q, refs)
			gotStop := kd.Neighbors5(&q)
			gotAll := neighborsAll(kd, &q)
			if gotAll != want {
				allMiss++
				if allMiss <= 4 {
					t.Errorf("cfg%d q%d ALL mismatch got=%v want=%v q=%v", cfg, qi, gotAll, want, q)
				}
			}
			if gotStop != want {
				stopMiss++
				if stopMiss <= 4 {
					t.Errorf("cfg%d q%d STOP mismatch got=%v want=%v q=%v", cfg, qi, gotStop, want, q)
				}
			}
		}
	}
	t.Logf("stopMiss=%d allMiss=%d", stopMiss, allMiss)
	if stopMiss > 0 || allMiss > 0 {
		t.Fatalf("tie-alignment fuzz found divergence: stop=%d all=%d", stopMiss, allMiss)
	}
}
