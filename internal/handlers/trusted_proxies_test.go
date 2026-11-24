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

	if tp := mustTrustedProxies(t, "", ProxyHeaderXForwardedFor); len(tp.prefixes) != 0 {
		t.Errorf("empty list: got %v", tp.prefixes)
	}
	if tp := mustTrustedProxies(t, DefaultTrustedProxies, ProxyHeaderXForwardedFor); len(tp.prefixes) != 2 {
		t.Errorf("default list: got %v", tp.prefixes)
	}

	for _, bad := range []struct{ list, header string }{
		{"10.0.0.0/33", ProxyHeaderXForwardedFor},
		{"proxy.example.com", ProxyHeaderXForwardedFor},
		{"10.0.0.1", "forwarded"},
		{"10.0.0.1", "X-Forwarded-For"},
		{"10.0.0.1", ""},
	} {
		if _, err := ParseTrustedProxies(bad.list, bad.header); err == nil {
			t.Errorf("ParseTrustedProxies(%q, %q): expected an error", bad.list, bad.header)
		}
	}
}

func TestGetSourceIP(t *testing.T) {
	xff := mustTrustedProxies(t, "10.0.0.0/8, 2001:db8::/32", ProxyHeaderXForwardedFor)
	realIP := mustTrustedProxies(t, "10.0.0.0/8", ProxyHeaderXRealIP)
	none := mustTrustedProxies(t, "", ProxyHeaderXForwardedFor)

	tests := []struct {
		name    string
		proxies TrustedProxies
		peer    string
		header  http.Header
		want    string
	}{
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

// The default trusts loopback, where the embedded console and the SFTP and
// FTP servers call the S3 API from, and nothing else.
func TestDefaultTrustedProxies(t *testing.T) {
	withTrustedProxies(t, mustTrustedProxies(t, DefaultTrustedProxies, ProxyHeaderXForwardedFor))
	h := http.Header{xForwardedFor: {"192.0.2.1"}}
	for peer, want := range map[string]string{
		"127.0.0.1:5000":  "192.0.2.1",
		"[::1]:5000":      "192.0.2.1",
		"127.0.0.2:5000":  "127.0.0.2",
		"10.0.0.1:5000":   "10.0.0.1",
		"[fe80::1]:5000":  "fe80::1",
		"192.0.2.99:5000": "192.0.2.99",
	} {
		if got := GetSourceIPRaw(&http.Request{RemoteAddr: peer, Header: h}); got != want {
			t.Errorf("peer %s: got %q, want %q", peer, got, want)
		}
	}
}
