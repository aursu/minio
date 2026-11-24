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
	"net/netip"
	"sync"
	"time"
)

// LoginLimits are the limits on password guessing for one account from
// one client address (CVE-2026-33419).
type LoginLimits struct {
	// Backoff is the wait after the first failure before the next try.
	Backoff time.Duration
	// BackoffFactor multiplies the wait after every further failure; 1
	// keeps it at Backoff.
	BackoffFactor float64
	// MaxFailures is the number of failures in a row that locks the
	// account out for the address; 0 turns the limits off.
	MaxFailures int
	// Lockout is how long a lockout lasts. Failures are also forgotten
	// once Lockout has passed after the last wait ended.
	Lockout time.Duration
}

// loginKey is one account, by its normalized DN, at one client address.
type loginKey struct {
	account string
	addr    string
}

type loginState struct {
	failures int       // in a row, since the last success or lockout
	next     time.Time // no attempt before this
}

// loginLimiter enforces LoginLimits. It is safe for concurrent use. Its
// state is in memory and per server: a restart, or a change of the LDAP
// configuration, forgets it.
type loginLimiter struct {
	limits LoginLimits
	now    func() time.Time

	mu        sync.Mutex
	state     map[loginKey]*loginState
	nextSweep int
}

const minSweep = 1024

func newLoginLimiter(limits LoginLimits) *loginLimiter {
	return &loginLimiter{
		limits:    limits,
		now:       time.Now,
		state:     make(map[loginKey]*loginState),
		nextSweep: minSweep,
	}
}

// key returns the key for account at addr. IPv6 addresses count per /64,
// the smallest network a client is usually given, so that a client cannot
// start afresh by changing its address within its own network.
func (l *loginLimiter) key(account, addr string) loginKey {
	if a, err := netip.ParseAddr(addr); err == nil {
		if a = a.Unmap(); a.Is6() {
			addr = netip.PrefixFrom(a, 64).Masked().String()
		} else {
			addr = a.String()
		}
	}
	return loginKey{account: account, addr: addr}
}

// enabled reports whether the limits apply at all.
func (l *loginLimiter) enabled() bool {
	return l != nil && l.limits.MaxFailures > 0
}

// attempt asks whether account may try a password from addr now. If so,
// the wait that would follow a failure is reserved at once, so that
// concurrent attempts are refused instead of all reaching the LDAP server
// before the first failure is counted. Otherwise it returns how long to
// wait.
func (l *loginLimiter) attempt(account, addr string) (ok bool, wait time.Duration) {
	if !l.enabled() {
		return true, 0
	}
	k := l.key(account, addr)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state[k]
	if st != nil && l.expired(st, now) {
		st = nil
	}
	if st != nil && now.Before(st.next) {
		return false, st.next.Sub(now)
	}
	if st == nil {
		st = &loginState{}
		l.state[k] = st
		l.sweepLocked(now)
	}
	st.next = now.Add(l.waitAfter(st.failures + 1))
	return true, 0
}

// failed records a failed attempt. It returns true when the failure
// started a lockout.
func (l *loginLimiter) failed(account, addr string) (lockedOut bool) {
	if !l.enabled() {
		return false
	}
	k := l.key(account, addr)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state[k]
	if st == nil {
		st = &loginState{}
		l.state[k] = st
	}
	st.failures++
	if st.failures >= l.limits.MaxFailures {
		st.failures = 0
		st.next = now.Add(l.limits.Lockout)
		return true
	}
	st.next = now.Add(l.waitAfter(st.failures))
	return false
}

// succeeded forgets the failures of account at addr.
func (l *loginLimiter) succeeded(account, addr string) {
	if !l.enabled() {
		return
	}
	k := l.key(account, addr)
	l.mu.Lock()
	delete(l.state, k)
	l.mu.Unlock()
}

// waitAfter returns the wait after failure number n of a row: Backoff,
// multiplied by BackoffFactor for each failure after the first, never more
// than Lockout. The failure that reaches MaxFailures waits Lockout.
func (l *loginLimiter) waitAfter(n int) time.Duration {
	if n >= l.limits.MaxFailures {
		return l.limits.Lockout
	}
	wait := float64(l.limits.Backoff)
	for i := 1; i < n && wait < float64(l.limits.Lockout); i++ {
		wait *= l.limits.BackoffFactor
	}
	return min(time.Duration(wait), l.limits.Lockout)
}

// expired reports whether st can be forgotten: its wait ended at least
// Lockout ago.
func (l *loginLimiter) expired(st *loginState, now time.Time) bool {
	return !now.Before(st.next.Add(l.limits.Lockout))
}

// sweepLocked drops expired states once the map has doubled since the
// last sweep, so that memory follows the number of live states.
func (l *loginLimiter) sweepLocked(now time.Time) {
	if len(l.state) < l.nextSweep {
		return
	}
	for k, st := range l.state {
		if l.expired(st, now) {
			delete(l.state, k)
		}
	}
	l.nextSweep = max(minSweep, 2*len(l.state))
}
