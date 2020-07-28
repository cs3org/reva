Enhancement: Update github.com/go-ldap/ldap to v3

In the current version of the ldap lib attribute comparisons are case sensitive. With v3 `GetEqualFoldAttributeValue` is introduced, which allows a case insensitive comparison. Which AFAICT is what the spec says: see https://github.com/go-ldap/ldap/issues/129#issuecomment-333744641

https://github.com/cs3org/reva/pull/1004
