package actions

import (
	"testing"
	"time"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int32
		maxBackoff time.Duration
	}{
		{name: "first retry", retryCount: 0, maxBackoff: 6 * time.Second},
		{name: "second retry", retryCount: 1, maxBackoff: 7 * time.Second},
		{name: "third retry", retryCount: 2, maxBackoff: 9 * time.Second},
		{name: "high retry count caps at max", retryCount: 20, maxBackoff: maxRetryBackoff + 5*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBackoff(tt.retryCount)
			if got < 0 {
				t.Errorf("calculateBackoff(%d) returned negative duration: %v", tt.retryCount, got)
			}
			if got > tt.maxBackoff {
				t.Errorf("calculateBackoff(%d) = %v, exceeds expected max %v", tt.retryCount, got, tt.maxBackoff)
			}
		})
	}
}

func TestCalculateBackoffIncreases(t *testing.T) {
	// Run multiple times to average out jitter.
	sumLow := time.Duration(0)
	sumHigh := time.Duration(0)
	iterations := 100

	for range iterations {
		sumLow += calculateBackoff(0)
		sumHigh += calculateBackoff(5)
	}

	avgLow := sumLow / time.Duration(iterations)
	avgHigh := sumHigh / time.Duration(iterations)

	if avgHigh <= avgLow {
		t.Errorf("Expected higher retry count to produce longer backoff: avg(retry=0)=%v, avg(retry=5)=%v", avgLow, avgHigh)
	}
}

func TestCalculateBackoffCapsAtMax(t *testing.T) {
	for range 50 {
		got := calculateBackoff(100)
		// Should never exceed maxRetryBackoff + max jitter (5s).
		if got > maxRetryBackoff+5*time.Second {
			t.Errorf("calculateBackoff(100) = %v, exceeds max + jitter", got)
		}
	}
}
