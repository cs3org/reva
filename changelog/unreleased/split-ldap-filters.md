Enhancement: Split LDAP user filters

The current LDAP user filters only allow a single `%s` to be replaced with the relevant string. We moved to filter templates that can use the CS3 user id properties `{{.OpaqueId}}` and `{{.Idp}}`. Furthermore,
we introduced a new find filter that is used when searching for users. An example config would be
```
userfilter = "(&(objectclass=posixAccount)(|(ownclouduuid={{.OpaqueId}})(cn={{.OpaqueId}})))"
findfilter = "(&(objectclass=posixAccount)(|(cn={{query}}*)(displayname={{query}}*)(mail={{query}}*)))"
```
As you can see the userfilter is used to lookup a single user, whereas the findfilter is for a broader search, e.g. used when searching for share recipients.

https://github.com/cs3org/reva/pull/996
