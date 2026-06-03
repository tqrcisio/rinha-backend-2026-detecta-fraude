package index

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
)

const Stride = 16
const RawDims = 14

type Refs struct {
	N     int
	Vec   []int16
	Fraud []bool
}

const binMagic = "RNHREF01"

func Build(gzPath, binPath string) (*Refs, error) {
	f, err := os.Open(gzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(gz, 1<<20))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}

	refs := &Refs{Vec: make([]int16, 0, 3_000_000*Stride), Fraud: make([]bool, 0, 3_000_000)}
	var rec struct {
		Vector [RawDims]float64 `json:"vector"`
		Label  string           `json:"label"`
	}
	for dec.More() {
		if err := dec.Decode(&rec); err != nil {
			return nil, err
		}
		var q [Stride]int16
		for d := 0; d < RawDims; d++ {
			q[d] = int16(roundHalfAway(rec.Vector[d] * 10000))
		}
		refs.Vec = append(refs.Vec, q[:]...)
		refs.Fraud = append(refs.Fraud, rec.Label == "fraud")
	}
	refs.N = len(refs.Fraud)

	if err := writeBin(binPath, refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func writeBin(path string, r *Refs) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	if _, err := w.WriteString(binMagic); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, int64(r.N)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, r.Vec); err != nil {
		return err
	}
	bits := make([]byte, r.N)
	for i, fr := range r.Fraud {
		if fr {
			bits[i] = 1
		}
	}
	if _, err := w.Write(bits); err != nil {
		return err
	}
	return w.Flush()
}

func LoadBin(path string) (*Refs, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 16 || string(b[:8]) != binMagic {
		return nil, fmt.Errorf("bad magic in %s", path)
	}
	n := int(binary.LittleEndian.Uint64(b[8:16]))
	off := 16
	vec := make([]int16, n*Stride)
	for i := range vec {
		vec[i] = int16(binary.LittleEndian.Uint16(b[off : off+2]))
		off += 2
	}
	fraud := make([]bool, n)
	for i := 0; i < n; i++ {
		fraud[i] = b[off+i] == 1
	}
	return &Refs{N: n, Vec: vec, Fraud: fraud}, nil
}

func Load(gzPath, binPath string) (*Refs, error) {
	if r, err := LoadBin(binPath); err == nil {
		return r, nil
	}
	return Build(gzPath, binPath)
}

func roundHalfAway(x float64) float64 {
	if x < 0 {
		return float64(int64(x - 0.5))
	}
	return float64(int64(x + 0.5))
}
