package vectorize

import (
	"math"
	"testing"
)

func newNormalizer() *Normalizer {
	return &Normalizer{
		c: Consts{
			MaxAmount:            10000,
			MaxInstallments:      12,
			AmountVsAvgRatio:     10,
			MaxMinutes:           1440,
			MaxKm:                1000,
			MaxTxCount24h:        20,
			MaxMerchantAvgAmount: 10000,
		},
		mcc: MccRisk{
			"5411": 0.15, "5812": 0.30, "5912": 0.20, "5944": 0.45,
			"7801": 0.80, "7802": 0.75, "7995": 0.85, "4511": 0.35,
			"5311": 0.25, "5999": 0.50,
		},
	}
}

func assertVector(t *testing.T, got, want [Dims]float64) {
	t.Helper()
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-4 {
			t.Errorf("dim %d: got %.6f want %.4f", i, got[i], want[i])
		}
	}
}

func TestVectorizeLegit(t *testing.T) {
	p := &Payload{
		Transaction: Transaction{Amount: 41.12, Installments: 2, RequestedAt: "2026-03-11T18:45:53Z"},
		Customer:    Customer{AvgAmount: 82.24, TxCount24h: 3, KnownMerchants: []string{"MERC-003", "MERC-016"}},
		Merchant:    Merchant{ID: "MERC-016", MCC: "5411", AvgAmount: 60.25},
		Terminal:    Terminal{IsOnline: false, CardPresent: true, KmFromHome: 29.23},
	}
	want := [Dims]float64{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}
	assertVector(t, newNormalizer().Vectorize(p), want)
}

func TestVectorizeFraud(t *testing.T) {
	p := &Payload{
		Transaction: Transaction{Amount: 9505.97, Installments: 10, RequestedAt: "2026-03-14T05:15:12Z"},
		Customer:    Customer{AvgAmount: 81.28, TxCount24h: 20, KnownMerchants: []string{"MERC-008", "MERC-007", "MERC-005"}},
		Merchant:    Merchant{ID: "MERC-068", MCC: "7802", AvgAmount: 54.86},
		Terminal:    Terminal{IsOnline: false, CardPresent: true, KmFromHome: 952.27},
	}
	want := [Dims]float64{0.9506, 0.8333, 1.0, 0.2174, 0.8333, -1, -1, 0.9523, 1.0, 0, 1, 1, 0.75, 0.0055}
	assertVector(t, newNormalizer().Vectorize(p), want)
}

func TestVectorizeWithLastTransaction(t *testing.T) {
	p := &Payload{
		Transaction:     Transaction{Amount: 384.88, Installments: 3, RequestedAt: "2026-03-11T20:23:35Z"},
		Customer:        Customer{AvgAmount: 769.76, TxCount24h: 3, KnownMerchants: []string{"MERC-009", "MERC-001"}},
		Merchant:        Merchant{ID: "MERC-001", MCC: "5912", AvgAmount: 298.95},
		Terminal:        Terminal{IsOnline: false, CardPresent: true, KmFromHome: 13.7090520965},
		LastTransaction: &LastTransaction{Timestamp: "2026-03-11T14:58:35Z", KmFromCurrent: 18.8626479774},
	}
	got := newNormalizer().Vectorize(p)
	if math.Abs(got[5]-325.0/1440.0) > 1e-4 {
		t.Errorf("dim 5 minutes: got %.6f", got[5])
	}
	if math.Abs(got[6]-0.0189) > 1e-4 {
		t.Errorf("dim 6 km_from_last: got %.6f", got[6])
	}
}

func TestQuantizeSentinel(t *testing.T) {
	v := [Dims]float64{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}
	q := Quantize(v)
	if q[0] != 41 || q[5] != -10000 || q[10] != 10000 {
		t.Errorf("quantize mismatch: %v", q)
	}
}
