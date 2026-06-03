package index

const numBuckets = 16
const sentinel = -10000

type bucket struct {
	vec  []int16
	gidx []int32
	n    int
}

type Index struct {
	fraud   []bool
	buckets [numBuckets]bucket
}

func partKey16(v []int16) int {
	k := 0
	if v[9] != 0 {
		k |= 1
	}
	if v[10] != 0 {
		k |= 2
	}
	if v[11] != 0 {
		k |= 4
	}
	if v[5] != sentinel {
		k |= 8
	}
	return k
}

func BuildIndex(r *Refs) *Index {
	ix := &Index{fraud: r.Fraud}
	for i := 0; i < r.N; i++ {
		v := r.Vec[i*Stride : i*Stride+Stride]
		b := &ix.buckets[partKey16(v)]
		b.vec = append(b.vec, v...)
		b.gidx = append(b.gidx, int32(i))
		b.n++
	}
	return ix
}

func (ix *Index) BucketSizes() [numBuckets]int {
	var s [numBuckets]int
	for i := range ix.buckets {
		s[i] = ix.buckets[i].n
	}
	return s
}

type pneighbor struct {
	dist int64
	gidx int32
}

func (ix *Index) FraudCount5(q *[Stride]int16) int {
	b := &ix.buckets[partKey16(q[:])]
	var best [5]pneighbor
	for i := range best {
		best[i] = pneighbor{dist: 1 << 62, gidx: 1 << 30}
	}
	worst := 0
	for i := 0; i < b.n; i++ {
		base := i * Stride
		var d int64
		for k := 0; k < Stride; k++ {
			diff := int64(q[k]) - int64(b.vec[base+k])
			d += diff * diff
		}
		g := b.gidx[i]
		w := best[worst]
		if d < w.dist || (d == w.dist && g < w.gidx) {
			best[worst] = pneighbor{dist: d, gidx: g}
			worst = 0
			for j := 1; j < 5; j++ {
				bj, bw := best[j], best[worst]
				if bj.dist > bw.dist || (bj.dist == bw.dist && bj.gidx > bw.gidx) {
					worst = j
				}
			}
		}
	}
	cnt := 0
	for _, nb := range best {
		if ix.fraud[nb.gidx] {
			cnt++
		}
	}
	return cnt
}
