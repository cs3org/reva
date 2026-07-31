Enhancement: Implement PasswordModify and ModifyWithResult on the LDAP clients

`ConnWithReconnect` and `ConnPool` (`pkg/utils/ldap`) previously stubbed out
`PasswordModify` and `ModifyWithResult` with a "not implemented" error. Both
are now implemented (with the same retry-on-network-error behaviour as the
other operations), so callers that rely on the LDAP Password Modify Extended
Operation or on `ModifyWithResult` no longer break when using either client.

https://github.com/owncloud/reva/pull/681
