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

package cmd

import (
	"context"
	"net/netip"
	"time"

	"github.com/minio/minio/internal/handlers"
)

// clusterNodesRefresh is how often the node names are resolved again: a
// node that comes back may have a new address.
const clusterNodesRefresh = time.Minute

// trustClusterNodes makes the nodes of a distributed deployment trusted
// proxies, whatever api trusted_proxies says. A node forwards requests to
// another node (a listing continued with a token from that node, pool
// status, batch jobs), and the node it forwards to sees it as the peer.
func trustClusterNodes(ctx context.Context, eps []ProxyEndpoint) {
	if len(eps) == 0 {
		return
	}
	hosts := make([]string, 0, len(eps))
	for _, ep := range eps {
		hosts = append(hosts, ep.Hostname())
	}
	known := make(map[string][]netip.Addr, len(hosts))
	refresh := func() {
		handlers.SetClusterNodes(resolveClusterNodes(ctx, hosts, globalDNSCache.LookupHost, known))
	}
	refresh()
	go func() {
		t := time.NewTicker(clusterNodesRefresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh()
			}
		}
	}()
}

// resolveClusterNodes returns the addresses of hosts, and records them in
// known. A host that does not resolve keeps the addresses known for it.
func resolveClusterNodes(ctx context.Context, hosts []string,
	lookup func(context.Context, string) ([]string, error), known map[string][]netip.Addr,
) []netip.Addr {
	var all []netip.Addr
	for _, host := range hosts {
		if addr, err := netip.ParseAddr(host); err == nil {
			known[host] = []netip.Addr{addr}
		} else if names, err := lookup(ctx, host); err == nil {
			var addrs []netip.Addr
			for _, name := range names {
				if addr, err := netip.ParseAddr(name); err == nil {
					addrs = append(addrs, addr)
				}
			}
			if len(addrs) > 0 {
				known[host] = addrs
			}
		}
		all = append(all, known[host]...)
	}
	return all
}
