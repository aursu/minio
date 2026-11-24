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

	xldap "github.com/minio/minio/internal/config/identity/ldap"
	"github.com/minio/minio/internal/logger"
)

// logLDAPLoginFailure logs the cause of a failed LDAP login when it is not
// an unknown user or a wrong password. Such a cause (an ambiguous user, a
// refused bind, a lost connection) is for an administrator; the client
// only gets xldap.ErrLoginFailed. Ordinary failures are not logged, so that
// password guessing does not flood the log.
func logLDAPLoginFailure(ctx context.Context, err error) {
	var loginErr *xldap.LoginError
	if errors.As(err, &loginErr) && !loginErr.Ordinary() {
		authNLogIf(ctx, loginErr.Cause(), logger.WarningKind)
	}
}
