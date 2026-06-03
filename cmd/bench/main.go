package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type entry struct {
	Request    json.RawMessage `json:"request"`
	Approved   bool            `json:"expected_approved"`
	FraudScore float64         `json:"expected_fraud_score"`
}

func main() {
	url := flag.String("url", "http://localhost:9999/fraud-score", "")
	td := flag.String("testdata", "/home/tarcisio/rinha/rinha-de-backend-2026/test/test-data.json", "")
	rate := flag.Int("rate", 900, "target requests per second")
	conns := flag.Int("conns", 64, "max in-flight connections")
	n := flag.Int("n", 0, "limit number of requests, 0 = all entries")
	flag.Parse()

	debug.SetGCPercent(-1)

	entries := loadEntries(*td)
	if *n > 0 && *n < len(entries) {
		entries = entries[:*n]
	}

	client := &http.Client{
		Timeout: 2001 * time.Millisecond,
		Transport: &http.Transport{
			MaxIdleConns:        *conns,
			MaxIdleConnsPerHost: *conns,
			MaxConnsPerHost:     *conns,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true,
		},
	}

	lat := make([]time.Duration, len(entries))
	approved := make([]int32, len(entries))
	var errs int64

	sem := make(chan struct{}, *conns)
	var wg sync.WaitGroup
	interval := time.Second / time.Duration(*rate)
	start := time.Now()
	next := start

	for i := range entries {
		next = next.Add(interval)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			t0 := time.Now()
			ok, app := do(client, *url, entries[i].Request)
			lat[i] = time.Since(t0)
			if !ok {
				atomic.AddInt64(&errs, 1)
				approved[i] = -1
				return
			}
			if app {
				approved[i] = 1
			} else {
				approved[i] = 0
			}
		}(i)
	}
	wg.Wait()
	wall := time.Since(start)

	report(entries, lat, approved, int(errs), wall, *rate)
}

func do(c *http.Client, url string, body []byte) (ok bool, approved bool) {
	resp, err := c.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return false, false
	}
	var r struct {
		Approved bool `json:"approved"`
	}
	if json.Unmarshal(b, &r) != nil {
		return false, false
	}
	return true, r.Approved
}

func report(entries []entry, lat []time.Duration, approved []int32, errs int, wall time.Duration, rate int) {
	var tp, tn, fp, fn int
	for i := range entries {
		switch approved[i] {
		case -1:
			continue
		case 1:
			if entries[i].Approved {
				tn++
			} else {
				fn++
			}
		case 0:
			if entries[i].Approved {
				fp++
			} else {
				tp++
			}
		}
	}
	ok := make([]time.Duration, 0, len(lat))
	for i := range lat {
		if approved[i] != -1 {
			ok = append(ok, lat[i])
		}
	}
	sort.Slice(ok, func(a, b int) bool { return ok[a] < ok[b] })

	n := len(entries)
	p99ms := pct(ok, 0.99).Seconds() * 1000
	fmt.Printf("requests=%d  wall=%s  achieved=%.0f rps (target %d)\n", n, wall.Round(time.Millisecond), float64(n)/wall.Seconds(), rate)
	fmt.Printf("latency: p50=%v p90=%v p99=%v p999=%v max=%v\n",
		pct(ok, 0.50), pct(ok, 0.90), pct(ok, 0.99), pct(ok, 0.999), ok[len(ok)-1])
	fmt.Printf("detection: TP=%d TN=%d FP=%d FN=%d errors=%d\n", tp, tn, fp, fn, errs)

	score, p99s, dets := finalScore(fp, fn, errs, n, p99ms)
	fmt.Printf("estimated score: p99_score=%.1f detection_score=%.1f  final=%.1f\n", p99s, dets, score)
}

func pct(s []time.Duration, p float64) time.Duration {
	if len(s) == 0 {
		return 0
	}
	i := int(float64(len(s)) * p)
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

func finalScore(fp, fn, errs, n int, p99ms float64) (final, p99Score, detScore float64) {
	const K, tMax, p99Min, p99Max, epsMin, beta = 1000.0, 1000.0, 1.0, 2000.0, 0.001, 300.0
	if p99ms <= 0 {
		p99Score = 0
	} else if p99ms > p99Max {
		p99Score = -3000
	} else {
		p99Score = K * math.Log10(tMax/math.Max(p99ms, p99Min))
		if p99Score > 3000 {
			p99Score = 3000
		}
	}
	e := float64(fp*1 + fn*3 + errs*5)
	failures := fp + fn + errs
	failRate := float64(failures) / float64(n)
	if failRate > 0.15 {
		detScore = -3000
	} else {
		eps := e / float64(n)
		detScore = K*math.Log10(1/math.Max(eps, epsMin)) - beta*math.Log10(1+e)
	}
	return p99Score + detScore, p99Score, detScore
}

func loadEntries(path string) []entry {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var d struct {
		Entries []entry `json:"entries"`
	}
	if json.Unmarshal(b, &d) != nil {
		fmt.Fprintln(os.Stderr, "bad testdata")
		os.Exit(1)
	}
	return d.Entries
}
