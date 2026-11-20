package p2p

import "testing"

func TestRelayHostStatus_IdleAndNil(t *testing.T) {
	var none *RelayHost
	if st := none.Status(); st.Running {
		t.Fatal("a missing relay host reported itself running")
	}
	h := NewRelayHost(func(string, string) {})
	if st := h.Status(); st.Running || st.Port != 0 || st.RouterOpened {
		t.Fatalf("an idle host: %+v, want nothing running", st)
	}
}
