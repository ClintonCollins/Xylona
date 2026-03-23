package actions

import (
	"context"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestWaitForServerOnlineReturnsFalseWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	restarted := waitForServerOnline(ctx, func() (xylona.Status, bool) {
		t.Fatal("status lookup should not be called after cancellation")
		return xylona.Status_UNKNOWN, false
	}, 60, time.Second)
	if restarted {
		t.Fatal("waitForServerOnline() = true, want false after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("waitForServerOnline() took %v after cancellation, want fast exit", elapsed)
	}
}

func TestWaitForServerOnlineReturnsTrueWhenServerComesOnline(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	restarted := waitForServerOnline(ctx, func() (xylona.Status, bool) {
		attempts++
		if attempts < 3 {
			return xylona.Status_OFFLINE, true
		}
		return xylona.Status_ONLINE, true
	}, 5, time.Millisecond)
	if !restarted {
		t.Fatal("waitForServerOnline() = false, want true once server reports ONLINE")
	}
	if attempts != 3 {
		t.Fatalf("status lookup attempts = %d, want 3", attempts)
	}
}
