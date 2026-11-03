package portal

import (
	"testing"
	"time"
)

func TestSyncOptionsValidateRange(t *testing.T) {
	for _, test := range []struct {
		days, until int
		ok          bool
	}{
		{3, 0, true},
		{180, 120, true},
		{365, 0, true},
		{0, 0, false},
		{366, 0, false},
		{60, 60, false},
		{60, 90, false},
		{60, -1, false},
	} {
		err := SyncOptions{Days: test.days, UntilDays: test.until}.ValidateRange()
		if (err == nil) != test.ok {
			t.Errorf("days %d until %d: got %v", test.days, test.until, err)
		}
	}
}

func TestActivityDateFilter(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

	filter := activityDateFilter(now, SyncOptions{Days: 3})
	if filter[">=CREATED"] != "2026-10-31T12:00:00Z" || filter["<CREATED"] != nil {
		t.Fatalf("expected the last 3 days up to now, got %v", filter)
	}

	filter = activityDateFilter(now, SyncOptions{Days: 180, UntilDays: 120})
	if filter[">=CREATED"] != "2026-05-07T12:00:00Z" || filter["<CREATED"] != "2026-07-06T12:00:00Z" {
		t.Fatalf("expected 180 to 120 days ago, got %v", filter)
	}
}
