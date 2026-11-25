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
	"errors"
	"fmt"
	"net/netip"
	"testing"
)

func TestResolveClusterNodes(t *testing.T) {
	dns := map[string][]string{
		"node1": {"10.0.0.1"},
		"node2": {"10.0.0.2", "2001:db8::2", "not-an-address"},
	}
	lookup := func(_ context.Context, host string) ([]string, error) {
		if addrs, ok := dns[host]; ok {
			return addrs, nil
		}
		return nil, errors.New("no such host")
	}
	hosts := []string{"node1", "node2", "10.0.0.9", "node3"}
	known := map[string][]netip.Addr{}

	got := fmt.Sprint(resolveClusterNodes(context.Background(), hosts, lookup, known))
	if want := "[10.0.0.1 10.0.0.2 2001:db8::2 10.0.0.9]"; got != want {
		t.Fatalf("first resolution: got %s, want %s", got, want)
	}

	// node1 comes back on a new address; node2 does not resolve for now
	// and keeps the addresses it had; node3 appears.
	dns["node1"] = []string{"10.0.0.11"}
	delete(dns, "node2")
	dns["node3"] = []string{"10.0.0.3"}
	got = fmt.Sprint(resolveClusterNodes(context.Background(), hosts, lookup, known))
	if want := "[10.0.0.11 10.0.0.2 2001:db8::2 10.0.0.9 10.0.0.3]"; got != want {
		t.Fatalf("second resolution: got %s, want %s", got, want)
	}
}
