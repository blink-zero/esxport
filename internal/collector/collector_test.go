package collector

import (
	"testing"
	"time"

	"github.com/vmware/govmomi/vim25/types"
)

func TestMatchesFilterIncludeOnly(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		include string
		exclude string
		want    bool
	}{
		{"match all", "anything", ".*", "", true},
		{"match prefix", "prod-web-01", "prod-.*", "", true},
		{"no match", "dev-web-01", "prod-.*", "", false},
		{"empty include matches all", "anything", "", "", true},
		{"exclude match", "template-base", ".*", "template-.*", false},
		{"exclude no match", "prod-web-01", ".*", "template-.*", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesFilter(tc.value, tc.include, tc.exclude)
			if got != tc.want {
				t.Errorf("matchesFilter(%q, %q, %q) = %v, want %v",
					tc.value, tc.include, tc.exclude, got, tc.want)
			}
		})
	}
}

func TestToolsStatusToFloat(t *testing.T) {
	tests := []struct {
		status string
		want   float64
	}{
		{"toolsNotInstalled", 0},
		{"toolsNotRunning", 1},
		{"toolsOk", 2},
		{"toolsOld", 3},
		{"unknown", -1},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			got := toolsStatusToFloat(tc.status)
			if got != tc.want {
				t.Errorf("toolsStatusToFloat(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestCountAndOldestSnapshot(t *testing.T) {
	now := time.Now()
	old := now.Add(-24 * time.Hour)
	older := now.Add(-48 * time.Hour)

	t.Run("empty list", func(t *testing.T) {
		count, oldest := countAndOldestSnapshot(nil)
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
		if !oldest.IsZero() {
			t.Errorf("expected zero time, got %v", oldest)
		}
	})

	t.Run("flat snapshots", func(t *testing.T) {
		snaps := []types.VirtualMachineSnapshotTree{
			{CreateTime: now},
			{CreateTime: old},
		}
		count, oldest := countAndOldestSnapshot(snaps)
		if count != 2 {
			t.Errorf("expected 2, got %d", count)
		}
		if !oldest.Equal(old) {
			t.Errorf("expected oldest %v, got %v", old, oldest)
		}
	})

	t.Run("nested snapshots", func(t *testing.T) {
		snaps := []types.VirtualMachineSnapshotTree{
			{
				CreateTime: now,
				ChildSnapshotList: []types.VirtualMachineSnapshotTree{
					{CreateTime: older},
				},
			},
			{CreateTime: old},
		}
		count, oldest := countAndOldestSnapshot(snaps)
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
		if !oldest.Equal(older) {
			t.Errorf("expected oldest %v, got %v", older, oldest)
		}
	})
}
