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
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
)

// TrustedProxies says whose forwarding header the client address of a
// request may be taken from.
//
// A forwarding header is written by whoever sends the request, so it can
// only be believed when the peer of the connection is a proxy we run, and
// then only the part that proxy added. The client address is therefore found
// by walking the header from the right, past every trusted proxy, to the
// first address that is not one.
type TrustedProxies struct {
	prefixes []netip.Prefix
	header   string // canonical header key
}

// Forwarding headers that a trusted proxy may set.
const (
	ProxyHeaderXForwardedFor = "x-forwarded-for"
	ProxyHeaderXRealIP       = "x-real-ip"
	ProxyHeaderForwarded     = "forwarded" // RFC 7239
)

// TrustNoProxy is the list that names no proxy. An empty list does too,
// but an empty environment variable counts as unset in MinIO, so the
// environment needs a word for it.
const TrustNoProxy = "none"

// ParseTrustedProxies parses a comma separated list of IP addresses and
// CIDR prefixes, and the name of the header those proxies set. An empty
// list, or TrustNoProxy, names no proxy. Loopback and the cluster nodes are
// trusted whatever the list says, see trusts.
func ParseTrustedProxies(list, header string) (TrustedProxies, error) {
	var t TrustedProxies
	// Header names are case-insensitive (RFC 9110), and "none" is read
	// the same way, so a value copied from a proxy config works.
	if strings.EqualFold(strings.TrimSpace(list), TrustNoProxy) {
		list = ""
	}
	switch name := strings.ToLower(strings.TrimSpace(header)); name {
	case ProxyHeaderXForwardedFor, ProxyHeaderXRealIP, ProxyHeaderForwarded:
		t.header = http.CanonicalHeaderKey(name)
	default:
		return t, fmt.Errorf("unsupported proxy header %q, expecting %q, %q or %q",
			header, ProxyHeaderXForwardedFor, ProxyHeaderXRealIP, ProxyHeaderForwarded)
	}
	for item := range strings.SplitSeq(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			addr, aerr := netip.ParseAddr(item)
			if aerr != nil {
				return t, fmt.Errorf("invalid trusted proxy %q: expecting an IP address or a CIDR prefix", item)
			}
			addr = addr.Unmap()
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		// Request addresses are compared unmapped, as IPv4, so an
		// IPv4-mapped prefix must become the IPv4 prefix it stands for, or
		// it would never match. Shorter than /96 it is not one.
		if a := prefix.Addr(); a.Is4In6() {
			if prefix.Bits() < 96 {
				return t, fmt.Errorf("invalid trusted proxy %q: an IPv4-mapped prefix must be /96 or longer", item)
			}
			prefix = netip.PrefixFrom(a.Unmap(), prefix.Bits()-96)
		}
		t.prefixes = append(t.prefixes, prefix.Masked())
	}
	return t, nil
}

var trustedProxies atomic.Pointer[TrustedProxies]

func init() {
	t, err := ParseTrustedProxies("", ProxyHeaderXForwardedFor)
	if err != nil {
		panic(err)
	}
	trustedProxies.Store(&t)
}

// SetTrustedProxies replaces the trusted proxies used by GetSourceIP.
func SetTrustedProxies(t TrustedProxies) {
	trustedProxies.Store(&t)
}

// clusterNodes holds the addresses of the nodes of this deployment.
var clusterNodes atomic.Pointer[map[netip.Addr]struct{}]

// SetClusterNodes replaces the addresses of the nodes of this deployment.
// The nodes forward requests to each other, so they are trusted proxies
// whatever the list says.
func SetClusterNodes(addrs []netip.Addr) {
	nodes := make(map[netip.Addr]struct{}, len(addrs))
	for _, addr := range addrs {
		nodes[addr.Unmap().WithZone("")] = struct{}{}
	}
	clusterNodes.Store(&nodes)
}

// trusts reports whether addr is a trusted proxy. Loopback and the cluster
// nodes always are: the embedded console and the SFTP and FTP servers call
// the S3 API over loopback, and a node forwards requests to another node.
// Each passes the client address in the forwarding headers, and the
// administrator should not have to list them.
func (t *TrustedProxies) trusts(addr netip.Addr) bool {
	if addr.IsLoopback() {
		return true
	}
	if nodes := clusterNodes.Load(); nodes != nil {
		if _, ok := (*nodes)[addr]; ok {
			return true
		}
	}
	for _, p := range t.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// hop is one entry of a forwarding header: the address a proxy received
// the request from and, for Forwarded only, the scheme it was received
// with.
type hop struct {
	addr  string
	proto string
}

// clientAddr returns the address of the client, given the address of the
// connection peer (with or without a port) and the request headers. A peer
// that is not an IP address is returned as is.
func (t *TrustedProxies) clientAddr(peer string, h http.Header) string {
	addr, ok := parseNode(peer)
	if !ok {
		return peer
	}
	if t.trusts(addr) {
		addr, _ = t.walk(addr, h)
	}
	return addr.String()
}

// clientScheme returns the scheme the client used, "http" or "https", as a
// trusted peer reports it, or "" when the peer is not trusted or reports
// nothing valid.
//
// With Forwarded it is the proto parameter of the element the client
// address was found in. That element was written by the proxy the client
// connected to. Otherwise it is X-Forwarded-Proto, or X-Forwarded-Scheme,
// as the nearest proxy set it: the last value, since an earlier one may
// come from the client.
func (t *TrustedProxies) clientScheme(peer string, h http.Header) string {
	addr, ok := parseNode(peer)
	if !ok || !t.trusts(addr) {
		return ""
	}
	var proto string
	if t.header == forwarded {
		if _, client := t.walk(addr, h); client != nil {
			proto = client.proto
		}
	} else {
		proto = lastValue(h, xForwardedProto)
		if proto == "" {
			proto = lastValue(h, xForwardedScheme)
		}
	}
	switch proto = strings.ToLower(proto); proto {
	case "http", "https":
		return proto
	}
	return ""
}

// walk follows the forwarding header of a request from the trusted peer,
// from the right, past every trusted proxy. It returns the client address
// and the hop it was found in, or nil when it is the peer itself.
func (t *TrustedProxies) walk(peer netip.Addr, h http.Header) (netip.Addr, *hop) {
	addr := peer
	var client *hop
	hops := forwardingHops(h, t.header)
	for i := len(hops) - 1; i >= 0; i-- {
		next, ok := parseNode(hops[i].addr)
		if !ok {
			// Not an address ("unknown", a hostname, garbage): the
			// trusted proxy that wrote it is the last address we know.
			break
		}
		addr, client = next, &hops[i]
		if !t.trusts(addr) {
			break
		}
	}
	return addr, client
}

// forwardingHops returns the entries of header, the client first and the
// nearest proxy last. A header may be sent as several fields; they are one
// list in the order received.
func forwardingHops(h http.Header, header string) []hop {
	var hops []hop
	for _, field := range h.Values(header) {
		switch header {
		case xRealIP:
			// One address, replaced rather than appended by each proxy.
			hops = append(hops[:0], hop{addr: strings.TrimSpace(field)})
		case forwarded:
			for _, element := range splitUnquoted(field, ',') {
				hops = append(hops, forwardedElement(element))
			}
		default:
			for addr := range strings.SplitSeq(field, ",") {
				hops = append(hops, hop{addr: strings.TrimSpace(addr)})
			}
		}
	}
	return hops
}

// lastValue returns the last comma separated value of header.
func lastValue(h http.Header, header string) string {
	values := h.Values(header)
	if len(values) == 0 {
		return ""
	}
	last := values[len(values)-1]
	return strings.TrimSpace(last[strings.LastIndexByte(last, ',')+1:])
}

// forwardedElement returns the "for" and "proto" parameters of one element
// of a Forwarded header (RFC 7239, section 4), unquoted. A parameter that
// is missing, or given twice, yields "": not an address, not a scheme.
func forwardedElement(element string) hop {
	var h hop
	var seenFor, seenProto bool
	for _, pair := range splitUnquoted(element, ';') {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		value = unquote(strings.TrimSpace(value))
		switch name = strings.TrimSpace(name); {
		case strings.EqualFold(name, "for"):
			h.addr = onlyOnce(&seenFor, value)
		case strings.EqualFold(name, "proto"):
			h.proto = onlyOnce(&seenProto, value)
		}
	}
	return h
}

// onlyOnce returns value the first time and "" every later time, so that a
// parameter given twice in one element counts as missing.
func onlyOnce(seen *bool, value string) string {
	if *seen {
		return ""
	}
	*seen = true
	return value
}

// splitUnquoted splits s at every sep that is not inside a quoted-string.
func splitUnquoted(s string, sep byte) []string {
	var parts []string
	start, quoted, escaped := 0, false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case escaped:
			escaped = false
		case quoted && c == '\\':
			escaped = true
		case c == '"':
			quoted = !quoted
		case !quoted && c == sep:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// unquote returns the content of a quoted-string, or s if it is not one.
func unquote(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	s = s[1 : len(s)-1]
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++ // quoted-pair: the next character stands for itself
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// SetForwardingHeaders sets every header a trusted proxy may be configured
// to use to the client address addr, replacing what the request carried.
// It is for MinIO's own clients of the S3 API on loopback, which must pass
// on their client whichever header the deployment trusts.
func SetForwardingHeaders(h http.Header, addr string) {
	h.Set(xForwardedFor, addr)
	h.Set(xRealIP, addr)
	h.Set(forwarded, forwardedForParam(addr))
}

// forwardedForParam returns the "for" parameter of a Forwarded element for
// an IP address; IPv6 is bracketed and quoted (RFC 7239, section 6).
func forwardedForParam(addr string) string {
	if strings.ContainsRune(addr, ':') {
		return `for="[` + addr + `]"`
	}
	return "for=" + addr
}

// parseNode parses an address as proxies write it: an IP address, with an
// optional port, IPv6 optionally in brackets. IPv4-mapped IPv6 addresses
// are returned as IPv4. Brackets come in pairs and only around IPv6; a
// port is a decimal number, or an obfuscated port of RFC 7239 ("_abc").
func parseNode(s string) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Unmap().WithZone(""), true
	}
	bracketed := strings.HasPrefix(s, "[")
	if host, port, err := net.SplitHostPort(s); err == nil {
		if !isPort(port) {
			return netip.Addr{}, false
		}
		s = host
	} else if bracketed && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	} else {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || bracketed != addr.Is6() {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

// isPort reports whether s is a port as RFC 7239 writes it (node-port): a
// decimal number up to 65535, or "_" and an obfuscated identifier.
func isPort(s string) bool {
	if obf, ok := strings.CutPrefix(s, "_"); ok {
		if obf == "" {
			return false
		}
		for _, c := range obf {
			switch {
			case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '.', c == '_', c == '-':
			default:
				return false
			}
		}
		return true
	}
	if s == "" || len(s) > 5 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(s)
	return err == nil && n <= 65535
}
