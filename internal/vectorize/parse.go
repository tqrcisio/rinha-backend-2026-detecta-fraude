package vectorize

import (
	"math"
	"strconv"
	"unsafe"
)

type parsed struct {
	b            []byte
	amount       float64
	customerAvg  float64
	merchantAvg  float64
	kmHome       float64
	kmLast       float64
	ts           int64
	lastTS       int64
	installments int32
	txCount24h   int32
	isOnline     bool
	cardPresent  bool
	hasLastTx    bool
	kmStart      int
	kmEnd        int
	midStart     int
	midLen       int
	mccStart     int
	mccLen       int
}

type cur struct {
	b   []byte
	p   int
	end int
}

func (s *cur) ws() {
	for s.p < s.end {
		c := s.b[s.p]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return
		}
		s.p++
	}
}

func (s *cur) str() (cs, ce int, ok bool) {
	if s.p >= s.end || s.b[s.p] != '"' {
		return 0, 0, false
	}
	s.p++
	cs = s.p
	for s.p < s.end {
		switch s.b[s.p] {
		case '\\':
			s.p += 2
			continue
		case '"':
			ce = s.p
			s.p++
			return cs, ce, true
		}
		s.p++
	}
	return 0, 0, false
}

func (s *cur) skip() bool {
	s.ws()
	if s.p >= s.end {
		return false
	}
	switch s.b[s.p] {
	case '"':
		_, _, ok := s.str()
		return ok
	case '{', '[':
		open := s.b[s.p]
		clo := byte('}')
		if open == '[' {
			clo = ']'
		}
		depth := 0
		for s.p < s.end {
			c := s.b[s.p]
			switch c {
			case '"':
				if _, _, ok := s.str(); !ok {
					return false
				}
				continue
			case open:
				depth++
			case clo:
				depth--
				if depth == 0 {
					s.p++
					return true
				}
			}
			s.p++
		}
		return false
	default:
		for s.p < s.end {
			c := s.b[s.p]
			if c == ',' || c == '}' || c == ']' {
				break
			}
			s.p++
		}
		return true
	}
}

var pow10 = [...]float64{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18}

func (s *cur) num() (float64, bool) {
	start := s.p
	neg := false
	if s.p < s.end && (s.b[s.p] == '-' || s.b[s.p] == '+') {
		neg = s.b[s.p] == '-'
		s.p++
	}
	var mant uint64
	digits, frac := 0, 0
	for s.p < s.end && s.b[s.p] >= '0' && s.b[s.p] <= '9' {
		mant = mant*10 + uint64(s.b[s.p]-'0')
		s.p++
		digits++
	}
	if s.p < s.end && s.b[s.p] == '.' {
		s.p++
		for s.p < s.end && s.b[s.p] >= '0' && s.b[s.p] <= '9' {
			mant = mant*10 + uint64(s.b[s.p]-'0')
			s.p++
			digits++
			frac++
		}
	}
	if (s.p < s.end && (s.b[s.p] == 'e' || s.b[s.p] == 'E')) || digits > 18 {
		for s.p < s.end {
			c := s.b[s.p]
			if (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.' || c == 'e' || c == 'E' {
				s.p++
				continue
			}
			break
		}
		f, err := strconv.ParseFloat(unsafe.String(&s.b[start], s.p-start), 64)
		return f, err == nil
	}
	if digits == 0 {
		return 0, false
	}
	val := float64(mant)
	if frac > 0 {
		val /= pow10[frac]
	}
	if neg {
		val = -val
	}
	return val, true
}

func (s *cur) nextKey() (key []byte, more, ok bool) {
	s.ws()
	if s.p >= s.end {
		return nil, false, false
	}
	if s.b[s.p] == '}' {
		s.p++
		return nil, false, true
	}
	cs, ce, o := s.str()
	if !o {
		return nil, false, false
	}
	s.ws()
	if s.p >= s.end || s.b[s.p] != ':' {
		return nil, false, false
	}
	s.p++
	s.ws()
	return s.b[cs:ce], true, true
}

func (s *cur) afterValue() {
	s.ws()
	if s.p < s.end && s.b[s.p] == ',' {
		s.p++
	}
}

func (s *cur) obj() bool {
	s.ws()
	if s.p >= s.end || s.b[s.p] != '{' {
		return false
	}
	s.p++
	return true
}

func parse(body []byte, r *parsed) bool {
	*r = parsed{b: body}
	s := cur{b: body, end: len(body)}
	if !s.obj() {
		return false
	}
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			break
		}
		switch string(key) {
		case "transaction":
			if !s.transaction(r) {
				return false
			}
		case "customer":
			if !s.customer(r) {
				return false
			}
		case "merchant":
			if !s.merchant(r) {
				return false
			}
		case "terminal":
			if !s.terminal(r) {
				return false
			}
		case "last_transaction":
			if !s.lastTx(r) {
				return false
			}
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
	return true
}

func (s *cur) transaction(r *parsed) bool {
	if !s.obj() {
		return false
	}
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			return true
		}
		switch string(key) {
		case "amount":
			v, o := s.num()
			if !o {
				return false
			}
			r.amount = v
		case "installments":
			v, o := s.num()
			if !o {
				return false
			}
			r.installments = int32(v)
		case "requested_at":
			cs, ce, o := s.str()
			if !o {
				return false
			}
			r.ts = parseISO8601(s.b[cs:ce])
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
}

func (s *cur) customer(r *parsed) bool {
	if !s.obj() {
		return false
	}
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			return true
		}
		switch string(key) {
		case "avg_amount":
			v, o := s.num()
			if !o {
				return false
			}
			r.customerAvg = v
		case "tx_count_24h":
			v, o := s.num()
			if !o {
				return false
			}
			r.txCount24h = int32(v)
		case "known_merchants":
			s.ws()
			if s.p < s.end && s.b[s.p] == '[' {
				start := s.p
				if !s.skip() {
					return false
				}
				r.kmStart, r.kmEnd = start, s.p
			} else if !s.skip() {
				return false
			}
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
}

func (s *cur) merchant(r *parsed) bool {
	if !s.obj() {
		return false
	}
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			return true
		}
		switch string(key) {
		case "id":
			cs, ce, o := s.str()
			if !o {
				return false
			}
			r.midStart, r.midLen = cs, ce-cs
		case "mcc":
			cs, ce, o := s.str()
			if !o {
				return false
			}
			r.mccStart, r.mccLen = cs, ce-cs
		case "avg_amount":
			v, o := s.num()
			if !o {
				return false
			}
			r.merchantAvg = v
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
}

func (s *cur) terminal(r *parsed) bool {
	if !s.obj() {
		return false
	}
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			return true
		}
		switch string(key) {
		case "is_online":
			r.isOnline = s.p < s.end && s.b[s.p] == 't'
			if !s.skip() {
				return false
			}
		case "card_present":
			r.cardPresent = s.p < s.end && s.b[s.p] == 't'
			if !s.skip() {
				return false
			}
		case "km_from_home":
			v, o := s.num()
			if !o {
				return false
			}
			r.kmHome = v
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
}

func (s *cur) lastTx(r *parsed) bool {
	s.ws()
	if s.p+4 <= s.end && string(s.b[s.p:s.p+4]) == "null" {
		r.hasLastTx = false
		s.p += 4
		return true
	}
	if !s.obj() {
		return false
	}
	r.hasLastTx = true
	for {
		key, more, ok := s.nextKey()
		if !ok {
			return false
		}
		if !more {
			return true
		}
		switch string(key) {
		case "timestamp":
			cs, ce, o := s.str()
			if !o {
				return false
			}
			r.lastTS = parseISO8601(s.b[cs:ce])
		case "km_from_current":
			v, o := s.num()
			if !o {
				return false
			}
			r.kmLast = v
		default:
			if !s.skip() {
				return false
			}
		}
		s.afterValue()
	}
}

func knownMerchant(r *parsed) bool {
	if r.kmEnd <= r.kmStart || r.midLen <= 0 {
		return false
	}
	arr := r.b[r.kmStart:r.kmEnd]
	id := r.b[r.midStart : r.midStart+r.midLen]
	last := len(arr) - (r.midLen + 2)
	for i := 0; i <= last; i++ {
		if arr[i] != '"' || arr[i+r.midLen+1] != '"' {
			continue
		}
		match := true
		for k := 0; k < r.midLen; k++ {
			if arr[i+1+k] != id[k] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func atoi2(b []byte) int64 {
	return int64(b[0]-'0')*10 + int64(b[1]-'0')
}

func daysFromCivil(y int64, m, d int) int64 {
	if m <= 2 {
		y--
	}
	var era int64
	if y >= 0 {
		era = y / 400
	} else {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mm := int64(m)
	var mp int64
	if mm > 2 {
		mp = mm - 3
	} else {
		mp = mm + 9
	}
	doy := (153*mp+2)/5 + int64(d) - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

func parseISO8601(b []byte) int64 {
	if len(b) < 19 {
		return 0
	}
	y := int64(b[0]-'0')*1000 + int64(b[1]-'0')*100 + int64(b[2]-'0')*10 + int64(b[3]-'0')
	mo := int(atoi2(b[5:7]))
	d := int(atoi2(b[8:10]))
	h := atoi2(b[11:13])
	mi := atoi2(b[14:16])
	se := atoi2(b[17:19])
	ep := daysFromCivil(y, mo, d)*86400 + h*3600 + mi*60 + se
	if len(b) > 19 {
		c := b[19]
		if c == '+' || c == '-' {
			oh := atoi2(b[20:22])
			var om int64
			if len(b) >= 25 {
				om = atoi2(b[23:25])
			}
			off := oh*3600 + om*60
			if c == '+' {
				ep -= off
			} else {
				ep += off
			}
		}
	}
	return ep
}

func (n *Normalizer) QueryFromBody(body []byte, q *[Dims]int16) bool {
	var r parsed
	if !parse(body, &r) {
		return false
	}
	c := &n.c

	q[0] = q01(r.amount / c.MaxAmount)
	q[1] = q01(float64(r.installments) / c.MaxInstallments)
	if r.customerAvg > 0 {
		q[2] = q01((r.amount / r.customerAvg) / c.AmountVsAvgRatio)
	} else {
		q[2] = QScale
	}

	wd := (r.ts/86400 + 3) % 7
	wd = (wd + 7) % 7
	hr := (r.ts / 3600) % 24
	hr = (hr + 24) % 24
	q[3] = q01(float64(hr) / 23.0)
	q[4] = q01(float64(wd) / 6.0)

	if r.hasLastTx {
		q[5] = q01((float64(r.ts-r.lastTS) / 60.0) / c.MaxMinutes)
		q[6] = q01(r.kmLast / c.MaxKm)
	} else {
		q[5] = -QScale
		q[6] = -QScale
	}

	q[7] = q01(r.kmHome / c.MaxKm)
	q[8] = q01(float64(r.txCount24h) / c.MaxTxCount24h)
	q[9] = boolq(r.isOnline)
	q[10] = boolq(r.cardPresent)
	q[11] = boolq(!knownMerchant(&r))
	q[12] = q01(n.mcc.Risk(mccStr(&r)))
	q[13] = q01(r.merchantAvg / c.MaxMerchantAvgAmount)
	return true
}

func q01(x float64) int16 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return QScale
	}
	return int16(math.Round(x * QScale))
}

func boolq(b bool) int16 {
	if b {
		return QScale
	}
	return 0
}

func mccStr(r *parsed) string {
	if r.mccLen <= 0 {
		return ""
	}
	return unsafe.String(&r.b[r.mccStart], r.mccLen)
}
