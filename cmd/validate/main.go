package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/vectorize"
)

type entry struct {
	Request       vectorize.Payload `json:"request"`
	Approved      bool              `json:"expected_approved"`
	FraudScore    float64           `json:"expected_fraud_score"`
	expectedCount int
}

type testData struct {
	Entries []entry `json:"entries"`
}

func main() {
	gz := flag.String("gz", "/home/tarcisio/rinha/rinha-de-backend-2026/resources/references.json.gz", "")
	bin := flag.String("bin", "/tmp/rinha-refs.bin", "")
	td := flag.String("testdata", "/home/tarcisio/rinha/rinha-de-backend-2026/test/test-data.json", "")
	norm := flag.String("norm", "resources/normalization.json", "")
	mcc := flag.String("mcc", "resources/mcc_risk.json", "")
	n := flag.Int("n", 3000, "sample size, 0 = all")
	flag.Parse()

	t0 := time.Now()
	refs, err := index.Load(*gz, *bin)
	if err != nil {
		die(err)
	}
	fmt.Printf("refs: N=%d loaded in %s\n", refs.N, time.Since(t0).Round(time.Millisecond))

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
	fmt.Printf("test entries: %d\n", len(entries))

	full := make([][index.Stride]float64, len(entries))
	rounded := make([][index.Stride]float64, len(entries))
	for i := range entries {
		v := nrm.Vectorize(&entries[i].Request)
		for d := 0; d < vectorize.Dims; d++ {
			s := v[d] * 10000
			full[i][d] = s
			rounded[i][d] = math.Round(s)
		}
	}

	run("query full precision", refs, full, entries)
	run("query rounded to int16", refs, rounded, entries)
}

func run(name string, refs *index.Refs, queries [][index.Stride]float64, entries []entry) {
	t0 := time.Now()
	counts := refs.FraudCounts(queries)
	var approvedMiss, scoreMiss, fp, fn int
	for i, c := range counts {
		e := entries[i]
		approved := c < 3
		if c != e.expectedCount {
			scoreMiss++
		}
		if approved != e.Approved {
			approvedMiss++
			if approved {
				fn++
			} else {
				fp++
			}
		}
	}
	dt := time.Since(t0)
	fmt.Printf("\n[%s] %s (%.0f q/s)\n", name, dt.Round(time.Millisecond), float64(len(queries))/dt.Seconds())
	fmt.Printf("  fraud_score mismatches: %d/%d (%.4f%%)\n", scoreMiss, len(queries), 100*float64(scoreMiss)/float64(len(queries)))
	fmt.Printf("  approved mismatches:    %d/%d (%.4f%%)  [FP=%d FN=%d]\n", approvedMiss, len(queries), 100*float64(approvedMiss)/float64(len(queries)), fp, fn)
}

func loadTestData(path string) ([]entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d testData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	for i := range d.Entries {
		d.Entries[i].expectedCount = int(math.Round(d.Entries[i].FraudScore * 5))
	}
	return d.Entries, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
