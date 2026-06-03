package index

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const kdMagic = "RNHKD003"

type partDir struct {
	Count    int64
	NNodes   int64
	NodesOff int64
	BoxesOff int64
	VecOff   int64
	GidxOff  int64
}

func align8(n int64) int64 { return (n + 7) &^ 7 }

func WriteKD(kd *KD, n int, path string) error {
	header := int64(16)
	dirSize := int64(numBuckets) * int64(unsafe.Sizeof(partDir{}))
	off := align8(header + dirSize)

	labelsOff := off
	off = align8(off + int64(n))

	var dir [numBuckets]partDir
	for k := range kd.parts {
		p := &kd.parts[k]
		dir[k].Count = int64(p.count)
		dir[k].NNodes = int64(len(p.nodes))
		off = align8(off)
		dir[k].NodesOff = off
		off = align8(off + int64(len(p.nodes))*int64(unsafe.Sizeof(kdNode{})))
		dir[k].BoxesOff = off
		off = align8(off + int64(len(p.boxes))*int64(unsafe.Sizeof(kdBox{})))
		dir[k].VecOff = off
		off = align8(off + int64(len(p.vec))*2)
		dir[k].GidxOff = off
		off = align8(off + int64(len(p.gidx))*4)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(off); err != nil {
		return err
	}
	buf, err := unix.Mmap(int(f.Fd()), 0, int(off), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return err
	}
	defer unix.Munmap(buf)

	copy(buf[0:8], kdMagic)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(n))
	dirBytes := unsafe.Slice((*byte)(unsafe.Pointer(&dir[0])), int(dirSize))
	copy(buf[16:16+dirSize], dirBytes)

	for i := 0; i < n; i++ {
		if kd.fraud[i] {
			buf[labelsOff+int64(i)] = 1
		}
	}
	for k := range kd.parts {
		p := &kd.parts[k]
		copy(buf[dir[k].NodesOff:], asBytes(p.nodes))
		copy(buf[dir[k].BoxesOff:], asBytes(p.boxes))
		copy(buf[dir[k].VecOff:], asBytes(p.vec))
		copy(buf[dir[k].GidxOff:], asBytes(p.gidx))
	}
	return nil
}

func Open(path string) (*KD, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := int(st.Size())
	buf, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	if size < 16 || string(buf[0:8]) != kdMagic {
		return nil, fmt.Errorf("bad magic in %s", path)
	}
	unix.Madvise(buf, unix.MADV_WILLNEED)
	unix.Madvise(buf, unix.MADV_HUGEPAGE)
	unix.Mlock(buf)

	n := int(binary.LittleEndian.Uint64(buf[8:16]))
	dir := (*[numBuckets]partDir)(unsafe.Pointer(&buf[16]))

	kd := &KD{}
	kd.fraud = unsafe.Slice((*bool)(unsafe.Pointer(&buf[labelsBase()])), n)
	for k := range kd.parts {
		d := &dir[k]
		p := &kd.parts[k]
		p.count = int(d.Count)
		if d.NNodes > 0 {
			p.nodes = unsafe.Slice((*kdNode)(unsafe.Pointer(&buf[d.NodesOff])), int(d.NNodes))
			p.boxes = unsafe.Slice((*kdBox)(unsafe.Pointer(&buf[d.BoxesOff])), int(d.NNodes))
		}
		if d.Count > 0 {
			p.vec = unsafe.Slice((*int16)(unsafe.Pointer(&buf[d.VecOff])), int(d.Count)*Stride)
			p.gidx = unsafe.Slice((*int32)(unsafe.Pointer(&buf[d.GidxOff])), int(d.Count))
		}
		kd.probeOrder[k] = probeOrderFor(k)
	}
	return kd, nil
}

func labelsBase() int64 {
	dirSize := int64(numBuckets) * int64(unsafe.Sizeof(partDir{}))
	return align8(16 + dirSize)
}

func asBytes[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*int(unsafe.Sizeof(s[0])))
}
