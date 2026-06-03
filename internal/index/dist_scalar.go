//go:build !goexperiment.simd

package index

func sqDist(q, r *[Stride]int16) int64 {
	var s int64
	for i := 0; i < Stride; i++ {
		d := int64(q[i]) - int64(r[i])
		s += d * d
	}
	return s
}
