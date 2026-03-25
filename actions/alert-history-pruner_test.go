package actions

import (
	"errors"
	"testing"
	"time"
)

// fakeHistoryPruner implements historyPruner for testing.
type fakeHistoryPruner struct {
	calls     []time.Time
	returnN   int64
	returnErr error
}

func (f *fakeHistoryPruner) PruneAlertHistory(olderThan time.Time) (int64, error) {
	f.calls = append(f.calls, olderThan)
	return f.returnN, f.returnErr
}

func TestPruneAlertHistoryOnce(t *testing.T) {
	tests := []struct {
		name       string
		returnN    int64
		returnErr  error
		wantCalls  int
		wantErrNil bool
		// cutoffAge is how far in the past the pruner should target (relative to now)
		// We can't check exact time, but we verify it's approximately 90 days ago.
	}{
		{
			name:       "deletes old records successfully",
			returnN:    5,
			returnErr:  nil,
			wantCalls:  1,
			wantErrNil: true,
		},
		{
			name:       "zero rows deleted is not an error",
			returnN:    0,
			returnErr:  nil,
			wantCalls:  1,
			wantErrNil: true,
		},
		{
			name:       "db error is returned",
			returnN:    0,
			returnErr:  errors.New("db failure"),
			wantCalls:  1,
			wantErrNil: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeHistoryPruner{
				returnN:   tc.returnN,
				returnErr: tc.returnErr,
			}

			before := time.Now()
			_, errPrune := pruneAlertHistoryOnce(fake)
			after := time.Now()

			if tc.wantErrNil && errPrune != nil {
				t.Errorf("expected nil error, got %v", errPrune)
			}
			if !tc.wantErrNil && errPrune == nil {
				t.Error("expected non-nil error, got nil")
			}

			if len(fake.calls) != tc.wantCalls {
				t.Errorf("expected %d call(s), got %d", tc.wantCalls, len(fake.calls))
			}

			if len(fake.calls) > 0 {
				// The cutoff passed to PruneAlertHistory should be approximately
				// 90 days before the call time.
				cutoff := fake.calls[0]
				expectedLow := before.AddDate(0, 0, -90)
				expectedHigh := after.AddDate(0, 0, -90)

				if cutoff.Before(expectedLow) || cutoff.After(expectedHigh) {
					t.Errorf(
						"cutoff %v is not within expected range [%v, %v]",
						cutoff, expectedLow, expectedHigh,
					)
				}
			}
		})
	}
}

func TestPruneAlertHistoryOnceCutoffIs90Days(t *testing.T) {
	fake := &fakeHistoryPruner{returnN: 3}
	before := time.Now()
	_, _ = pruneAlertHistoryOnce(fake)
	after := time.Now()

	if len(fake.calls) == 0 {
		t.Fatal("pruner was not called")
	}

	cutoff := fake.calls[0]
	// The cutoff must be between (now-90d) at call-before and (now-90d) at call-after.
	lo := before.AddDate(0, 0, -90)
	hi := after.AddDate(0, 0, -90)
	if cutoff.Before(lo) || cutoff.After(hi) {
		t.Errorf("cutoff %v not in expected 90-day window [%v, %v]", cutoff, lo, hi)
	}
}
