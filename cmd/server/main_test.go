package main

import (
	"testing"
	"time"
)

func TestComputeRecoveryIntervalGrowsAndCaps(t *testing.T) {
	prev := time.Duration(0)
	for i := 0; i < len(recoverySchedule); i++ {
		d := computeRecoveryInterval(i)
		if d <= prev {
			t.Fatalf("interval %d (%v) must grow over %v", i, d, prev)
		}
		prev = d
	}
	if got := computeRecoveryInterval(100); got != 6*time.Hour {
		t.Fatalf("interval must cap at 6h, got %v", got)
	}
	if got := computeRecoveryInterval(-1); got != recoverySchedule[0] {
		t.Fatalf("negative attempt must use first interval, got %v", got)
	}
}

func TestAutoRecoveryEnabled(t *testing.T) {
	t.Setenv("SESSION_AUTO_RECOVERY", "")
	if !autoRecoveryEnabled() {
		t.Fatal("auto recovery must be enabled by default")
	}
	for _, v := range []string{"false", "0", "off", "NO"} {
		t.Setenv("SESSION_AUTO_RECOVERY", v)
		if autoRecoveryEnabled() {
			t.Fatalf("SESSION_AUTO_RECOVERY=%s must disable auto recovery", v)
		}
	}
}
