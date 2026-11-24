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

package ldap

import (
	"fmt"
	"testing"
	"time"
)

// clock is a fake time source for the limiter.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func testLimiter(limits LoginLimits) (*loginLimiter, *clock) {
	c := &clock{t: time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)}
	l := newLoginLimiter(limits)
	l.now = c.now
	return l, c
}

var defaultLimits = LoginLimits{Backoff: time.Second, BackoffFactor: 2, MaxFailures: 10, Lockout: 15 * time.Minute}

const dn, addr = "uid=dillon,ou=people,dc=min,dc=io", "192.0.2.1"

// fail makes one failed attempt that must be allowed, and returns whether
// it started a lockout.
func fail(t *testing.T, l *loginLimiter, account, addr string) bool {
	t.Helper()
	if ok, wait := l.attempt(account, addr); !ok {
		t.Fatalf("attempt refused, wait %v", wait)
	}
	return l.failed(account, addr)
}

func wantWait(t *testing.T, l *loginLimiter, account, addr string, want time.Duration) {
	t.Helper()
	ok, wait := l.attempt(account, addr)
	if want == 0 {
		if !ok {
			t.Fatalf("attempt refused, wait %v; want allowed", wait)
		}
		return
	}
	if ok || wait != want {
		t.Fatalf("attempt: ok %v, wait %v; want refused, wait %v", ok, wait, want)
	}
}

func TestLoginLimiterBackoffAndLockout(t *testing.T) {
	l, c := testLimiter(defaultLimits)
	for n := 1; n <= 9; n++ {
		if fail(t, l, dn, addr) {
			t.Fatalf("failure %d started a lockout", n)
		}
		want := time.Second << (n - 1) // 1s, 2s, 4s ... 256s
		wantWait(t, l, dn, addr, want)
		c.advance(want - time.Millisecond)
		wantWait(t, l, dn, addr, time.Millisecond)
		c.advance(time.Millisecond)
	}
	if !fail(t, l, dn, addr) {
		t.Fatal("failure 10 did not start a lockout")
	}
	wantWait(t, l, dn, addr, 15*time.Minute)
	c.advance(15 * time.Minute)

	// After the lockout the count starts again.
	if fail(t, l, dn, addr) {
		t.Fatal("first failure after the lockout started another")
	}
	wantWait(t, l, dn, addr, time.Second)
}

func TestLoginLimiterStaticPeriod(t *testing.T) {
	limits := defaultLimits
	limits.BackoffFactor = 1
	l, c := testLimiter(limits)
	for range 5 {
		fail(t, l, dn, addr)
		wantWait(t, l, dn, addr, time.Second)
		c.advance(time.Second)
	}
}

func TestLoginLimiterWaitCapped(t *testing.T) {
	limits := defaultLimits
	limits.BackoffFactor = 1000
	l, c := testLimiter(limits)
	fail(t, l, dn, addr)
	c.advance(time.Second)
	fail(t, l, dn, addr)
	c.advance(1000 * time.Second)
	fail(t, l, dn, addr)
	wantWait(t, l, dn, addr, 15*time.Minute) // 1e6 s, capped
}

// An attempt reserves the wait at once: a concurrent attempt is refused
// before the first one has a result.
func TestLoginLimiterConcurrentAttempts(t *testing.T) {
	l, _ := testLimiter(defaultLimits)
	wantWait(t, l, dn, addr, 0)
	wantWait(t, l, dn, addr, time.Second)

	// The attempt that would reach MaxFailures reserves the lockout.
	l, c := testLimiter(defaultLimits)
	for n := 1; n <= 9; n++ {
		fail(t, l, dn, addr)
		c.advance(time.Second << (n - 1))
	}
	wantWait(t, l, dn, addr, 0)
	wantWait(t, l, dn, addr, 15*time.Minute)
}

func TestLoginLimiterSuccessForgets(t *testing.T) {
	l, c := testLimiter(defaultLimits)
	for n := 1; n <= 5; n++ {
		fail(t, l, dn, addr)
		c.advance(time.Second << (n - 1))
	}
	wantWait(t, l, dn, addr, 0)
	l.succeeded(dn, addr)
	fail(t, l, dn, addr)
	wantWait(t, l, dn, addr, time.Second)
}

func TestLoginLimiterKeys(t *testing.T) {
	l, _ := testLimiter(defaultLimits)
	fail(t, l, dn, addr)

	// Another address, and another account, are not affected.
	wantWait(t, l, dn, "192.0.2.2", 0)
	wantWait(t, l, "uid=other,dc=min,dc=io", addr, 0)

	// IPv4-mapped IPv6 is the IPv4 address.
	wantWait(t, l, dn, "::ffff:"+addr, time.Second)

	// IPv6 counts per /64.
	fail(t, l, dn, "2001:db8:1:2::1")
	wantWait(t, l, dn, "2001:db8:1:2:ffff::9", time.Second)
	wantWait(t, l, dn, "2001:db8:1:3::1", 0)
}

func TestLoginLimiterForgets(t *testing.T) {
	l, c := testLimiter(defaultLimits)
	for n := 1; n <= 5; n++ {
		fail(t, l, dn, addr)
		c.advance(time.Second << (n - 1))
	}
	// The wait after failure 5 (16s) has ended; once a lockout's length
	// more has passed, the failures are forgotten.
	c.advance(15*time.Minute - time.Millisecond)
	fail(t, l, dn, addr)
	wantWait(t, l, dn, addr, 32*time.Second)

	c.advance(32*time.Second + 15*time.Minute)
	fail(t, l, dn, addr)
	wantWait(t, l, dn, addr, time.Second)
}

func TestLoginLimiterDisabled(t *testing.T) {
	limits := defaultLimits
	limits.MaxFailures = 0
	l, _ := testLimiter(limits)
	for range 100 {
		fail(t, l, dn, addr)
	}
	wantWait(t, l, dn, addr, 0)
	if len(l.state) != 0 {
		t.Fatalf("disabled limiter keeps %d states", len(l.state))
	}

	var none *loginLimiter
	if ok, _ := none.attempt(dn, addr); !ok || none.failed(dn, addr) {
		t.Fatal("a nil limiter must allow everything")
	}
	none.succeeded(dn, addr)
}

func TestLoginLimiterSweep(t *testing.T) {
	l, c := testLimiter(defaultLimits)
	for i := range minSweep {
		fail(t, l, dn, fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	if l.nextSweep != minSweep || len(l.state) != minSweep {
		t.Fatalf("%d states, next sweep at %d", len(l.state), l.nextSweep)
	}
	c.advance(time.Hour)
	fail(t, l, dn, addr) // one more state, which sweeps: every other one has expired
	if len(l.state) != 1 {
		t.Fatalf("%d states after the sweep, want 1", len(l.state))
	}
	if l.nextSweep != minSweep {
		t.Fatalf("next sweep at %d, want %d", l.nextSweep, minSweep)
	}
}

// The attempt that triggers a sweep keeps its reservation: the new state
// must not be swept as expired before its wait is set.
func TestLoginLimiterSweepKeepsNewState(t *testing.T) {
	l, _ := testLimiter(defaultLimits)
	l.nextSweep = 1
	wantWait(t, l, dn, addr, 0) // sweeps
	wantWait(t, l, dn, addr, time.Second)
}
