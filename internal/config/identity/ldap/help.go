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

package ldap

import "github.com/minio/minio/internal/config"

// Help template for LDAP identity feature.
var (
	defaultHelpPostfix = func(key string) string {
		return config.DefaultHelpPostfix(DefaultKVS, key)
	}

	Help = config.HelpKVS{
		config.HelpKV{
			Key:         ServerAddr,
			Description: `AD/LDAP server address e.g. "myldap.com" or "myldapserver.com:636"` + defaultHelpPostfix(ServerAddr),
			Type:        "address",
			Sensitive:   true,
		},
		config.HelpKV{
			Key:         SRVRecordName,
			Description: `DNS SRV record name for LDAP service, if given, must be one of "ldap", "ldaps" or "on"` + defaultHelpPostfix(SRVRecordName),
			Optional:    true,
			Type:        "string",
			Sensitive:   false,
		},
		config.HelpKV{
			Key:         LookupBindDN,
			Description: `DN for LDAP read-only service account used to perform DN and group lookups` + defaultHelpPostfix(LookupBindDN),
			Optional:    true,
			Type:        "string",
			Sensitive:   true,
		},
		config.HelpKV{
			Key:         LookupBindPassword,
			Description: `Password for LDAP read-only service account used to perform DN and group lookups` + defaultHelpPostfix(LookupBindPassword),
			Optional:    true,
			Type:        "string",
			Sensitive:   true,
			Secret:      true,
		},
		config.HelpKV{
			Key:         UserDNSearchBaseDN,
			Description: `";" separated list of user search base DNs e.g. "dc=myldapserver,dc=com"` + defaultHelpPostfix(UserDNSearchBaseDN),
			Optional:    true,
			Type:        "list",
		},
		config.HelpKV{
			Key:         UserDNSearchFilter,
			Description: `Search filter to lookup user DN` + defaultHelpPostfix(UserDNSearchFilter),
			Optional:    true,
			Type:        "string",
		},
		config.HelpKV{
			Key:         UserDNAttributes,
			Description: `"," separated list of user DN attributes e.g. "uid,cn,mail,sshPublicKey"` + defaultHelpPostfix(UserDNAttributes),
			Optional:    true,
			Type:        "list",
		},
		config.HelpKV{
			Key:         GroupSearchFilter,
			Description: `search filter for groups e.g. "(&(objectclass=groupOfNames)(memberUid=%s))"` + defaultHelpPostfix(GroupSearchFilter),
			Optional:    true,
			Type:        "string",
		},
		config.HelpKV{
			Key:         GroupSearchBaseDN,
			Description: `";" separated list of group search base DNs e.g. "dc=myldapserver,dc=com"` + defaultHelpPostfix(GroupSearchBaseDN),
			Optional:    true,
			Type:        "list",
		},
		config.HelpKV{
			Key:         TLSSkipVerify,
			Description: `trust server TLS without verification` + defaultHelpPostfix(TLSSkipVerify),
			Optional:    true,
			Type:        "on|off",
		},
		config.HelpKV{
			Key:         ServerInsecure,
			Description: `allow plain text connection to AD/LDAP server` + defaultHelpPostfix(ServerInsecure),
			Optional:    true,
			Type:        "on|off",
		},
		config.HelpKV{
			Key:         ServerStartTLS,
			Description: `use StartTLS connection to AD/LDAP server` + defaultHelpPostfix(ServerStartTLS),
			Optional:    true,
			Type:        "on|off",
		},
		config.HelpKV{
			Key:         LoginFailureDelay,
			Description: `least time a failed login takes, so that an unknown user and a wrong password cannot be told apart by timing; set it above the slowest normal LDAP lookup and bind` + defaultHelpPostfix(LoginFailureDelay),
			Optional:    true,
			Type:        "duration",
		},
		config.HelpKV{
			Key:         LoginBackoff,
			Description: `wait after a failed login before the same account may try again from the same client address` + defaultHelpPostfix(LoginBackoff),
			Optional:    true,
			Type:        "duration",
		},
		config.HelpKV{
			Key:         LoginBackoffFactor,
			Description: `factor the wait grows by after every further failure in a row; 1 keeps it at login_backoff` + defaultHelpPostfix(LoginBackoffFactor),
			Optional:    true,
			Type:        "number",
		},
		config.HelpKV{
			Key:         LoginMaxFailures,
			Description: `failed logins in a row that lock the account out for that client address; 0 turns the login limits off` + defaultHelpPostfix(LoginMaxFailures),
			Optional:    true,
			Type:        "number",
		},
		config.HelpKV{
			Key:         LoginLockout,
			Description: `how long a lockout lasts; failures are also forgotten this long after the last wait ended` + defaultHelpPostfix(LoginLockout),
			Optional:    true,
			Type:        "duration",
		},
		config.HelpKV{
			Key:         LoginMaxAccountsPerAddress,
			Description: `different accounts that may fail from one client address within login_address_lockout before the address is locked out for every account; 0 turns this limit off` + defaultHelpPostfix(LoginMaxAccountsPerAddress),
			Optional:    true,
			Type:        "number",
		},
		config.HelpKV{
			Key:         LoginAddressLockout,
			Description: `how long an address lockout lasts, and how long a failed account counts against its address` + defaultHelpPostfix(LoginAddressLockout),
			Optional:    true,
			Type:        "duration",
		},
		config.HelpKV{
			Key:         LoginMaxFailuresPerAccount,
			Description: `failed logins of one account, from any client addresses, within login_account_window that lock the account out for every address for login_account_window; 0 turns this limit off` + defaultHelpPostfix(LoginMaxFailuresPerAccount),
			Optional:    true,
			Type:        "number",
		},
		config.HelpKV{
			Key:         LoginAccountWindow,
			Description: `the window login_max_failures_per_account counts in, and how long the account lockout lasts` + defaultHelpPostfix(LoginAccountWindow),
			Optional:    true,
			Type:        "duration",
		},
		config.HelpKV{
			Key:         config.Comment,
			Description: config.DefaultComment,
			Optional:    true,
			Type:        "sentence",
		},
	}
)
