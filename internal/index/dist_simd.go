//go:build goexperiment.simd

package index

import "simd/archsimd"

func sqDist(q, r *[Stride]int16) int64 {
	d := archsimd.LoadInt16x16(q).Sub(archsimd.LoadInt16x16(r))
	var out [8]int32
	d.DotProductPairs(d).Store(&out)
	return int64(out[0]) + int64(out[1]) + int64(out[2]) + int64(out[3]) +
		int64(out[4]) + int64(out[5]) + int64(out[6]) + int64(out[7])
}
