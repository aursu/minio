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
	"net/http/httptest"
	"net/url"
	"testing"
)

// The forwarder must pass on every forwarding header the way a proxy does:
// a deployment that trusts it may read any of them, and would take what
// the client wrote as the client address.
func TestForwarderHeaders(t *testing.T) {
	withTrustedProxies(t, mustTrustedProxies(t, "", ProxyHeaderXForwardedFor))

	var got http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}

	f := NewForwarder(&Forwarder{RoundTripper: http.DefaultTransport})
	r := httptest.NewRequest(http.MethodGet, "/bucket/object", nil)
	r.RemoteAddr = "192.0.2.1:5000"
	r.Header.Set(xRealIP, "1.1.1.1")
	r.Header.Set(xForwardedFor, "1.1.1.1")
	r.Header.Set(forwarded, "for=1.1.1.1")
	r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
	f.ServeHTTP(httptest.NewRecorder(), r)

	if v := got.Get(xRealIP); v != "192.0.2.1" {
		t.Errorf("X-Real-IP: got %q, want 192.0.2.1", v)
	}
	if v := got.Get(xForwardedFor); v != "1.1.1.1, 192.0.2.1" {
		t.Errorf("X-Forwarded-For: got %q, want the peer appended", v)
	}
	if v := got.Values(forwarded); len(v) != 2 || v[0] != "for=1.1.1.1" || v[1] != "for=192.0.2.1;proto=http" {
		t.Errorf("Forwarded: got %q, want an element for the peer appended", v)
	}
	// What the receiving side resolves, trusting this forwarder.
	withTrustedProxies(t, mustTrustedProxies(t, "10.0.0.1", ProxyHeaderForwarded))
	if a := GetSourceIPRaw(&http.Request{RemoteAddr: "10.0.0.1:9000", Header: got}); a != "192.0.2.1" {
		t.Errorf("resolved through the forwarder: got %q, want 192.0.2.1", a)
	}
}
