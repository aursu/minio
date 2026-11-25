// Copyright (c) 2015-2021 MinIO, Inc.
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

// Originally from https://github.com/gorilla/handlers with following license
// https://raw.githubusercontent.com/gorilla/handlers/master/LICENSE, forked
// and heavily modified for MinIO's internal needs.

package handlers

import (
	"net/http"
	"strings"
)

var (
	// De-facto standard header keys.
	xForwardedFor    = http.CanonicalHeaderKey("X-Forwarded-For")
	xForwardedHost   = http.CanonicalHeaderKey("X-Forwarded-Host")
	xForwardedPort   = http.CanonicalHeaderKey("X-Forwarded-Port")
	xForwardedProto  = http.CanonicalHeaderKey("X-Forwarded-Proto")
	xForwardedScheme = http.CanonicalHeaderKey("X-Forwarded-Scheme")
	xRealIP          = http.CanonicalHeaderKey("X-Real-IP")
)

// RFC 7239 defines the "Forwarded" header to replace the X-Forwarded-*
// headers, e.g. Forwarded: for=192.0.2.60;proto=https;by=203.0.113.43
var forwarded = http.CanonicalHeaderKey("Forwarded")

// GetSourceScheme returns the scheme the client used, "http" or "https",
// when the request came from a trusted proxy that reports it (see
// TrustedProxies), and "" otherwise.
func GetSourceScheme(r *http.Request) string {
	return trustedProxies.Load().clientScheme(r.RemoteAddr, r.Header)
}

// GetSourceIPRaw returns the IP address of the client that sent r, without
// brackets. It is the address of the connection peer, unless the peer is a
// trusted proxy (see TrustedProxies); then it is taken from the forwarding
// header that proxy set.
func GetSourceIPRaw(r *http.Request) string {
	return trustedProxies.Load().clientAddr(r.RemoteAddr, r.Header)
}

// GetSourceIP returns GetSourceIPRaw with an IPv6 address in brackets.
func GetSourceIP(r *http.Request) string {
	addr := GetSourceIPRaw(r)
	if strings.ContainsRune(addr, ':') {
		return "[" + addr + "]"
	}
	return addr
}
