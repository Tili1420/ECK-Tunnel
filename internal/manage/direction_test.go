package manage

import (
	"strings"
	"testing"
)

// The direct kinds must dial OUT from Iran. That is the whole point of the
// word: a reverse tunnel has kharej dialling in to Iran, and where that
// inbound connection cannot be made the tunnel never comes up. Direct turns it
// around. If Iran ever ended up listening for the tunnel, the feature would be
// a reverse tunnel with different key names.
func TestDirectMeansIranDialsOut(t *testing.T) {
	iran := directSpec{Side: sideIran, Transport: "tcp", Addr: "203.0.113.9:8443",
		Token: "t", Ports: []string{"443"}}.render()
	kharej := directSpec{Side: sideKharej, Transport: "tcp", Addr: "0.0.0.0:8443",
		Token: "t"}.render()

	// Iran is given the kharej server's real address to reach out to.
	if !strings.Contains(iran, `addr         = "203.0.113.9:8443"`) {
		t.Fatalf("the Iran side was not given the kharej address to dial:\n%s", iran)
	}
	// Kharej is given a bind address, which is what listening looks like.
	if !strings.Contains(kharej, `addr         = "0.0.0.0:8443"`) {
		t.Fatalf("the kharej side is not binding:\n%s", kharej)
	}
	// And Iran must never be handed a bind address for the tunnel.
	if strings.Contains(iran, "0.0.0.0") {
		t.Fatalf("the Iran side looks like it is listening for the tunnel:\n%s", iran)
	}

	// Layer 3 says the same thing in its own words.
	l3iran := l3Spec{Side: sideIran, Carrier: "pck", Encap: "gre",
		Addr: "203.0.113.9:9000", Token: "t", Iface: "bp0",
		LocalIP: "10.10.0.1/30", PeerIP: "10.10.0.2", MTU: 1371}.render()
	l3kharej := l3Spec{Side: sideKharej, Carrier: "pck", Encap: "gre",
		Addr: "0.0.0.0:9000", Token: "t", Iface: "bp0",
		LocalIP: "10.10.0.2/30", PeerIP: "10.10.0.1", MTU: 1371}.render()

	if !strings.Contains(l3iran, `mode         = "dial"`) {
		t.Fatalf("the Iran side of a layer-3 tunnel does not dial:\n%s", l3iran)
	}
	if !strings.Contains(l3kharej, `mode         = "listen"`) {
		t.Fatalf("the kharej side of a layer-3 tunnel does not listen:\n%s", l3kharej)
	}

	// The display predicates have to agree, or the screens describe a tunnel
	// that is not the one running.
	if !DialsOut(Tunnel{Role: "iran"}) {
		t.Error("the Iran side is not reported as the side that dials out")
	}
	if DialsOut(Tunnel{Role: "kharej"}) {
		t.Error("the kharej side is reported as dialling out")
	}
	// And it is the exact mirror of a reverse tunnel, where Iran is the server
	// that waits and kharej is the client that dials in.
	if DialsOut(Tunnel{Role: "server"}) {
		t.Error("a reverse server was reported as dialling out")
	}
	if !DialsOut(Tunnel{Role: "client"}) {
		t.Error("a reverse client was not reported as dialling out")
	}
}

// The layer-3 wizard can now build a reverse tunnel too: kharej dials Iran, so
// Iran is the side that listens. Only who opens the connection changes — Iran
// keeps its .1 tunnel address and still exposes ports. The mode each side
// writes is computed from Side and Reverse together, so the two machines,
// configured separately, cannot end up both dialling or both listening.
func TestL3ReverseFlipsWhoDials(t *testing.T) {
	base := func(side directSide, reverse bool, addr, local, peer string) string {
		return l3Spec{Side: side, Reverse: reverse, Carrier: "quic", Encap: "gre",
			Addr: addr, Token: "t", Iface: "bp0",
			LocalIP: local, PeerIP: peer, MTU: 1371}.render()
	}

	// Reverse: kharej dials Iran's real address, Iran binds.
	iran := base(sideIran, true, "0.0.0.0:9000", "10.10.0.1/30", "10.10.0.2")
	kharej := base(sideKharej, true, "198.51.100.7:9000", "10.10.0.2/30", "10.10.0.1")

	if !strings.Contains(iran, `mode         = "listen"`) {
		t.Fatalf("reverse: the Iran side should listen:\n%s", iran)
	}
	if !strings.Contains(kharej, `mode         = "dial"`) {
		t.Fatalf("reverse: the kharej side should dial:\n%s", kharej)
	}
	// Iran still keeps its .1 address — the direction changed, not the layout.
	if !strings.Contains(iran, `local_ip     = "10.10.0.1/30"`) {
		t.Fatalf("reverse: the Iran side lost its .1 address:\n%s", iran)
	}

	// All four combinations, stated once as a table so the mapping is checked
	// whole rather than by two examples.
	for _, c := range []struct {
		side    directSide
		reverse bool
		want    string
	}{
		{sideIran, false, "dial"}, // direct: Iran dials
		{sideKharej, false, "listen"},
		{sideIran, true, "listen"}, // reverse: Iran listens
		{sideKharej, true, "dial"},
	} {
		got := l3Spec{Side: c.side, Reverse: c.reverse}.mode()
		if got != c.want {
			t.Errorf("side=%v reverse=%v: mode=%q, want %q", c.side, c.reverse, got, c.want)
		}
	}
}
