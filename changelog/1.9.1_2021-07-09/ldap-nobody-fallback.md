Bugfix: Fill in missing gid/uid number with nobody

When an LDAP server does not provide numeric uid or gid properties for a user we now fall back to a configurable `nobody` id (default 99).

https://github.com/cs3org/reva/pull/1848
