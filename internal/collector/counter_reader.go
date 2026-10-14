package collector

import "fmt"

// These structs match the native-endian C map ABI. RX=0, TX=1.
type TrafficKey struct {
	CgroupID  uint64
	Direction uint32
	Reserved  uint32
}
type TrafficValue struct {
	Bytes   uint64
	Packets uint64
}
type TrafficTotals struct{ RXBytes, TXBytes, RXPackets, TXPackets uint64 }
type Iterator interface {
	Iterate(func(TrafficKey, []TrafficValue)) error
}

// ReadTotals discards partial reads on error. Only explicitly registered IDs count.
func ReadTotals(it Iterator, allowed map[uint64]bool) (map[TrafficKey]TrafficValue, error) {
	out := make(map[TrafficKey]TrafficValue)
	err := it.Iterate(func(k TrafficKey, cpus []TrafficValue) {
		if k.CgroupID == 0 || !allowed[k.CgroupID] || k.Reserved != 0 || k.Direction > 1 {
			return
		}
		var sum TrafficValue
		for _, v := range cpus {
			sum.Bytes += v.Bytes
			sum.Packets += v.Packets
		}
		out[k] = sum
	})
	if err != nil {
		return nil, fmt.Errorf("iterate per-CPU traffic: %w", err)
	}
	return out, nil
}
