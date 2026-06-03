package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/vectorize"
)

type entry struct {
	Request    vectorize.Payload `json:"request"`
	Approved   bool              `json:"expected_approved"`
	FraudScore float64           `json:"expected_fraud_score"`
}

func main() {
	refs, err := index.Load("/home/tarcisio/rinha/rinha-de-backend-2026/resources/references.json.gz", "/tmp/rinha-refs.bin")
	if err != nil { panic(err) }
	nrm, err := vectorize.Load("resources/normalization.json", "resources/mcc_risk.json")
	if err != nil { panic(err) }
	b, _ := os.ReadFile("/home/tarcisio/rinha/rinha-de-backend-2026/test/test-data.json")
	var d struct{ Entries []entry `json:"entries"` }
	json.Unmarshal(b, &d)

	type rec struct{ gap6, gap5 int64; n5 int }
	out := make([]rec, len(d.Entries))
	q := make([][index.Stride]float64, len(d.Entries))
	for i := range d.Entries {
		v := nrm.Vectorize(&d.Entries[i].Request)
		for k := 0; k < vectorize.Dims; k++ { q[i][k] = math.Round(v[k]*10000) }
	}
	nw := runtime.NumCPU(); ch := make(chan int, nw); var wg sync.WaitGroup
	for w := 0; w < nw; w++ { wg.Add(1); go func(){ defer wg.Done()
		for i := range ch {
			dists := make([]int64, 0, 16)
			// collect all distances? too big. instead track top 6
			var best [6]int64; for j := range best { best[j]=math.MaxInt64 }
			for r := 0; r < refs.N; r++ {
				base := r*index.Stride; var dd int64
				for k:=0;k<index.Stride;k++{ e:=int64(q[i][k])-int64(refs.Vec[base+k]); dd+=e*e }
				if dd>=best[5]{continue}
				p:=5; for p>0 && dd<best[p-1]{best[p]=best[p-1];p--}; best[p]=dd
			}
			_=dists
			out[i]=rec{gap6:best[5]-best[4], gap5:best[4]-best[3]}
			_=sort.Ints
		}
	}() }
	for i := range d.Entries { ch<-i }; close(ch); wg.Wait()

	// report tie risk only on boundary entries (0.4,0.6,0.8) where a flip matters
	var bndZeroGap6, bndTotal, edgeZeroGap6, edge int
	for i,e := range d.Entries {
		if e.FraudScore==0.4||e.FraudScore==0.6||e.FraudScore==0.8 {
			bndTotal++
			if out[i].gap6==0 { bndZeroGap6++ }
			if e.FraudScore==0.6 { edge++; if out[i].gap6==0 { edgeZeroGap6++ } }
		}
	}
	fmt.Printf("boundary entries(0.4/0.6/0.8): %d ; with 5th==6th tie(gap6==0): %d (%.3f%%)\n", bndTotal, bndZeroGap6, 100*float64(bndZeroGap6)/float64(bndTotal))
	fmt.Printf("edge entries(0.6): %d ; with 5th==6th tie: %d (%.3f%%)\n", edge, edgeZeroGap6, 100*float64(edgeZeroGap6)/float64(edge))
}
