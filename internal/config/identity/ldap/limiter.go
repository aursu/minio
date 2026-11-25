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
	"net/netip"
	"sync"
	"time"
)

// LoginLimits are the limits on password guessing (CVE-2026-33419).
type LoginLimits struct {
	// Backoff is the wait after the first failure of an account at an
	// address before the next try.
	Backoff time.Duration
	// BackoffFactor multiplies the wait after every further failure; 1
	// keeps it at Backoff.
	BackoffFactor float64
	// MaxFailures is the number of failures in a row that locks the
	// account out for the address; 0 turns this limit off.
	MaxFailures int
	// Lockout is how long such a lockout lasts. Failures are also
	// forgotten once Lockout has passed after the last wait ended.
	Lockout time.Duration

	// MaxAccountsPerAddress is the number of different accounts that may
	// fail from one address within AddressLockout before the address is
	// locked out for every account (password spraying); 0 turns this
	// limit off.
	MaxAccountsPerAddress int
	// AddressLockout is how long an address lockout lasts, and how long a
	// failed account counts against its address.
	AddressLockout time.Duration

	// MaxFailuresPerAccount is the number of failures of one account, from
	// any addresses, within AccountWindow that locks the account out for
	// every address for AccountWindow (many addresses guessing one
	// password); 0 turns this limit off.
	MaxFailuresPerAccount int
	AccountWindow         time.Duration
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

type addressState struct {
	failed      map[string]time.Time // account: its last failure
	lockedUntil time.Time
}

type accountState struct {
	failures    []time.Time // within AccountWindow, oldest first
	lockedUntil time.Time
}

// loginLimiter enforces LoginLimits. It is safe for concurrent use. Its
// state is in memory and per server: a restart, or a change of the LDAP
// configuration, forgets it.
type loginLimiter struct {
	limits LoginLimits
	now    func() time.Time

	mu        sync.Mutex
	state     map[loginKey]*loginState
	addresses map[string]*addressState
	accounts  map[string]*accountState
	nextSweep int
}

const minSweep = 1024

func newLoginLimiter(limits LoginLimits) *loginLimiter {
	return &loginLimiter{
		limits:    limits,
		now:       time.Now,
		state:     make(map[loginKey]*loginState),
		addresses: make(map[string]*addressState),
		accounts:  make(map[string]*accountState),
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

func (l *loginLimiter) pairLimit() bool    { return l != nil && l.limits.MaxFailures > 0 }
func (l *loginLimiter) addressLimit() bool { return l != nil && l.limits.MaxAccountsPerAddress > 0 }
func (l *loginLimiter) accountLimit() bool { return l != nil && l.limits.MaxFailuresPerAccount > 0 }
func (l *loginLimiter) anyLimit() bool     { return l.pairLimit() || l.addressLimit() || l.accountLimit() }

// attempt asks whether account may try a password from addr now. If so,
// the wait that would follow a failure is reserved at once, so that
// concurrent attempts are refused instead of all reaching the LDAP server
// before the first failure is counted. Otherwise it returns how long to
// wait.
func (l *loginLimiter) attempt(account, addr string) (ok bool, wait time.Duration) {
	if !l.anyLimit() {
		return true, 0
	}
	k := l.key(account, addr)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	if as := l.addresses[k.addr]; as != nil && now.Before(as.lockedUntil) {
		return false, as.lockedUntil.Sub(now)
	}
	if ac := l.accounts[account]; ac != nil && now.Before(ac.lockedUntil) {
		return false, ac.lockedUntil.Sub(now)
	}
	if !l.pairLimit() {
		return true, 0
	}
	st := l.state[k]
	if st != nil && l.expired(st, now) {
		st = nil
	}
	if st != nil && now.Before(st.next) {
		return false, st.next.Sub(now)
	}
	if st == nil {
		// Sweep before the new state is in the map: until its wait is
		// set below, it would look expired and be swept.
		l.sweepLocked(now)
		st = &loginState{}
		l.state[k] = st
	}
	st.next = now.Add(l.waitAfter(st.failures + 1))
	return true, 0
}

// failed records a failed attempt. When the failure starts a lockout, it
// returns a description of it, for the server log.
func (l *loginLimiter) failed(account, addr string) (lockout string) {
	if !l.anyLimit() {
		return ""
	}
	k := l.key(account, addr)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pairLimit() {
		st := l.state[k]
		if st == nil {
			st = &loginState{}
			l.state[k] = st
		}
		st.failures++
		if st.failures >= l.limits.MaxFailures {
			st.failures = 0
			st.next = now.Add(l.limits.Lockout)
			lockout = fmt.Sprintf("%s from %s locked out for %v after %d failures",
				account, k.addr, l.limits.Lockout, l.limits.MaxFailures)
		} else {
			st.next = now.Add(l.waitAfter(st.failures))
		}
	}
	if l.addressLimit() {
		as := l.addresses[k.addr]
		if as == nil {
			l.sweepLocked(now) // before the new, still empty state is in the map
			as = &addressState{failed: make(map[string]time.Time)}
			l.addresses[k.addr] = as
		}
		l.forgetOldFailures(as, now)
		as.failed[account] = now
		if len(as.failed) >= l.limits.MaxAccountsPerAddress {
			as.lockedUntil = now.Add(l.limits.AddressLockout)
			clear(as.failed)
			if lockout != "" {
				lockout += "; "
			}
			lockout += fmt.Sprintf("address %s locked out for %v after failures on %d accounts",
				k.addr, l.limits.AddressLockout, l.limits.MaxAccountsPerAddress)
		}
	}
	if l.accountLimit() {
		ac := l.accounts[account]
		if ac == nil {
			l.sweepLocked(now) // before the new, still empty state is in the map
			ac = &accountState{}
			l.accounts[account] = ac
		}
		l.forgetOldAccountFailures(ac, now)
		ac.failures = append(ac.failures, now)
		if len(ac.failures) >= l.limits.MaxFailuresPerAccount {
			ac.lockedUntil = now.Add(l.limits.AccountWindow)
			ac.failures = nil
			if lockout != "" {
				lockout += "; "
			}
			lockout += fmt.Sprintf("%s locked out from every address for %v after %d failures within %v",
				account, l.limits.AccountWindow, l.limits.MaxFailuresPerAccount, l.limits.AccountWindow)
		}
	}
	return lockout
}

// succeeded forgets the failures of account at addr. The account no longer
// counts against the address either; the other accounts that failed there
// still do, so that a guesser cannot clear them with an account of its own.
// The failures of the account from other addresses still count too: a
// success of the owner during an attack would otherwise give the attackers
// a fresh count.
func (l *loginLimiter) succeeded(account, addr string) {
	if !l.anyLimit() {
		return
	}
	k := l.key(account, addr)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, k)
	if as := l.addresses[k.addr]; as != nil {
		delete(as.failed, account)
	}
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

// forgetOldFailures drops the accounts that failed at an address longer
// than AddressLockout ago.
func (l *loginLimiter) forgetOldFailures(as *addressState, now time.Time) {
	for account, at := range as.failed {
		if !now.Before(at.Add(l.limits.AddressLockout)) {
			delete(as.failed, account)
		}
	}
}

// forgetOldAccountFailures drops the failures of an account older than
// AccountWindow.
func (l *loginLimiter) forgetOldAccountFailures(ac *accountState, now time.Time) {
	i := 0
	for i < len(ac.failures) && !now.Before(ac.failures[i].Add(l.limits.AccountWindow)) {
		i++
	}
	ac.failures = ac.failures[i:]
}

func (l *loginLimiter) size() int { return len(l.state) + len(l.addresses) + len(l.accounts) }

// sweepLocked drops expired states once the maps have doubled since the
// last sweep, so that memory follows the number of live states. It is
// called before a new state is added.
func (l *loginLimiter) sweepLocked(now time.Time) {
	if l.size() < l.nextSweep {
		return
	}
	for k, st := range l.state {
		if l.expired(st, now) {
			delete(l.state, k)
		}
	}
	for addr, as := range l.addresses {
		l.forgetOldFailures(as, now)
		if len(as.failed) == 0 && !now.Before(as.lockedUntil) {
			delete(l.addresses, addr)
		}
	}
	for account, ac := range l.accounts {
		l.forgetOldAccountFailures(ac, now)
		if len(ac.failures) == 0 && !now.Before(ac.lockedUntil) {
			delete(l.accounts, account)
		}
	}
	l.nextSweep = max(minSweep, 2*l.size())
}
