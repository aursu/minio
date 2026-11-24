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

package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/minio/minio/internal/bpool"
)

const defaultFlushInterval = time.Duration(100) * time.Millisecond

// Forwarder forwards all incoming HTTP requests to configured transport.
type Forwarder struct {
	RoundTripper http.RoundTripper
	PassHost     bool
	Logger       func(error)
	ErrorHandler func(http.ResponseWriter, *http.Request, error)

	// internal variables
	rewriter *headerRewriter
}

// NewForwarder creates an instance of Forwarder based on the provided list of configuration options
func NewForwarder(f *Forwarder) *Forwarder {
	f.rewriter = &headerRewriter{}
	if f.RoundTripper == nil {
		f.RoundTripper = http.DefaultTransport
	}

	return f
}

type bufPool struct {
	sz   int
	pool bpool.Pool[*[]byte]
}

func (b *bufPool) Put(buf []byte) {
	if cap(buf) < b.sz || cap(buf) > b.sz*2 {
		// Buffer too small or will likely leak memory after being expanded.
		// Drop it.
		return
	}
	b.pool.Put(&buf)
}

func (b *bufPool) Get() []byte {
	bufp := b.pool.Get()
	if bufp == nil || cap(*bufp) < b.sz {
		return make([]byte, 0, b.sz)
	}
	return (*bufp)[:b.sz]
}

func newBufPool(sz int) httputil.BufferPool {
	return &bufPool{sz: sz, pool: bpool.Pool[*[]byte]{
		New: func() *[]byte {
			buf := make([]byte, sz)
			return &buf
		},
	}}
}

// ServeHTTP forwards HTTP traffic using the configured transport
func (f *Forwarder) ServeHTTP(w http.ResponseWriter, inReq *http.Request) {
	outReq := new(http.Request)
	*outReq = *inReq // includes shallow copies of maps, but we handle this in Director

	revproxy := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			f.modifyRequest(req, inReq.URL)
		},
		BufferPool:    newBufPool(128 << 10),
		Transport:     f.RoundTripper,
		FlushInterval: defaultFlushInterval,
		ErrorHandler:  f.customErrHandler,
	}

	if f.ErrorHandler != nil {
		revproxy.ErrorHandler = f.ErrorHandler
	}

	revproxy.ServeHTTP(w, outReq)
}

// customErrHandler is originally implemented to avoid having the following error
//
//	`http: proxy error: context canceled` printed by Golang
func (f *Forwarder) customErrHandler(w http.ResponseWriter, r *http.Request, err error) {
	if f.Logger != nil && err != context.Canceled {
		f.Logger(err)
	}
	w.WriteHeader(http.StatusBadGateway)
}

func (f *Forwarder) getURLFromRequest(req *http.Request) *url.URL {
	// If the Request was created by Go via a real HTTP request,  RequestURI will
	// contain the original query string. If the Request was created in code, RequestURI
	// will be empty, and we will use the URL object instead
	u := req.URL
	if req.RequestURI != "" {
		parsedURL, err := url.ParseRequestURI(req.RequestURI)
		if err == nil {
			u = parsedURL
		}
	}
	return u
}

// copyURL provides update safe copy by avoiding shallow copying User field
func copyURL(i *url.URL) *url.URL {
	out := *i
	if i.User != nil {
		u := *i.User
		out.User = &u
	}
	return &out
}

// Modify the request to handle the target URL
func (f *Forwarder) modifyRequest(outReq *http.Request, target *url.URL) {
	outReq.URL = copyURL(outReq.URL)
	outReq.URL.Scheme = target.Scheme
	outReq.URL.Host = target.Host

	u := f.getURLFromRequest(outReq)

	outReq.URL.Path = u.Path
	outReq.URL.RawPath = u.RawPath
	outReq.URL.RawQuery = u.RawQuery
	outReq.RequestURI = "" // Outgoing request should not have RequestURI

	// Do not pass client Host header unless requested.
	if !f.PassHost {
		outReq.Host = target.Host
	}

	// TODO: only supports HTTP 1.1 for now.
	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1

	f.rewriter.Rewrite(outReq)

	// Disable closeNotify when method GET for http pipelining
	if outReq.Method == http.MethodGet {
		quietReq := outReq.WithContext(context.Background())
		*outReq = *quietReq
	}
}

// headerRewriter is responsible for removing hop-by-hop headers and setting forwarding headers
type headerRewriter struct{}

func (rw *headerRewriter) Rewrite(req *http.Request) {
	// A deployment that trusts this one may read any of the three headers,
	// so each is passed on the way a proxy should. httputil.ReverseProxy
	// appends the peer to X-Forwarded-For itself. X-Real-IP is set to the
	// client as resolved here, never passed on from the client. Forwarded
	// gets an element for this hop (RFC 7239, section 4).
	received := "http"
	if req.TLS != nil {
		received = "https"
	}
	req.Header.Set(xRealIP, GetSourceIPRaw(req))
	if peer, ok := parseNode(req.RemoteAddr); ok {
		req.Header.Add(forwarded, forwardedForParam(peer.String())+";proto="+received)
	}

	// X-Forwarded-Proto is the scheme the client used: as a trusted proxy
	// in front of this deployment reported it, or as received here. It is
	// never passed on from the client.
	clientScheme := GetSourceScheme(req)
	if clientScheme == "" {
		clientScheme = received
	}
	req.Header.Set(xForwardedProto, clientScheme)

	if xfPort := req.Header.Get(xForwardedPort); xfPort == "" {
		req.Header.Set(xForwardedPort, forwardedPort(req))
	}

	if xfHost := req.Header.Get(xForwardedHost); xfHost == "" && req.Host != "" {
		req.Header.Set(xForwardedHost, req.Host)
	}
}

func forwardedPort(req *http.Request) string {
	if req == nil {
		return ""
	}

	if _, port, err := net.SplitHostPort(req.Host); err == nil && port != "" {
		return port
	}

	if req.TLS != nil {
		return "443"
	}

	return "80"
}
