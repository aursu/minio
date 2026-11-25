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

package api

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/minio/minio/internal/config"
	"github.com/minio/minio/internal/handlers"
)

// apiKVS returns the api defaults with the given keys set, as the server
// config looks after it is merged with DefaultKVS.
func apiKVS(set map[string]string) config.KVS {
	kvs := slices.Clone(DefaultKVS)
	for k, v := range set {
		kvs.Set(k, v)
	}
	return kvs
}

// clientOf returns the client address cfg finds for a request from peer
// with X-Forwarded-For: 192.0.2.1.
func clientOf(t *testing.T, cfg Config, peer string) string {
	t.Helper()
	handlers.SetTrustedProxies(cfg.TrustedProxies)
	t.Cleanup(func() {
		tp, _ := handlers.ParseTrustedProxies(handlers.DefaultTrustedProxies, handlers.ProxyHeaderXForwardedFor)
		handlers.SetTrustedProxies(tp)
	})
	r := &http.Request{RemoteAddr: peer + ":5000", Header: http.Header{"X-Forwarded-For": {"192.0.2.1"}}}
	return handlers.GetSourceIPRaw(r)
}

func TestLookupConfigTrustedProxies(t *testing.T) {
	t.Run("default trusts loopback", func(t *testing.T) {
		cfg, err := LookupConfig(apiKVS(nil))
		if err != nil {
			t.Fatal(err)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "192.0.2.1" {
			t.Errorf("loopback peer: got %s", got)
		}
		if got := clientOf(t, cfg, "10.0.0.1"); got != "10.0.0.1" {
			t.Errorf("other peer: got %s", got)
		}
	})

	t.Run("configured list", func(t *testing.T) {
		cfg, err := LookupConfig(apiKVS(map[string]string{apiTrustedProxies: "10.0.0.0/8"}))
		if err != nil {
			t.Fatal(err)
		}
		if got := clientOf(t, cfg, "10.0.0.1"); got != "192.0.2.1" {
			t.Errorf("listed peer: got %s", got)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
			t.Errorf("loopback no longer listed: got %s", got)
		}
	})

	t.Run("empty list trusts no proxy", func(t *testing.T) {
		cfg, err := LookupConfig(apiKVS(map[string]string{apiTrustedProxies: ""}))
		if err != nil {
			t.Fatal(err)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
			t.Errorf("got %s", got)
		}
	})

	// An empty environment variable counts as unset, so "none" is the way
	// to trust no proxy from the environment.
	t.Run("environment none trusts no proxy", func(t *testing.T) {
		t.Setenv(EnvAPITrustedProxies, "")
		cfg, err := LookupConfig(apiKVS(nil))
		if err != nil {
			t.Fatal(err)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "192.0.2.1" {
			t.Errorf("empty variable: got %s, want the default list applied", got)
		}
		t.Setenv(EnvAPITrustedProxies, "none")
		if cfg, err = LookupConfig(apiKVS(nil)); err != nil {
			t.Fatal(err)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
			t.Errorf("none: got %s", got)
		}
	})

	t.Run("environment overrides", func(t *testing.T) {
		t.Setenv(EnvAPITrustedProxies, "10.0.0.0/8")
		t.Setenv(EnvAPITrustedProxyHeader, "x-real-ip")
		cfg, err := LookupConfig(apiKVS(nil))
		if err != nil {
			t.Fatal(err)
		}
		// The configured header is X-Real-IP, which the request does not carry.
		if got := clientOf(t, cfg, "10.0.0.1"); got != "10.0.0.1" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("invalid values fail and trust no proxy", func(t *testing.T) {
		for k, v := range map[string]string{
			apiTrustedProxies:     "proxy.example.com",
			apiTrustedProxyHeader: "forwarded",
		} {
			cfg, err := LookupConfig(apiKVS(map[string]string{k: v}))
			if err == nil {
				t.Errorf("%s=%s: expected an error", k, v)
			}
			if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
				t.Errorf("%s=%s: got %s", k, v, got)
			}
		}
	})
}

// The server applies the config even when LookupConfig fails. An invalid
// trusted proxy setting must therefore not reset the other api keys: CORS
// would open to "*", the remote transport would lose its deadline, and
// requests_max and sync_events would fall back to their defaults.
func TestLookupConfigInvalidTrustedProxiesKeepsOthers(t *testing.T) {
	// The environment overrides the config, and make test sets
	// MINIO_API_REQUESTS_MAX. An empty variable counts as unset.
	for _, env := range []string{
		EnvAPICorsAllowOrigin, EnvAPIRequestsMax, EnvAPIRemoteTransportDeadline,
		EnvAPISyncEvents, EnvAPIObjectMaxVersions,
	} {
		t.Setenv(env, "")
	}
	others := map[string]string{
		apiCorsAllowOrigin:         "https://console.example.com",
		apiRequestsMax:             "100",
		apiRemoteTransportDeadline: "30m",
		apiSyncEvents:              config.EnableOn,
		apiObjectMaxVersions:       "5",
	}
	check := func(t *testing.T, cfg Config, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !slices.Equal(cfg.CorsAllowOrigin, []string{"https://console.example.com"}) ||
			cfg.RequestsMax != 100 || cfg.RemoteTransportDeadline != 30*time.Minute ||
			!cfg.SyncEvents || cfg.ObjectMaxVersions != 5 {
			t.Errorf("other keys lost: cors %v, requests_max %d, remote_transport_deadline %v, sync_events %v, object_max_versions %d",
				cfg.CorsAllowOrigin, cfg.RequestsMax, cfg.RemoteTransportDeadline, cfg.SyncEvents, cfg.ObjectMaxVersions)
		}
		if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
			t.Errorf("an invalid setting trusts a proxy: got %s", got)
		}
	}

	for key, value := range map[string]string{apiTrustedProxies: "proxy.example.com", apiTrustedProxyHeader: "x-client-ip"} {
		t.Run("config "+key, func(t *testing.T) {
			kvs := apiKVS(others)
			kvs.Set(key, value)
			cfg, err := LookupConfig(kvs)
			check(t, cfg, err)
		})
	}
	// mc admin config set validates with the environment off, so an
	// invalid environment variable is found only when the server applies
	// the config.
	for env, value := range map[string]string{EnvAPITrustedProxies: "proxy.example.com", EnvAPITrustedProxyHeader: "x-client-ip"} {
		t.Run("environment "+env, func(t *testing.T) {
			t.Setenv(env, value)
			cfg, err := LookupConfig(apiKVS(others))
			check(t, cfg, err)
		})
	}
}

// A failure in another api key returns before the trusted proxies are
// parsed: the applied config then trusts no proxy.
func TestLookupConfigEarlierErrorTrustsNoProxy(t *testing.T) {
	cfg, err := LookupConfig(apiKVS(map[string]string{apiListQuorum: "bogus"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := clientOf(t, cfg, "127.0.0.1"); got != "127.0.0.1" {
		t.Errorf("got %s", got)
	}
}
