package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
)

func main() {
	gz := flag.String("gz", "resources/references.json.gz", "")
	bin := flag.String("bin", "/tmp/rinha-refs.bin", "")
	out := flag.String("out", "index.bin", "")
	flag.Parse()

	t0 := time.Now()
	refs, err := index.Load(*gz, *bin)
	if err != nil {
		log.Fatal(err)
	}
	kd := index.BuildKD(refs)
	if err := index.WriteKD(kd, refs.N, *out); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("index.bin written (n=%d) in %s\n", refs.N, time.Since(t0).Round(time.Millisecond))
}
