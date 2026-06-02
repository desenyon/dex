package network

import "testing"

func TestParseLsofPorts(t *testing.T) {
	input := `COMMAND   PID   USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
node    1234  alice  22u  IPv4 0xabc      0t0  TCP 127.0.0.1:3000 (LISTEN)
`

	got := ParseLsofPorts(input)
	if len(got) != 1 {
		t.Fatalf("expected one port, got %#v", got)
	}
	if got[0].Command != "node" || got[0].PID != 1234 || got[0].User != "alice" || got[0].Address != "127.0.0.1:3000" || got[0].State != "LISTEN" {
		t.Fatalf("unexpected port: %#v", got[0])
	}
}

func TestSplitAddress(t *testing.T) {
	ip, network := splitAddress("192.168.1.5/24")
	if ip != "192.168.1.5" || network != "24" {
		t.Fatalf("unexpected split: %q %q", ip, network)
	}
}

func TestParseDNSConfig(t *testing.T) {
	input := `resolver #1
  nameserver[0] : 192.168.4.1
  nameserver[1] : 1.1.1.1
resolver #2
  nameserver[0] : 8.8.8.8
`

	got := ParseDNSConfig(input)
	if len(got) != 3 {
		t.Fatalf("expected three resolvers, got %#v", got)
	}
	if got[0].Resolver != "resolver #1" || got[0].Nameserver != "192.168.4.1" {
		t.Fatalf("unexpected first resolver: %#v", got[0])
	}
	if got[2].Resolver != "resolver #2" || got[2].Nameserver != "8.8.8.8" {
		t.Fatalf("unexpected third resolver: %#v", got[2])
	}
}

func TestParseRoutes(t *testing.T) {
	input := `default            192.168.4.1        UGScg                 en0
127                127.0.0.1          UCS                   lo0
`

	got := ParseRoutes(input)
	if len(got) != 2 {
		t.Fatalf("expected two routes, got %#v", got)
	}
	if got[0].Destination != "default" || got[0].Gateway != "192.168.4.1" || got[0].Interface != "en0" {
		t.Fatalf("unexpected first route: %#v", got[0])
	}
}
