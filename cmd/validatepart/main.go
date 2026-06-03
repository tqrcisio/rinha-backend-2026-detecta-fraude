package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/vectorize"
)

type entry struct {
	Request    vectorize.Payload `json:"request"`
	Approved   bool              `json:"expected_approved"`
	FraudScore float64           `json:"expected_fraud_score"`
}

func main() {
	gz := flag.String("gz", "/home/tarcisio/rinha/rinha-de-backend-2026/resources/references.json.gz", "")
	bin := flag.String("bin", "/tmp/rinha-refs.bin", "")
	td := flag.String("testdata", "/home/tarcisio/rinha/rinha-de-backend-2026/test/test-data.json", "")
	norm := flag.String("norm", "resources/normalization.json", "")
	mcc := flag.String("mcc", "resources/mcc_risk.json", "")
	n := flag.Int("n", 0, "sample size, 0 = all")
	withOracle := flag.Bool("oracle", false, "also run brute-force cross-check")
	flag.Parse()

	refs, err := index.Load(*gz, *bin)
	if err != nil {
		die(err)
	}
	ix := index.BuildIndex(refs)
	fmt.Printf("refs=%d\n", refs.N)
	printBuckets(ix)

	nrm, err := vectorize.Load(*norm, *mcc)
	if err != nil {
		die(err)
	}
	entries, err := loadTestData(*td)
	if err != nil {
		die(err)
	}
	if *n > 0 && *n < len(entries) {
		entries = entries[:*n]
	}

	qi := make([][index.Stride]int16, len(entries))
	qf := make([][index.Stride]float64, len(entries))
	expected := make([]int, len(entries))
	for i := range entries {
		v := nrm.Vectorize(&entries[i].Request)
		for d := 0; d < vectorize.Dims; d++ {
			r := math.Round(v[d] * 10000)
			qi[i][d] = int16(r)
			qf[i][d] = r
		}
		expected[i] = int(math.Round(entries[i].FraudScore * 5))
	}

	t0 := time.Now()
	part := make([]int, len(entries))
	for i := range qi {
		part[i] = ix.FraudCount5(&qi[i])
	}
	dt := time.Since(t0)

	var vsExpected, fp, fn int
	for i := range part {
		if part[i] != expected[i] {
			vsExpected++
			pa, ea := part[i] < 3, expected[i] < 3
			if pa && !ea {
				fn++
			} else if !pa && ea {
				fp++
			}
		}
	}
	fmt.Printf("\nqueries=%d\n", len(entries))
	fmt.Printf("partitioned vs expected: %d score mismatches (%.4f%%)  [FP=%d FN=%d]\n", vsExpected, 100*float64(vsExpected)/float64(len(entries)), fp, fn)

	if *withOracle {
		oracle := refs.FraudCounts(qf)
		vsBrute := 0
		for i := range part {
			if part[i] != oracle[i] {
				vsBrute++
			}
		}
		fmt.Printf("partitioned vs brute-force: %d mismatches\n", vsBrute)
	}

	lat := latencies(ix, qi)
	fmt.Printf("\nlatency serial single-core: total=%s mean=%s\n", dt.Round(time.Millisecond), (dt / time.Duration(len(entries))).Round(time.Nanosecond))
	fmt.Printf("  p50=%v p99=%v p999=%v max=%v\n", lat[len(lat)*50/100], lat[len(lat)*99/100], lat[len(lat)*999/1000], lat[len(lat)-1])
}

func latencies(ix *index.Index, qi [][index.Stride]int16) []time.Duration {
	out := make([]time.Duration, len(qi))
	for i := range qi {
		s := time.Now()
		ix.FraudCount5(&qi[i])
		out[i] = time.Since(s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func printBuckets(ix *index.Index) {
	sizes := ix.BucketSizes()
	mx := 0
	for _, s := range sizes {
		if s > mx {
			mx = s
		}
	}
	fmt.Printf("buckets (max=%d): %v\n", mx, sizes)
}

func loadTestData(path string) ([]entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return d.Entries, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
