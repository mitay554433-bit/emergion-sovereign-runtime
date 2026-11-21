package fieldapi

import (
	"testing"
	"time"
)

// Status reuses the ledger-derived counts until the ledger changes, but the
// live fields are fresh on every call and a decision shows up at once.
func TestStatusCacheFollowsLedger(t *testing.T) {
	rt, ids := seedAtGOV(t, 3)
	a, err := rt.Status()
	if err != nil || a.AtGOV != 3 {
		t.Fatalf("status: %+v %v", a, err)
	}
	stamp := rt.LedgerStamp()
	time.Sleep(2 * time.Millisecond)
	b, err := rt.Status()
	if err != nil || b.AtGOV != 3 || b.TipHash != a.TipHash || !b.MeasuredAt.After(a.MeasuredAt) {
		t.Fatalf("cached status: %+v %v", b, err)
	}
	if err := rt.Decide(ids[0], "HOLD", "status cache proof"); err != nil {
		t.Fatal(err)
	}
	if rt.LedgerStamp() == stamp {
		t.Fatal("ledger stamp must move when an event is appended")
	}
	c, err := rt.Status()
	if err != nil || c.AtGOV != 2 || c.Held != 1 || c.TipHash == a.TipHash || c.Events != a.Events+1 {
		t.Fatalf("status after decision: %+v %v", c, err)
	}
}
