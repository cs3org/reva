Bugfix: decode binary UUID when looking up a users group memberships

The LDAP backend for the users service didn't correctly decode binary UUIDs
when looking up a user's group memberships.

https://github.com/cs3org/reva/pull/3767
