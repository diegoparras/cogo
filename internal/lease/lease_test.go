package lease

import (
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestLeaseLifecycle(t *testing.T) {
	s := Open(t.TempDir())
	t0 := mustTime("2026-07-07T10:00:00Z")

	// A acquires "migrate-db" for 5 min.
	if _, err := s.Acquire("migrate-db", "agentA", "running the 0007 migration", 5*time.Minute, t0); err != nil {
		t.Fatalf("A acquire: %v", err)
	}

	// B can't take it while A holds it — and learns who/when.
	if _, err := s.Acquire("migrate-db", "agentB", "", 5*time.Minute, t0.Add(time.Minute)); err == nil {
		t.Fatal("B acquired a lease A still holds")
	} else if err.Error() == "" {
		t.Fatal("conflict error should name the holder")
	}

	// A renews (re-acquire by same holder) — keeps the original acquired time.
	l0, _ := s.Acquire("migrate-db", "agentA", "", 5*time.Minute, t0)
	l1, err := s.Acquire("migrate-db", "agentA", "", 5*time.Minute, t0.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("A renew: %v", err)
	}
	if l1.Acquired != l0.Acquired {
		t.Errorf("renew changed acquired time: %s -> %s", l0.Acquired, l1.Acquired)
	}
	if l1.Expires <= l0.Expires {
		t.Errorf("renew did not push expiry forward")
	}

	// List shows one held lease now.
	if got := s.List(t0.Add(3 * time.Minute)); len(got) != 1 || got[0].Holder != "agentA" {
		t.Fatalf("List = %+v, want [agentA]", got)
	}

	// After expiry, B can take it.
	if _, err := s.Acquire("migrate-db", "agentB", "", time.Minute, t0.Add(10*time.Minute)); err != nil {
		t.Fatalf("B acquire after A expired: %v", err)
	}

	// Release by the wrong holder is a no-op; by the right holder frees it.
	if s.Release("migrate-db", "agentA") {
		t.Error("A released a lease B holds")
	}
	if !s.Release("migrate-db", "agentB") {
		t.Error("B could not release its own lease")
	}
	if got := s.List(t0.Add(11 * time.Minute)); len(got) != 0 {
		t.Fatalf("after release List = %+v, want empty", got)
	}
}
