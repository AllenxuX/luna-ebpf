package collector

import (
	"errors"
	"testing"
	"unsafe"
)

type fakeIterator struct {
	rows map[TrafficKey][]TrafficValue
	err  error
}

func (f fakeIterator) Iterate(fn func(TrafficKey, []TrafficValue)) error {
	for k, v := range f.rows {
		fn(k, v)
	}
	return f.err
}
func TestReadTotals(t *testing.T) {
	if unsafe.Sizeof(TrafficKey{}) != 16 || unsafe.Sizeof(TrafficValue{}) != 16 {
		t.Fatal("BPF ABI")
	}
	rows := fakeIterator{rows: map[TrafficKey][]TrafficValue{
		{CgroupID: 7}:               {{Bytes: 2, Packets: 1}, {Bytes: 3, Packets: 2}},
		{CgroupID: 7, Direction: 1}: {{Bytes: 9, Packets: 4}},
		{CgroupID: 0}:               {{Bytes: 100}}, {CgroupID: 8}: {{Bytes: 100}},
	}}
	got, err := ReadTotals(rows, map[uint64]bool{7: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[TrafficKey{CgroupID: 7}] != (TrafficValue{5, 3}) {
		t.Fatalf("%v", got)
	}
}
func TestIterationError(t *testing.T) {
	if _, err := ReadTotals(fakeIterator{err: errors.New("iterate")}, nil); err == nil {
		t.Fatal("missing error")
	}
}
