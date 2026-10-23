//go:build completionperf

package cmd_test

import (
	"testing"
	"time"
)

// Run through make completion-perf: race/coverage instrumentation is not native latency.
func TestCompletion500SubscriptionsIsOfflineAndFast(t *testing.T) {
	if testing.CoverMode() != "" {
		t.Fatal("completion performance acceptance must run without coverage instrumentation")
	}
	for _, elapsed := range checkCompletion500Subscriptions(t) {
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("500-subscription completion took %s (limit 50ms)", elapsed)
		}
	}
}
