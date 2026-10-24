package followup

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	due := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		now  time.Time
		want Decision
	}{
		{name: "before review window", now: due.Add(-time.Second), want: Wait},
		{name: "at review window", now: due, want: Review},
		{name: "after review window", now: due.Add(time.Hour), want: Review},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := Asset{AssetID: "asset-7", ReviewAfter: due}
			if got := Decide(tt.now, asset); got != tt.want {
				t.Fatalf("Decide() = %q, want %q", got, tt.want)
			}
		})
	}
}
