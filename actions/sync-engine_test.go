package actions

import (
	"testing"
	"time"
)

func TestNormalizeNodeSyncInterval(t *testing.T) {
	tests := []struct {
		name      string
		seconds   int32
		wantValue time.Duration
	}{
		{name: "uses default for zero", seconds: 0, wantValue: defaultNodeSyncInterval},
		{name: "uses default for negative", seconds: -30, wantValue: defaultNodeSyncInterval},
		{name: "clamps to minimum", seconds: 5, wantValue: minNodeSyncInterval},
		{name: "uses configured interval in range", seconds: 45, wantValue: 45 * time.Second},
		{name: "clamps to maximum", seconds: int32((maxNodeSyncInterval / time.Second) + 1), wantValue: maxNodeSyncInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNodeSyncInterval(tt.seconds)
			if got != tt.wantValue {
				t.Errorf("normalizeNodeSyncInterval(%d) = %v, want %v", tt.seconds, got, tt.wantValue)
			}
		})
	}
}

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
