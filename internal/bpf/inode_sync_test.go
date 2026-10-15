package bpf

import (
	"errors"
	"fmt"
	"testing"
)

func TestSyncInodeOwnersStopsOnDeleteFailure(t *testing.T) {
	want := errors.New("delete failed")
	updates := 0
	err := syncInodeOwners(map[uint64]uint64{1: 11}, map[uint64]uint64{2: 22}, func(uint64) error { return want }, func(uint64, uint64) error { updates++; return nil })
	if !errors.Is(err, want) || updates != 0 {
		t.Fatalf("err=%v updates=%d", err, updates)
	}
}

func TestSyncInodeOwnersReplacesFullMap(t *testing.T) {
	entries := map[uint64]uint64{1: 11, 2: 22}
	desired := map[uint64]uint64{2: 22, 3: 33}
	err := syncInodeOwners(entries, desired, func(k uint64) error { delete(entries, k); return nil }, func(k, v uint64) error {
		if _, ok := entries[k]; !ok && len(entries) >= 2 {
			return fmt.Errorf("map full")
		}
		entries[k] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[2] != 22 || entries[3] != 33 {
		t.Fatalf("unexpected entries: %v", entries)
	}
}
