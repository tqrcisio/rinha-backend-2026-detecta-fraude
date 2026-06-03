package index

import (
	"runtime"
	"sort"
	"sync"
)

type neighbor struct {
	dist float64
	idx  int
}

func (r *Refs) FraudCount5(q *[Stride]float64) int {
	var best [5]neighbor
	for i := range best {
		best[i] = neighbor{dist: 1e30, idx: 1 << 30}
	}
	worst := 0
	for i := 0; i < r.N; i++ {
		base := i * Stride
		var d float64
		for k := 0; k < Stride; k++ {
			diff := q[k] - float64(r.Vec[base+k])
			d += diff * diff
		}
		w := best[worst]
		if d < w.dist || (d == w.dist && i < w.idx) {
			best[worst] = neighbor{dist: d, idx: i}
			worst = 0
			for j := 1; j < 5; j++ {
				bj, bw := best[j], best[worst]
				if bj.dist > bw.dist || (bj.dist == bw.dist && bj.idx > bw.idx) {
					worst = j
				}
			}
		}
	}
	cnt := 0
	for _, b := range best {
		if r.Fraud[b.idx] {
			cnt++
		}
	}
	return cnt
}

func (r *Refs) Neighbors5(q *[Stride]float64) [5]int32 {
	var best [5]neighbor
	for i := range best {
		best[i] = neighbor{dist: 1e30, idx: 1 << 30}
	}
	worst := 0
	for i := 0; i < r.N; i++ {
		base := i * Stride
		var d float64
		for k := 0; k < Stride; k++ {
			diff := q[k] - float64(r.Vec[base+k])
			d += diff * diff
		}
		w := best[worst]
		if d < w.dist || (d == w.dist && i < w.idx) {
			best[worst] = neighbor{dist: d, idx: i}
			worst = 0
			for j := 1; j < 5; j++ {
				bj, bw := best[j], best[worst]
				if bj.dist > bw.dist || (bj.dist == bw.dist && bj.idx > bw.idx) {
					worst = j
				}
			}
		}
	}
	var out [5]int32
	for i, b := range best {
		out[i] = int32(b.idx)
	}
	sort.Slice(out[:], func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (r *Refs) FraudCounts(queries [][Stride]float64) []int {
	out := make([]int, len(queries))
	workers := runtime.NumCPU()
	chunk := (len(queries) + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo := w * chunk
		hi := lo + chunk
		if hi > len(queries) {
			hi = len(queries)
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				q := queries[i]
				out[i] = r.FraudCount5(&q)
			}
		}(lo, hi)
	}
	wg.Wait()
	return out
}
