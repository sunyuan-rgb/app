package clock

import (
	"testing"
	"time"
)

func TestFixedClock(t *testing.T) {
	want := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	got := Fixed{T: want}.Now()
	if !got.Equal(want) {
		t.Fatalf("Fixed.Now() = %v, want %v", got, want)
	}
}

func TestShiftClock(t *testing.T) {
	before := time.Now()
	got := Shift{Offset: 48 * time.Hour}.Now()
	after := time.Now().Add(48 * time.Hour)
	if got.Before(before.Add(48*time.Hour)) || got.After(after) {
		t.Fatalf("Shift.Now() = %v, expect within [%v, %v]", got, before.Add(48*time.Hour), after)
	}
}

func TestSystemClockNearNow(t *testing.T) {
	now := time.Now()
	got := System{}.Now()
	if got.Sub(now) > time.Second || now.Sub(got) > time.Second {
		t.Fatalf("System.Now() = %v, too far from %v", got, now)
	}
}
