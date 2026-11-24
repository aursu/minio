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

// defaultLimits are the defaults, without the address limit, which has
// tests of its own.
var defaultLimits = LoginLimits{Backoff: time.Second, BackoffFactor: 2, MaxFailures: 10, Lockout: 15 * time.Minute}

const dn, addr = "uid=dillon,ou=people,dc=min,dc=io", "192.0.2.1"

// fail makes one failed attempt that must be allowed, and returns whether
// it started a lockout.
func fail(t *testing.T, l *loginLimiter, account, addr string) bool {
	t.Helper()
	if ok, wait := l.attempt(account, addr); !ok {
		t.Fatalf("attempt refused, wait %v", wait)
	}
	return l.failed(account, addr) != ""
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
	if len(l.state)+len(l.addresses) != 0 {
		t.Fatalf("disabled limiter keeps %d states", len(l.state)+len(l.addresses))
	}

	var none *loginLimiter
	if ok, _ := none.attempt(dn, addr); !ok || none.failed(dn, addr) != "" {
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

var addressLimits = LoginLimits{
	Backoff: time.Second, BackoffFactor: 2, MaxFailures: 10, Lockout: 15 * time.Minute,
	MaxAccountsPerAddress: 3, AddressLockout: 15 * time.Minute,
}

func account(i int) string { return fmt.Sprintf("uid=user%d,ou=people,dc=min,dc=io", i) }

// Failures on MaxAccountsPerAddress different accounts lock the address out
// for every account, other addresses not.
func TestLoginLimiterAddressLockout(t *testing.T) {
	l, c := testLimiter(addressLimits)
	fail(t, l, account(1), addr)
	fail(t, l, account(1), "192.0.2.2") // another address does not count here
	c.advance(time.Second)
	fail(t, l, account(1), addr) // the same account twice counts once
	fail(t, l, account(2), addr)
	if lockout := func() string { l.attempt(account(3), addr); return l.failed(account(3), addr) }(); lockout == "" {
		t.Fatal("failures on 3 accounts did not lock the address out")
	}
	wantWait(t, l, account(4), addr, 15*time.Minute)
	wantWait(t, l, dn, addr, 15*time.Minute)
	wantWait(t, l, account(4), "192.0.2.2", 0)

	// IPv6: the lockout covers the /64.
	for i := 1; i <= 3; i++ {
		fail(t, l, account(i), fmt.Sprintf("2001:db8:1:2::%d", i))
	}
	wantWait(t, l, account(9), "2001:db8:1:2:ffff::1", 15*time.Minute)

	c.advance(15 * time.Minute)
	wantWait(t, l, account(4), addr, 0)
}

// An account counts against its address for AddressLockout only.
func TestLoginLimiterAddressWindow(t *testing.T) {
	l, c := testLimiter(addressLimits)
	fail(t, l, account(1), addr)
	fail(t, l, account(2), addr)
	c.advance(15 * time.Minute)
	if fail(t, l, account(3), addr) {
		t.Fatal("failures older than the window still counted")
	}
	wantWait(t, l, account(4), addr, 0)
}

// A success takes only its own account off the address: a guesser cannot
// clear its failures with an account of its own.
func TestLoginLimiterAddressSuccess(t *testing.T) {
	l, _ := testLimiter(addressLimits)
	fail(t, l, account(1), addr)
	fail(t, l, account(2), addr)
	l.succeeded(account(1), addr)
	l.succeeded(dn, addr)
	if fail(t, l, account(3), addr) {
		t.Fatal("account 1 still counted after its success")
	}
	if !fail(t, l, account(4), addr) {
		t.Fatal("the success of another account cleared the address")
	}
}

// Each limit works with the other turned off.
func TestLoginLimiterLimitsIndependent(t *testing.T) {
	onlyAddress := addressLimits
	onlyAddress.MaxFailures = 0
	l, _ := testLimiter(onlyAddress)
	for range 20 {
		fail(t, l, dn, addr) // no wait between failures of one account
	}
	fail(t, l, account(1), addr)
	if !fail(t, l, account(2), addr) {
		t.Fatal("address limit did not apply with the account limit off")
	}

	onlyAccount := addressLimits
	onlyAccount.MaxAccountsPerAddress = 0
	l, _ = testLimiter(onlyAccount)
	for i := range 20 {
		fail(t, l, account(i), addr)
	}
	wantWait(t, l, account(99), addr, 0)
	if len(l.addresses) != 0 {
		t.Fatalf("%d address states with the address limit off", len(l.addresses))
	}
}

func TestLoginLimiterAddressSweep(t *testing.T) {
	l, c := testLimiter(addressLimits)
	for i := range minSweep / 2 {
		fail(t, l, dn, fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	c.advance(time.Hour)
	fail(t, l, dn, addr) // two more states; the first sweeps
	if len(l.state) != 1 || len(l.addresses) != 1 {
		t.Fatalf("after the sweep: %d states, %d addresses; want 1 and 1", len(l.state), len(l.addresses))
	}
}
