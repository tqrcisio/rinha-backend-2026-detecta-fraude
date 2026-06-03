package index

import (
	"sort"
	"testing"
)

func bruteInt(q *[Stride]int16, r *Refs) [5]int32 {
	type nb struct {
		d int64
		g int32
	}
	all := make([]nb, r.N)
	for i := 0; i < r.N; i++ {
		base := i * Stride
		var d int64
		for k := 0; k < Stride; k++ {
			diff := int64(q[k]) - int64(r.Vec[base+k])
			d += diff * diff
		}
		all[i] = nb{d, int32(i)}
	}
	sort.Slice(all, func(a, b int) bool {
		if all[a].d != all[b].d {
			return all[a].d < all[b].d
		}
		return all[a].g < all[b].g
	})
	var out [5]int32
	for i := 0; i < 5; i++ {
		out[i] = all[i].g
	}
	sort.Slice(out[:], func(a, b int) bool { return out[a] < out[b] })
	return out
}

func mkref(vals ...int16) []int16 {
	v := make([]int16, Stride)
	copy(v, vals)
	return v
}

// Hole #1: cross-partition stop with gap == worstDist exactly.
// Home partition key 0 (dims 9,10,11 = 0, dim5 = sentinel -10000).
// Put 5 refs in home at distance D = 200_000_000 (exactly the bit3 gap to a present partition),
// then a 6th ref in the bit3-flipped partition (key 8) at distance EXACTLY 200_000_000 with LOWER gidx.
// Brute (same tie rule) must include the lower-gidx point. KD with >= break must SKIP it -> mismatch.
func TestCrossPartitionStopTieHole(t *testing.T) {
	// query: sentinel on dim5/dim6, zeros elsewhere -> partKey16 = 0
	var q [Stride]int16
	q[5] = sentinel
	q[6] = sentinel
	if partKey16(q[:]) != 0 {
		t.Fatalf("query key=%d, want 0", partKey16(q[:]))
	}

	refs := &Refs{}
	add := func(v []int16, fraud bool) {
		refs.Vec = append(refs.Vec, v...)
		refs.Fraud = append(refs.Fraud, fraud)
		refs.N++
	}

	// gidx 0: a PRESENT-partition ref (key 8) at dims5,6 = (0,0) -> dist^2 from sentinel query
	// = (0-(-10000))^2 + (0-(-10000))^2 = 2e8 exactly. This is the bit3 gap-tight point, LOWEST gidx.
	add(mkref(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0), true)

	// gidx 1..5: home-partition refs (key 0: sentinel dim5/6) each at dist^2 = 2e8.
	// achieve 2e8 with a single non-key dim: dim0 = sqrt(2e8) not integer; use two dims.
	// dim0=10000 -> 1e8, dim1=10000 -> 1e8, total 2e8. dims5/6 sentinel to keep key 0.
	for i := 0; i < 5; i++ {
		add(mkref(10000, 10000, 0, 0, 0, sentinel, sentinel, 0, 0, 0, 0, 0, 0, 0), false)
	}

	// sanity: home refs are key 0, the (0,0) ref is key 8
	if partKey16(refs.Vec[0:Stride]) != 8 {
		t.Fatalf("ref0 key=%d want 8", partKey16(refs.Vec[0:Stride]))
	}
	if partKey16(refs.Vec[Stride:2*Stride]) != 0 {
		t.Fatalf("ref1 key=%d want 0", partKey16(refs.Vec[Stride:2*Stride]))
	}

	kd := BuildKD(refs)
	want := bruteInt(&q, refs)
	gotStop := kd.Neighbors5(&q)
	gotAll := neighborsAll(kd, &q)

	t.Logf("brute(exact)   = %v", want)
	t.Logf("KD stop(>=)    = %v", gotStop)
	t.Logf("KD all-probe   = %v", gotAll)

	if gotAll != want {
		t.Errorf("KD all-probe disagrees with brute: got %v want %v", gotAll, want)
	}
	if gotStop != want {
		t.Errorf("HOLE#1 CONFIRMED: KD cross-partition stop (>=) returns %v, brute returns %v", gotStop, want)
	}
}

// helper: neighbors with stop disabled
func neighborsAll(kd *KD, q *[Stride]int16) [5]int32 {
	var b best5
	b.init()
	stack := make([]int32, 0, 64)
	key := partKey16(q[:])
	stack = kd.searchPart(key, q, &b, stack)
	for _, ob := range kd.probeOrder[key] {
		stack = kd.searchPart(ob, q, &b, stack)
	}
	g := b.gidx
	sort.Slice(g[:], func(i, j int) bool { return g[i] < g[j] })
	return g
}

// Hole #2: intra-partition AABB prune with bound == worstDist exactly.
// All refs in ONE partition. Build so that a subtree's box lower-bound equals the worst dist,
// and that subtree contains a point at exactly worst dist with LOWER gidx than the current 5th.
func TestAABBPruneTieHole(t *testing.T) {
	var q [Stride]int16 // key 0, all zeros

	refs := &Refs{}
	add := func(v []int16, fraud bool) {
		refs.Vec = append(refs.Vec, v...)
		refs.Fraud = append(refs.Fraud, fraud)
		refs.N++
	}

	// Force many refs into one partition (key 0) so a KD tree with internal nodes is built (>64 rows).
	// gidx 0: the LOW-gidx tie point at dist^2 = D = 1e8 (dim0 = 10000).
	add(mkref(10000, 0, 0, 0, 0, sentinel, sentinel, 0, 0, 0, 0, 0, 0, 0), true)
	// gidx 1..4: four points strictly closer than D (dim0 = 5000 -> 2.5e7).
	for i := 0; i < 4; i++ {
		add(mkref(5000, 0, 0, 0, 0, sentinel, sentinel, 0, 0, 0, 0, 0, 0, 0), false)
	}
	// gidx 5..: many points at dist^2 = D = 1e8 with HIGHER gidx (dim1 = 10000), plus filler far points
	for i := 0; i < 200; i++ {
		add(mkref(0, 10000, 0, 0, 0, sentinel, sentinel, 0, 0, 0, 0, 0, 0, 0), false)
	}
	// filler far points to grow the tree and create internal split nodes
	for i := 0; i < 200; i++ {
		add(mkref(int16(20000), int16(20000), 0, 0, 0, sentinel, sentinel, 0, 0, 0, 0, 0, 0, 0), false)
	}

	kd := BuildKD(refs)
	want := bruteInt(&q, refs)
	gotStop := kd.Neighbors5(&q)
	gotAll := neighborsAll(kd, &q)
	t.Logf("brute(exact) = %v", want)
	t.Logf("KD          = %v", gotStop)
	t.Logf("KD all      = %v", gotAll)
	if gotStop != want {
		t.Errorf("HOLE#2 CANDIDATE: KD intra-partition returns %v, brute returns %v", gotStop, want)
	}
}
