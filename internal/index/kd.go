package index

import "sort"

const leafSize = 64
const leafMark = 0xFF

type kdNode struct {
	splitVal int16
	splitDim uint8
	left     int32
	right    int32
}

type kdBox struct {
	lo [Stride]int16
	hi [Stride]int16
}

type part struct {
	nodes []kdNode
	boxes []kdBox
	vec   []int16
	gidx  []int32
	count int
}

type KD struct {
	parts      [numBuckets]part
	fraud      []bool
	probeOrder [numBuckets][]int
}

func BuildKD(r *Refs) *KD {
	kd := &KD{fraud: r.Fraud}
	for i := 0; i < r.N; i++ {
		v := r.Vec[i*Stride : i*Stride+Stride]
		p := &kd.parts[partKey16(v)]
		p.vec = append(p.vec, v...)
		p.gidx = append(p.gidx, int32(i))
		p.count++
	}
	for k := range kd.parts {
		if kd.parts[k].count > 0 {
			buildPart(&kd.parts[k])
		}
		kd.probeOrder[k] = probeOrderFor(k)
	}
	return kd
}

func probeOrderFor(home int) []int {
	others := make([]int, 0, numBuckets-1)
	for b := 0; b < numBuckets; b++ {
		if b != home {
			others = append(others, b)
		}
	}
	sort.Slice(others, func(i, j int) bool {
		return gapBits(home, others[i]) < gapBits(home, others[j])
	})
	return others
}

func gapBits(a, b int) int64 {
	diff := a ^ b
	var g int64
	if diff&1 != 0 {
		g += 100_000_000
	}
	if diff&2 != 0 {
		g += 100_000_000
	}
	if diff&4 != 0 {
		g += 100_000_000
	}
	if diff&8 != 0 {
		g += 200_000_000
	}
	return g
}

func buildPart(p *part) {
	perm := make([]int32, p.count)
	for i := range perm {
		perm[i] = int32(i)
	}
	var build func(lo, hi int) int32
	build = func(lo, hi int) int32 {
		idx := int32(len(p.nodes))
		p.nodes = append(p.nodes, kdNode{})
		p.boxes = append(p.boxes, kdBox{})
		p.boxes[idx] = computeBox(p.vec, perm, lo, hi)
		dim, spread := widestDim(&p.boxes[idx])
		if hi-lo <= leafSize || spread == 0 {
			p.nodes[idx] = kdNode{splitDim: leafMark, left: int32(lo), right: int32(hi)}
			return idx
		}
		m := (lo + hi) / 2
		quickselect(p.vec, perm, lo, hi, m, dim)
		sv := p.vec[int(perm[m])*Stride+dim]
		l := build(lo, m)
		rr := build(m, hi)
		p.nodes[idx] = kdNode{splitVal: sv, splitDim: uint8(dim), left: l, right: rr}
		return idx
	}
	build(0, p.count)

	nv := make([]int16, p.count*Stride)
	ng := make([]int32, p.count)
	for r := 0; r < p.count; r++ {
		src := int(perm[r])
		copy(nv[r*Stride:r*Stride+Stride], p.vec[src*Stride:src*Stride+Stride])
		ng[r] = p.gidx[src]
	}
	p.vec = nv
	p.gidx = ng
}

func computeBox(vec []int16, perm []int32, lo, hi int) kdBox {
	var box kdBox
	base := int(perm[lo]) * Stride
	for d := 0; d < Stride; d++ {
		box.lo[d] = vec[base+d]
		box.hi[d] = vec[base+d]
	}
	for i := lo + 1; i < hi; i++ {
		b := int(perm[i]) * Stride
		for d := 0; d < Stride; d++ {
			x := vec[b+d]
			if x < box.lo[d] {
				box.lo[d] = x
			}
			if x > box.hi[d] {
				box.hi[d] = x
			}
		}
	}
	return box
}

func widestDim(box *kdBox) (int, int32) {
	best, spread := 0, int32(-1)
	for d := 0; d < RawDims; d++ {
		s := int32(box.hi[d]) - int32(box.lo[d])
		if s > spread {
			spread = s
			best = d
		}
	}
	return best, spread
}

func quickselect(vec []int16, perm []int32, lo, hi, k, dim int) {
	for hi-lo > 1 {
		pi := medianOfThree(vec, perm, lo, hi, dim)
		pivot := vec[int(perm[pi])*Stride+dim]
		lt, i, gt := lo, lo, hi
		for i < gt {
			v := vec[int(perm[i])*Stride+dim]
			if v < pivot {
				perm[lt], perm[i] = perm[i], perm[lt]
				lt++
				i++
			} else if v > pivot {
				gt--
				perm[i], perm[gt] = perm[gt], perm[i]
			} else {
				i++
			}
		}
		if k < lt {
			hi = lt
		} else if k >= gt {
			lo = gt
		} else {
			return
		}
	}
}

func medianOfThree(vec []int16, perm []int32, lo, hi, dim int) int {
	mid := (lo + hi) / 2
	a, b, c := vec[int(perm[lo])*Stride+dim], vec[int(perm[mid])*Stride+dim], vec[int(perm[hi-1])*Stride+dim]
	if a < b {
		if b < c {
			return mid
		} else if a < c {
			return hi - 1
		}
		return lo
	}
	if a < c {
		return lo
	} else if b < c {
		return hi - 1
	}
	return mid
}

type best5 struct {
	dist  [5]int64
	gidx  [5]int32
	worst int
}

func (b *best5) init() {
	for i := range b.dist {
		b.dist[i] = 1 << 62
		b.gidx[i] = 1 << 30
	}
	b.worst = 0
}

func (b *best5) consider(d int64, g int32) {
	w := b.worst
	if d < b.dist[w] || (d == b.dist[w] && g < b.gidx[w]) {
		b.dist[w] = d
		b.gidx[w] = g
		b.worst = 0
		for j := 1; j < 5; j++ {
			if b.dist[j] > b.dist[b.worst] || (b.dist[j] == b.dist[b.worst] && b.gidx[j] > b.gidx[b.worst]) {
				b.worst = j
			}
		}
	}
}

func aabbLowerBound(box *kdBox, q *[Stride]int16) int64 {
	var s int64
	for d := 0; d < Stride; d++ {
		var diff int64
		if q[d] < box.lo[d] {
			diff = int64(box.lo[d]) - int64(q[d])
		} else if q[d] > box.hi[d] {
			diff = int64(q[d]) - int64(box.hi[d])
		} else {
			continue
		}
		s += diff * diff
	}
	return s
}

func (kd *KD) searchPart(key int, q *[Stride]int16, b *best5, stack []int32) []int32 {
	p := &kd.parts[key]
	if p.count == 0 {
		return stack
	}
	stack = append(stack[:0], 0)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if aabbLowerBound(&p.boxes[n], q) > b.dist[b.worst] {
			continue
		}
		nd := &p.nodes[n]
		if nd.splitDim == leafMark {
			for r := nd.left; r < nd.right; r++ {
				rv := (*[Stride]int16)(p.vec[int(r)*Stride:])
				b.consider(sqDist(q, rv), p.gidx[r])
			}
			continue
		}
		delta := int64(q[nd.splitDim]) - int64(nd.splitVal)
		if delta <= 0 {
			if delta*delta <= b.dist[b.worst] {
				stack = append(stack, nd.right)
			}
			stack = append(stack, nd.left)
		} else {
			if delta*delta <= b.dist[b.worst] {
				stack = append(stack, nd.left)
			}
			stack = append(stack, nd.right)
		}
	}
	return stack
}

func (kd *KD) Warmup(n int) {
	var q [Stride]int16
	cnt := 0
	for k := range kd.parts {
		p := &kd.parts[k]
		for i := 0; i < p.count && cnt < n; i++ {
			copy(q[:], p.vec[i*Stride:i*Stride+Stride])
			kd.FraudCount5(&q)
			cnt++
		}
	}
}

func (kd *KD) FraudCount5(q *[Stride]int16) int {
	return kd.count(q, true)
}

func (kd *KD) FraudCount5All(q *[Stride]int16) int {
	return kd.count(q, false)
}

func (kd *KD) count(q *[Stride]int16, stop bool) int {
	var b best5
	b.init()
	stack := make([]int32, 0, 64)
	key := partKey16(q[:])
	stack = kd.searchPart(key, q, &b, stack)
	for _, ob := range kd.probeOrder[key] {
		if stop && gapBits(key, ob) > b.dist[b.worst] {
			break
		}
		stack = kd.searchPart(ob, q, &b, stack)
	}
	cnt := 0
	for _, g := range b.gidx {
		if kd.fraud[g] {
			cnt++
		}
	}
	return cnt
}

func (kd *KD) Neighbors5(q *[Stride]int16) [5]int32 {
	var b best5
	b.init()
	stack := make([]int32, 0, 64)
	key := partKey16(q[:])
	stack = kd.searchPart(key, q, &b, stack)
	for _, ob := range kd.probeOrder[key] {
		if gapBits(key, ob) > b.dist[b.worst] {
			break
		}
		stack = kd.searchPart(ob, q, &b, stack)
	}
	g := b.gidx
	sort.Slice(g[:], func(i, j int) bool { return g[i] < g[j] })
	return g
}
