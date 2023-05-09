package board

import "testing"

func TestBatteryApprox(t *testing.T) {
	for _, tc := range []struct {
		microvolts uint32
		percent    int8
	}{
		{2900_000, 0},
		{3400_000, 0},
		{3500_000, 0},
		{3510_000, 1},
		{3528_000, 2},  // the value is rounded down (this is more like 2.8%)
		{3730_000, 40}, // guess, probably higher
		{3749_999, 49}, // rounded down
		{3750_000, 50},
		{3750_001, 50},
		{4179_999, 99},  // rounded down
		{4180_000, 100}, // exactly at 100%
		{4180_001, 100}, // higher values get rounded down
		{5000_000, 100}, // unlikely high voltage, still 100%
	} {
		percent := lithumBatteryApproximation.approximate(tc.microvolts)
		if percent != tc.percent {
			t.Errorf("for %.3fV, expected %d%% but got %d%%", float64(tc.microvolts)/1e6, tc.percent, percent)
		}
	}
}
