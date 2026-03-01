package netutil

import "testing"

func TestRemoteHost(t *testing.T) {
	tcs := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "ipv4 with port", input: "127.0.0.1:2222", expect: "127.0.0.1"},
		{name: "ipv6 with port", input: "[::1]:2222", expect: "::1"},
		{name: "host with port", input: "bbs.example.org:2222", expect: "bbs.example.org"},
		{name: "raw ip", input: "10.0.0.12", expect: "10.0.0.12"},
		{name: "empty", input: "", expect: ""},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemoteHost(tc.input); got != tc.expect {
				t.Fatalf("RemoteHost(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestRemoteOrigin(t *testing.T) {
	tcs := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "loopback v4", input: "127.0.0.1:1234", expect: "loopback"},
		{name: "loopback v6", input: "[::1]:1234", expect: "loopback"},
		{name: "lan private", input: "192.168.1.10:23", expect: "lan"},
		{name: "lan cgnat", input: "100.64.10.2:23", expect: "lan"},
		{name: "wan", input: "8.8.8.8:53", expect: "wan"},
		{name: "hostname", input: "bbs.example.org:2222", expect: "host"},
		{name: "empty", input: "", expect: "unknown"},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemoteOrigin(tc.input); got != tc.expect {
				t.Fatalf("RemoteOrigin(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}
