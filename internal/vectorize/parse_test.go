package vectorize

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func randPayload(rng *rand.Rand) *Payload {
	mid := fmt.Sprintf("MERC-%03d", rng.Intn(120))
	var known []string
	for i := rng.Intn(6); i > 0; i-- {
		known = append(known, fmt.Sprintf("MERC-%03d", rng.Intn(120)))
	}
	if rng.Intn(2) == 0 {
		known = append(known, mid)
	}
	mccs := []string{"5411", "5812", "5912", "5944", "7801", "7802", "7995", "4511", "5311", "5999", "0000", "9999", "1234"}

	loc := time.UTC
	if rng.Intn(3) == 0 {
		loc = time.FixedZone("x", (rng.Intn(28)-14)*1800)
	}
	base := time.Date(2020+rng.Intn(15), time.Month(1+rng.Intn(12)), 1+rng.Intn(28),
		rng.Intn(24), rng.Intn(60), rng.Intn(60), 0, time.UTC).In(loc)

	p := &Payload{
		Transaction: Transaction{
			Amount:       float64(rng.Intn(1500000)) / 100,
			Installments: rng.Intn(16),
			RequestedAt:  base.Format(time.RFC3339),
		},
		Customer: Customer{
			AvgAmount:      float64(rng.Intn(200000)) / 100,
			TxCount24h:     rng.Intn(30),
			KnownMerchants: known,
		},
		Merchant: Merchant{
			ID:        mid,
			MCC:       mccs[rng.Intn(len(mccs))],
			AvgAmount: float64(rng.Intn(1500000)) / 100,
		},
		Terminal: Terminal{
			IsOnline:    rng.Intn(2) == 0,
			CardPresent: rng.Intn(2) == 0,
			KmFromHome:  float64(rng.Intn(150000)) / 100,
		},
	}
	if rng.Intn(2) == 0 {
		lastLoc := time.UTC
		if rng.Intn(3) == 0 {
			lastLoc = time.FixedZone("y", (rng.Intn(28)-14)*1800)
		}
		last := base.Add(-time.Duration(rng.Intn(4000)) * time.Minute).In(lastLoc)
		p.LastTransaction = &LastTransaction{
			Timestamp:     last.Format(time.RFC3339),
			KmFromCurrent: float64(rng.Intn(120000)) / 100,
		}
	}
	return p
}

func TestQueryFromBodyMatchesVectorize(t *testing.T) {
	nrm := newNormalizer()
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 200000; iter++ {
		p := randPayload(rng)
		body, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		want := Quantize(nrm.Vectorize(p))
		var got [Dims]int16
		if !nrm.QueryFromBody(body, &got) {
			t.Fatalf("QueryFromBody failed on %s", body)
		}
		for d := 0; d < Dims; d++ {
			if got[d] != want[d] {
				t.Fatalf("dim %d: got %d want %d\npayload: %s", d, got[d], want[d], body)
			}
		}
	}
}

func BenchmarkQueryFromBody(b *testing.B) {
	nrm := newNormalizer()
	body := []byte(`{"id":"tx-1","transaction":{"amount":384.88,"installments":3,"requested_at":"2026-03-11T20:23:35Z"},"customer":{"avg_amount":769.76,"tx_count_24h":3,"known_merchants":["MERC-009","MERC-001"]},"merchant":{"id":"MERC-001","mcc":"5912","avg_amount":298.95},"terminal":{"is_online":false,"card_present":true,"km_from_home":13.7},"last_transaction":{"timestamp":"2026-03-11T14:58:35Z","km_from_current":18.86}}`)
	var q [Dims]int16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nrm.QueryFromBody(body, &q)
	}
}
