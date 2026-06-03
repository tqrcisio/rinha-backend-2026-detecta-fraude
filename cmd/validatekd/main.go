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
	bruteN := flag.Int("bruten", 4000, "neighbor-set cross-check sample vs brute")
	idx := flag.String("idx", "", "load mmap index.bin instead of building in-memory")
	flag.Parse()

	refs, err := index.Load(*gz, *bin)
	if err != nil {
		die(err)
	}
	t0 := time.Now()
	var kd *index.KD
	if *idx != "" {
		kd, err = index.Open(*idx)
		if err != nil {
			die(err)
		}
		fmt.Printf("refs=%d, KD mmap'd from %s in %s\n", refs.N, *idx, time.Since(t0).Round(time.Millisecond))
	} else {
		kd = index.BuildKD(refs)
		fmt.Printf("refs=%d, KD built in %s\n", refs.N, time.Since(t0).Round(time.Millisecond))
	}

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

	var vsExpected, fp, fn int
	for i := range qi {
		c := kd.FraudCount5(&qi[i])
		if c != expected[i] {
			vsExpected++
			pa, ea := c < 3, expected[i] < 3
			if pa && !ea {
				fn++
			} else if !pa && ea {
				fp++
			}
		}
	}
	fmt.Printf("\nGATE 2  KD vs official labels: %d/%d score mismatches (%.4f%%) [FP=%d FN=%d]\n", vsExpected, len(qi), 100*float64(vsExpected)/float64(len(qi)), fp, fn)

	stopDiff := 0
	for i := range qi {
		if kd.FraudCount5(&qi[i]) != kd.FraudCount5All(&qi[i]) {
			stopDiff++
		}
	}
	fmt.Printf("GATE 3  gap-stop vs probe-all: %d/%d differences\n", stopDiff, len(qi))

	lat := make([]time.Duration, len(qi))
	for i := range qi {
		s := time.Now()
		kd.FraudCount5(&qi[i])
		lat[i] = time.Since(s)
	}
	sort.Slice(lat, func(a, b int) bool { return lat[a] < lat[b] })
	fmt.Printf("\nLATENCY serial single-core (n=%d):\n", len(lat))
	fmt.Printf("  p50=%v  p90=%v  p99=%v  p999=%v  max=%v\n",
		lat[len(lat)*50/100], lat[len(lat)*90/100], lat[len(lat)*99/100], lat[len(lat)*999/1000], lat[len(lat)-1])

	m := *bruteN
	if m > len(qi) {
		m = len(qi)
	}
	nbrDiff := 0
	for i := 0; i < m; i++ {
		kn := kd.Neighbors5(&qi[i])
		bn := refs.Neighbors5(&qf[i])
		if kn != bn {
			nbrDiff++
		}
	}
	fmt.Printf("GATE 1  KD neighbor-set vs brute (n=%d): %d differences\n", m, nbrDiff)
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
