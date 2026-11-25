// Copyright (c) 2015-2026 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package handlers

import (
	"net/http"
	"net/netip"
	"testing"
)

func mustTrustedProxies(t *testing.T, list, header string) TrustedProxies {
	t.Helper()
	tp, err := ParseTrustedProxies(list, header)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

// withTrustedProxies sets the trusted proxies for one test.
func withTrustedProxies(t *testing.T, tp TrustedProxies) {
	t.Helper()
	prev := trustedProxies.Load()
	SetTrustedProxies(tp)
	t.Cleanup(func() { trustedProxies.Store(prev) })
}

// An IPv4-mapped IPv6 prefix stands for the IPv4 prefix: request addresses
// are compared unmapped, so it must match IPv4 peers and hops.
func TestTrustedProxiesIPv4MappedPrefix(t *testing.T) {
	for list, want := range map[string]string{
		"::ffff:10.0.0.0/104":     "10.0.0.0/8",
		"::ffff:198.51.100.1/128": "198.51.100.1/32",
		"::ffff:0:0/96":           "0.0.0.0/0",
		"::ffff:10.1.2.3/104":     "10.0.0.0/8",
	} {
		tp := mustTrustedProxies(t, list, ProxyHeaderXForwardedFor)
		if len(tp.prefixes) != 1 || tp.prefixes[0].String() != want {
			t.Errorf("%s: got %v, want %s", list, tp.prefixes, want)
		}
	}
	if _, err := ParseTrustedProxies("::ffff:10.0.0.0/80", ProxyHeaderXForwardedFor); err == nil {
		t.Error("::ffff:10.0.0.0/80: expected an error")
	}

	withTrustedProxies(t, mustTrustedProxies(t, "::ffff:10.0.0.0/104", ProxyHeaderXForwardedFor))
	h := http.Header{xForwardedFor: {"192.0.2.1"}}
	for _, peer := range []string{"10.0.0.5:5000", "[::ffff:10.0.0.5]:5000"} {
		if got := GetSourceIPRaw(&http.Request{RemoteAddr: peer, Header: h}); got != "192.0.2.1" {
			t.Errorf("peer %s: got %q, want the trusted proxy's client 192.0.2.1", peer, got)
		}
	}
}

func TestParseTrustedProxies(t *testing.T) {
	tp := mustTrustedProxies(t, " 10.0.0.0/8, 192.0.2.7 ,2001:db8::/32,,::ffff:198.51.100.1, 10.1.2.3/8", "x-real-ip")
	want := []string{"10.0.0.0/8", "192.0.2.7/32", "2001:db8::/32", "198.51.100.1/32", "10.0.0.0/8"}
	if len(tp.prefixes) != len(want) {
		t.Fatalf("got %v, want %v", tp.prefixes, want)
	}
	for i, p := range tp.prefixes {
		if p.String() != want[i] {
			t.Errorf("prefix %d: got %s, want %s", i, p, want[i])
		}
	}
	if tp.header != "X-Real-Ip" {
		t.Errorf("header: got %q", tp.header)
	}

	for _, list := range []string{"", " ", TrustNoProxy, " none "} {
		if tp := mustTrustedProxies(t, list, ProxyHeaderXForwardedFor); len(tp.prefixes) != 0 {
			t.Errorf("list %q: got %v", list, tp.prefixes)
		}
	}

	for _, bad := range []struct{ list, header string }{
		{"10.0.0.0/33", ProxyHeaderXForwardedFor},
		{"proxy.example.com", ProxyHeaderXForwardedFor},
		{"10.0.0.1", "x-client-ip"},
		{"10.0.0.1", ""},
		{"nothing", ProxyHeaderXForwardedFor},
	} {
		if _, err := ParseTrustedProxies(bad.list, bad.header); err == nil {
			t.Errorf("ParseTrustedProxies(%q, %q): expected an error", bad.list, bad.header)
		}
	}
}

// Header names are case-insensitive, and "none" is read the same way, so
// values copied from a proxy config work.
func TestParseTrustedProxiesCase(t *testing.T) {
	for _, c := range []struct{ header, want string }{
		{"X-Forwarded-For", "X-Forwarded-For"},
		{"X-REAL-IP", "X-Real-Ip"},
		{"Forwarded", "Forwarded"},
		{" x-real-ip ", "X-Real-Ip"},
	} {
		if tp := mustTrustedProxies(t, "10.0.0.1", c.header); tp.header != c.want {
			t.Errorf("header %q: got %q, want %q", c.header, tp.header, c.want)
		}
	}
	for _, list := range []string{"NONE", "None", " nOnE "} {
		if tp := mustTrustedProxies(t, list, ProxyHeaderXForwardedFor); len(tp.prefixes) != 0 {
			t.Errorf("list %q: got %v", list, tp.prefixes)
		}
	}
}

func TestParseNode(t *testing.T) {
	for s, want := range map[string]string{
		"192.0.2.1":                "192.0.2.1",
		"192.0.2.1:80":             "192.0.2.1",
		"192.0.2.1:65535":          "192.0.2.1",
		"192.0.2.1:_hidden.port-1": "192.0.2.1",
		"2001:db8::1":              "2001:db8::1",
		"[2001:db8::1]":            "2001:db8::1",
		"[2001:db8::1]:4711":       "2001:db8::1",
		"[2001:db8::1]:_x":         "2001:db8::1",
		"[::ffff:192.0.2.1]":       "192.0.2.1",
		"[fe80::1%eth0]:80":        "fe80::1",
		"::ffff:192.0.2.1":         "192.0.2.1",
		"[2001:db8::1":             "",
		"2001:db8::1]":             "",
		"[192.0.2.1]":              "",
		"[192.0.2.1]:80":           "",
		"192.0.2.1:abc":            "",
		"192.0.2.1:":               "",
		"192.0.2.1:65536":          "",
		"192.0.2.1:123456":         "",
		"192.0.2.1:+80":            "",
		"192.0.2.1:_":              "",
		"192.0.2.1:_a/b":           "",
		"[2001:db8::1]:x":          "",
		"unknown":                  "",
		"":                         "",
		"proxy.example.com:80":     "",
		"[[2001:db8::1]]":          "",
		"192.0.2.1:80:80":          "",
		"[2001:db8::1]:80]":        "",
		"2001:db8::1:80":           "2001:db8::1:80",
	} {
		addr, ok := parseNode(s)
		got := ""
		if ok {
			got = addr.String()
		}
		if got != want {
			t.Errorf("parseNode(%q): got %q, want %q", s, got, want)
		}
	}
}

type sourceIPTest struct {
	name    string
	proxies TrustedProxies
	peer    string
	header  http.Header
	want    string
}

func TestGetSourceIP(t *testing.T) {
	xff := mustTrustedProxies(t, "10.0.0.0/8, 2001:db8::/32", ProxyHeaderXForwardedFor)
	realIP := mustTrustedProxies(t, "10.0.0.0/8", ProxyHeaderXRealIP)
	fwd := mustTrustedProxies(t, "10.0.0.0/8, 2001:db8::/32", ProxyHeaderForwarded)
	none := mustTrustedProxies(t, "", ProxyHeaderXForwardedFor)

	tests := []sourceIPTest{
		{
			"untrusted peer, header ignored", xff, "198.51.100.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1"}, xRealIP: {"192.0.2.2"}},
			"198.51.100.1",
		},
		{
			"no proxy trusted", none, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1"}},
			"10.0.0.1",
		},
		{
			"trusted peer, one hop", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1"}},
			"192.0.2.1",
		},
		{
			"client's own entry is skipped", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"1.1.1.1, 192.0.2.1"}},
			"192.0.2.1",
		},
		{
			"chain of trusted proxies", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"1.1.1.1, 192.0.2.1, 10.0.0.9, 10.0.0.8"}},
			"192.0.2.1",
		},
		{
			"several header fields are one list", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"1.1.1.1, 192.0.2.1", "10.0.0.9"}},
			"192.0.2.1",
		},
		{
			"every hop trusted", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"10.0.0.7, 10.0.0.8"}},
			"10.0.0.7",
		},
		{"trusted peer, no header", xff, "10.0.0.1:5000", http.Header{}, "10.0.0.1"},
		{
			"hop that is not an address", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1, unknown, 10.0.0.9"}},
			"10.0.0.9",
		},
		{
			"empty hop", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1, "}},
			"10.0.0.1",
		},
		{
			"hop with a port", xff, "10.0.0.1:5000",
			http.Header{xForwardedFor: {"192.0.2.1:4711"}},
			"192.0.2.1",
		},
		{
			"IPv6 hop in brackets with a port", xff, "[2001:db8::1]:5000",
			http.Header{xForwardedFor: {"[2001:db8:cafe::17]:4711"}},
			"2001:db8:cafe::17",
		},
		{
			"IPv6 hop in brackets", xff, "[2001:db8::1]:5000",
			http.Header{xForwardedFor: {"[2001:db8:cafe::17]"}},
			"2001:db8:cafe::17",
		},
		{
			"IPv4-mapped peer", xff, "[::ffff:10.0.0.1]:5000",
			http.Header{xForwardedFor: {"::ffff:192.0.2.1"}},
			"192.0.2.1",
		},
		{
			"X-Real-IP", realIP, "10.0.0.1:5000",
			http.Header{xRealIP: {"192.0.2.1"}, xForwardedFor: {"1.1.1.1"}},
			"192.0.2.1",
		},
		{
			"X-Real-IP, last field wins", realIP, "10.0.0.1:5000",
			http.Header{xRealIP: {"1.1.1.1", "192.0.2.1"}},
			"192.0.2.1",
		},
		{
			// One address, not a list: a trusted address in it is not
			// walked past, as it would be in X-Forwarded-For.
			"X-Real-IP is not a list", realIP, "10.0.0.1:5000",
			http.Header{xRealIP: {"192.0.2.1", "10.0.0.5"}},
			"10.0.0.5",
		},
		{
			"X-Real-IP header not chosen", xff, "10.0.0.1:5000",
			http.Header{xRealIP: {"192.0.2.1"}},
			"10.0.0.1",
		},
		{
			"peer without a port", xff, "10.0.0.1",
			http.Header{xForwardedFor: {"192.0.2.1"}},
			"192.0.2.1",
		},
		{
			"peer that is not an address", xff, "@",
			http.Header{xForwardedFor: {"192.0.2.1"}},
			"@",
		},
		{
			"no peer", xff, "",
			http.Header{xForwardedFor: {"192.0.2.1"}},
			"",
		},
	}
	// Forwarded (RFC 7239). The examples are those of the RFC, section 7.
	for _, tt := range []struct {
		name, value, want string
	}{
		{"for=", "for=192.0.2.43", "192.0.2.43"},
		{"several elements", "for=192.0.2.43, for=198.51.100.17", "198.51.100.17"},
		{"chain of trusted proxies", "for=192.0.2.43, for=198.51.100.17;by=10.0.0.2, for=10.0.0.3", "198.51.100.17"},
		{"other parameters", "for=192.0.2.60;proto=http;by=203.0.113.43", "192.0.2.60"},
		{"case of the parameter name", "For=192.0.2.60", "192.0.2.60"},
		{"spaces", " for = 192.0.2.60 ; proto=http", "192.0.2.60"},
		{"quoted IPv6 with a port", `for="[2001:db8:cafe::17]:4711"`, "2001:db8:cafe::17"},
		{"quoted IPv4 with a port", `for="192.0.2.43:47011"`, "192.0.2.43"},
		{"quoted-pair", `for="192.0.2.\43"`, "192.0.2.43"},
		{"separators inside a quoted-string", `for="198.51.100.17", proto="a,b;for=1.1.1.1"`, "10.0.0.1"},
		{"comma inside quotes", `for=192.0.2.43, for="[2001:db8::1],x"`, "10.0.0.1"},
		{"comma inside quotes is not an element", `for=192.0.2.43;ext="a,for=10.0.0.9"`, "192.0.2.43"},
		{"semicolon inside quotes is not a pair", `for=192.0.2.43;ext="a;for=10.0.0.9"`, "192.0.2.43"},
		{"escaped quote does not end a quoted-string", `for=192.0.2.43;ext="a\",for=10.0.0.9"`, "192.0.2.43"},
		{"unknown", "for=192.0.2.43, for=unknown", "10.0.0.1"},
		{"obfuscated identifier", `for=192.0.2.43, for="_gazonk"`, "10.0.0.1"},
		{"element without for=", "for=192.0.2.43, proto=https", "10.0.0.1"},
		{"for= twice in one element", "for=192.0.2.43;for=10.0.0.9", "10.0.0.1"},
		{"unterminated quote", `for="192.0.2.43`, "10.0.0.1"},
	} {
		tests = append(tests, sourceIPTest{"Forwarded, " + tt.name, fwd, "10.0.0.1:5000", http.Header{forwarded: {tt.value}}, tt.want})
	}
	tests = append(tests, sourceIPTest{
		"Forwarded header not chosen", xff, "10.0.0.1:5000",
		http.Header{forwarded: {"for=192.0.2.43"}},
		"10.0.0.1",
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTrustedProxies(t, tt.proxies)
			r := &http.Request{RemoteAddr: tt.peer, Header: tt.header}
			if got := GetSourceIPRaw(r); got != tt.want {
				t.Errorf("GetSourceIPRaw: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSourceIPBrackets(t *testing.T) {
	withTrustedProxies(t, mustTrustedProxies(t, "", ProxyHeaderXForwardedFor))
	for peer, want := range map[string]string{
		"192.0.2.1:5000":           "192.0.2.1",
		"[2001:db8:cafe::17]:4711": "[2001:db8:cafe::17]",
	} {
		if got := GetSourceIP(&http.Request{RemoteAddr: peer}); got != want {
			t.Errorf("GetSourceIP(%q): got %q, want %q", peer, got, want)
		}
	}
}

// withClusterNodes sets the cluster node addresses for one test.
func withClusterNodes(t *testing.T, addrs ...string) {
	t.Helper()
	prev := clusterNodes.Load()
	nodes := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		nodes = append(nodes, netip.MustParseAddr(a))
	}
	SetClusterNodes(nodes)
	t.Cleanup(func() { clusterNodes.Store(prev) })
}

// Loopback, where the embedded console and the SFTP and FTP servers call the
// S3 API from, and the cluster nodes, which forward requests to each other,
// are trusted whatever the list says. Nothing else is.
func TestAlwaysTrustedProxies(t *testing.T) {
	withClusterNodes(t, "10.0.0.2", "2001:db8::2", "::ffff:10.0.0.3")
	h := http.Header{xForwardedFor: {"192.0.2.1"}}
	for _, list := range []string{"", "198.51.100.0/24"} {
		withTrustedProxies(t, mustTrustedProxies(t, list, ProxyHeaderXForwardedFor))
		for peer, want := range map[string]string{
			"127.0.0.1:5000":          "192.0.2.1",
			"127.0.0.2:5000":          "192.0.2.1",
			"[::1]:5000":              "192.0.2.1",
			"[::ffff:127.0.0.1]:5000": "192.0.2.1",
			"10.0.0.2:5000":           "192.0.2.1",
			"[2001:db8::2]:5000":      "192.0.2.1",
			"10.0.0.3:5000":           "192.0.2.1",
			"10.0.0.1:5000":           "10.0.0.1",
			"[fe80::1]:5000":          "fe80::1",
			"192.0.2.99:5000":         "192.0.2.99",
		} {
			if got := GetSourceIPRaw(&http.Request{RemoteAddr: peer, Header: h}); got != want {
				t.Errorf("list %q, peer %s: got %q, want %q", list, peer, got, want)
			}
		}
	}

	// A node is trusted only while it is one.
	withClusterNodes(t)
	if got := GetSourceIPRaw(&http.Request{RemoteAddr: "10.0.0.2:5000", Header: h}); got != "10.0.0.2" {
		t.Errorf("former node: got %q", got)
	}
}

// SetForwardingHeaders must let a loopback client pass its client address
// whichever header the deployment trusts.
func TestSetForwardingHeaders(t *testing.T) {
	for _, header := range []string{ProxyHeaderXForwardedFor, ProxyHeaderXRealIP, ProxyHeaderForwarded} {
		withTrustedProxies(t, mustTrustedProxies(t, "", header))
		for _, addr := range []string{"192.0.2.1", "2001:db8:cafe::17"} {
			h := http.Header{
				xForwardedFor: {"1.1.1.1"},
				xRealIP:       {"1.1.1.1"},
				forwarded:     {"for=1.1.1.1"},
			}
			SetForwardingHeaders(h, addr)
			if want := map[string]string{"192.0.2.1": "for=192.0.2.1", "2001:db8:cafe::17": `for="[2001:db8:cafe::17]"`}[addr]; h.Get(forwarded) != want {
				t.Errorf("Forwarded: got %q, want %q (RFC 7239, section 6)", h.Get(forwarded), want)
			}
			if got := GetSourceIPRaw(&http.Request{RemoteAddr: "127.0.0.1:5000", Header: h}); got != addr {
				t.Errorf("%s, %s: got %q", header, addr, got)
			}
		}
	}
}
