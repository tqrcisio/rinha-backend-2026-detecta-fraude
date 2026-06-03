package vectorize

import (
	"encoding/json"
	"math"
	"os"
	"time"
)

const Dims = 14
const QScale = 10000
const DefaultMccRisk = 0.5

type Consts struct {
	MaxAmount            float64 `json:"max_amount"`
	MaxInstallments      float64 `json:"max_installments"`
	AmountVsAvgRatio     float64 `json:"amount_vs_avg_ratio"`
	MaxMinutes           float64 `json:"max_minutes"`
	MaxKm                float64 `json:"max_km"`
	MaxTxCount24h        float64 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount float64 `json:"max_merchant_avg_amount"`
}

type MccRisk map[string]float64

func (m MccRisk) Risk(mcc string) float64 {
	if v, ok := m[mcc]; ok {
		return v
	}
	return DefaultMccRisk
}

type Normalizer struct {
	c   Consts
	mcc MccRisk
}

func Load(normPath, mccPath string) (*Normalizer, error) {
	n := &Normalizer{}
	if err := readJSON(normPath, &n.c); err != nil {
		return nil, err
	}
	if err := readJSON(mccPath, &n.mcc); err != nil {
		return nil, err
	}
	return n, nil
}

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

type Transaction struct {
	Amount       float64 `json:"amount"`
	Installments int     `json:"installments"`
	RequestedAt  string  `json:"requested_at"`
}

type Customer struct {
	AvgAmount      float64  `json:"avg_amount"`
	TxCount24h     int      `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

type Merchant struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float64 `json:"avg_amount"`
}

type Terminal struct {
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
	KmFromHome  float64 `json:"km_from_home"`
}

type LastTransaction struct {
	Timestamp     string  `json:"timestamp"`
	KmFromCurrent float64 `json:"km_from_current"`
}

type Payload struct {
	ID              string           `json:"id"`
	Transaction     Transaction      `json:"transaction"`
	Customer        Customer         `json:"customer"`
	Merchant        Merchant         `json:"merchant"`
	Terminal        Terminal         `json:"terminal"`
	LastTransaction *LastTransaction `json:"last_transaction"`
}

func (n *Normalizer) Vectorize(p *Payload) [Dims]float64 {
	c := &n.c
	var v [Dims]float64

	v[0] = clamp01(p.Transaction.Amount / c.MaxAmount)
	v[1] = clamp01(float64(p.Transaction.Installments) / c.MaxInstallments)

	if p.Customer.AvgAmount > 0 {
		v[2] = clamp01((p.Transaction.Amount / p.Customer.AvgAmount) / c.AmountVsAvgRatio)
	} else {
		v[2] = 1
	}

	hour, weekday, ok := hourWeekday(p.Transaction.RequestedAt)
	if ok {
		v[3] = float64(hour) / 23
		v[4] = float64(weekday) / 6
	}

	if p.LastTransaction != nil {
		v[5] = clamp01(minutesBetween(p.LastTransaction.Timestamp, p.Transaction.RequestedAt) / c.MaxMinutes)
		v[6] = clamp01(p.LastTransaction.KmFromCurrent / c.MaxKm)
	} else {
		v[5] = -1
		v[6] = -1
	}

	v[7] = clamp01(p.Terminal.KmFromHome / c.MaxKm)
	v[8] = clamp01(float64(p.Customer.TxCount24h) / c.MaxTxCount24h)
	v[9] = boolf(p.Terminal.IsOnline)
	v[10] = boolf(p.Terminal.CardPresent)
	v[11] = boolf(!known(p.Merchant.ID, p.Customer.KnownMerchants))
	v[12] = n.mcc.Risk(p.Merchant.MCC)
	v[13] = clamp01(p.Merchant.AvgAmount / c.MaxMerchantAvgAmount)

	return v
}

func Quantize(v [Dims]float64) [Dims]int16 {
	var q [Dims]int16
	for i, x := range v {
		q[i] = int16(math.Round(x * QScale))
	}
	return q
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func known(id string, list []string) bool {
	for _, m := range list {
		if m == id {
			return true
		}
	}
	return false
}

func hourWeekday(ts string) (int, int, bool) {
	tm, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0, 0, false
	}
	tm = tm.UTC()
	return tm.Hour(), (int(tm.Weekday()) + 6) % 7, true
}

func minutesBetween(last, current string) float64 {
	lt, err1 := time.Parse(time.RFC3339, last)
	ct, err2 := time.Parse(time.RFC3339, current)
	if err1 != nil || err2 != nil {
		return 0
	}
	return ct.Sub(lt).Minutes()
}
