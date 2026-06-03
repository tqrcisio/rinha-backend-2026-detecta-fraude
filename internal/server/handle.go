package server

import (
	"bytes"
	"fmt"
	"unsafe"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/vectorize"
)

var (
	crlf2      = []byte("\r\n\r\n")
	methodPost = []byte("POST")
	fraudPath  = []byte("/fraud-score")
	readyPath  = []byte("/ready")
	clHeader   = []byte("content-length:")
)

type Handler struct {
	nrm      *vectorize.Normalizer
	kd       *index.KD
	resp     [6][]byte
	ready    []byte
	notfound []byte
}

func NewHandler(nrm *vectorize.Normalizer, kd *index.KD) *Handler {
	h := &Handler{nrm: nrm, kd: kd}
	scores := [6]string{"0.0", "0.2", "0.4", "0.6", "0.8", "1.0"}
	for c := 0; c < 6; c++ {
		body := fmt.Sprintf(`{"approved":%t,"fraud_score":%s}`, c < 3, scores[c])
		h.resp[c] = httpOK(body)
	}
	h.ready = httpOK(`{"status":"ok"}`)
	h.notfound = []byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
	return h
}

func httpOK(body string) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
}

func (h *Handler) handle(c *conn) bool {
	off := 0
	for {
		he := bytes.Index(c.rb[off:], crlf2)
		if he < 0 {
			break
		}
		headEnd := off + he
		post, path := parseReqLine(c.rb[off:headEnd])
		bodyLen := 0
		if post {
			bodyLen = contentLength(c.rb[off:headEnd])
		}
		total := headEnd + 4 + bodyLen
		if len(c.rb) < total {
			break
		}
		switch {
		case post && bytes.Equal(path, fraudPath):
			c.wb = append(c.wb, h.score(c.rb[headEnd+4:total])...)
		case bytes.Equal(path, readyPath):
			c.wb = append(c.wb, h.ready...)
		default:
			c.wb = append(c.wb, h.notfound...)
		}
		off = total
	}
	if off > 0 {
		n := copy(c.rb, c.rb[off:])
		c.rb = c.rb[:n]
	}
	return true
}

func (h *Handler) score(body []byte) []byte {
	var q [index.Stride]int16
	qd := (*[vectorize.Dims]int16)(unsafe.Pointer(&q))
	if !h.nrm.QueryFromBody(body, qd) {
		return h.resp[0]
	}
	return h.resp[h.kd.FraudCount5(&q)]
}

func parseReqLine(head []byte) (post bool, path []byte) {
	sp := bytes.IndexByte(head, ' ')
	if sp < 0 {
		return false, nil
	}
	post = bytes.Equal(head[:sp], methodPost)
	rest := head[sp+1:]
	sp2 := bytes.IndexByte(rest, ' ')
	if sp2 < 0 {
		return post, nil
	}
	return post, rest[:sp2]
}

func contentLength(head []byte) int {
	i := indexFold(head, clHeader)
	if i < 0 {
		return 0
	}
	j := i + len(clHeader)
	for j < len(head) && (head[j] == ' ' || head[j] == '\t') {
		j++
	}
	n := 0
	for j < len(head) && head[j] >= '0' && head[j] <= '9' {
		n = n*10 + int(head[j]-'0')
		j++
	}
	return n
}

func indexFold(b, sub []byte) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		k := 0
		for ; k < len(sub); k++ {
			x := b[i+k]
			if x >= 'A' && x <= 'Z' {
				x += 'a' - 'A'
			}
			if x != sub[k] {
				break
			}
		}
		if k == len(sub) {
			return i
		}
	}
	return -1
}
