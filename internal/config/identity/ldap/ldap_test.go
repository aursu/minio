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
	"errors"
	"fmt"
	"strings"
	"testing"

	ldap "github.com/go-ldap/ldap/v3"
)

func TestLoginError(t *testing.T) {
	notFound := &LoginError{fmt.Errorf("Unable to find user DN: %w", errors.New("User DN not found for: nosuchuser"))}
	wrongPassword := &LoginError{fmt.Errorf("LDAP auth failed for DN uid=dillon,ou=people,dc=min,dc=io: %w",
		ldap.NewError(ldap.LDAPResultInvalidCredentials, errors.New("Invalid Credentials")))}
	ambiguous := &LoginError{fmt.Errorf("Unable to find user DN: %w", errors.New("Multiple DNs for dillon found"))}
	refused := &LoginError{fmt.Errorf("LDAP auth failed for DN uid=dillon,ou=people,dc=min,dc=io: %w",
		ldap.NewError(ldap.LDAPResultUnwillingToPerform, errors.New("account locked")))}

	for name, tc := range map[string]struct {
		err      *LoginError
		ordinary bool
	}{
		"unknown user":    {notFound, true},
		"wrong password":  {wrongPassword, true},
		"ambiguous user":  {ambiguous, false},
		"refused bind":    {refused, false},
		"login limits":    {&LoginError{fmt.Errorf("%w: uid=dillon from 192.0.2.1, retry in 2s", errLoginRefused)}, true},
		"lockout started": {&LoginError{fmt.Errorf("%w: uid=dillon from 192.0.2.1: %w", errLockoutStarted, wrongPassword.cause)}, false},
	} {
		t.Run(name, func(t *testing.T) {
			var err error = tc.err
			if err.Error() != ErrLoginFailed.Error() {
				t.Errorf("message: got %q", err.Error())
			}
			if strings.Contains(err.Error(), "uid=") || strings.Contains(err.Error(), "dillon") {
				t.Errorf("message names the user: %q", err.Error())
			}
			// The console falls back to its built-in login on "not found".
			if !strings.Contains(strings.ToLower(err.Error()), "not found") {
				t.Errorf("message lacks \"not found\": %q", err.Error())
			}
			if !errors.Is(fmt.Errorf("wrapped: %w", err), ErrLoginFailed) {
				t.Error("errors.Is(err, ErrLoginFailed) is false")
			}
			if tc.err.Ordinary() != tc.ordinary {
				t.Errorf("Ordinary: got %v, want %v", tc.err.Ordinary(), tc.ordinary)
			}
			if tc.err.Cause() == nil || tc.err.Cause().Error() == err.Error() {
				t.Errorf("Cause: got %v", tc.err.Cause())
			}
		})
	}
}
