Enhancement: Split LDAP user filters

The current LDAP user and auth filters only allow a single `%s` to be replaced with the relevant string.
The current `userfilter` is used to lookup a single user, search for share recipients and for login. To make each use case more flexible we split this in three and introduced templates.

For the `userfilter` we moved to filter templates that can use the CS3 user id properties `{{.OpaqueId}}` and `{{.Idp}}`:
```
userfilter = "(&(objectclass=posixAccount)(|(ownclouduuid={{.OpaqueId}})(cn={{.OpaqueId}})))"
```

We introduced a new `findfilter` that is used when searching for users. Use it like this:
```
findfilter = "(&(objectclass=posixAccount)(|(cn={{query}}*)(displayname={{query}}*)(mail={{query}}*)))"
```

Furthermore, we also introduced a dedicated login filter for the LDAP auth manager:
```
loginfilter = "(&(objectclass=posixAccount)(|(cn={{login}})(mail={{login}})))"
```

These filter changes are backward compatible: `findfilter` and `loginfilter` will be derived from the `userfilter` by replacing `%s` with `{{query}}` and `{{login}}` respectively. The `userfilter` replaces `%s` with `{{.OpaqueId}}`

Finally, we changed the default attribute for the immutable uid of a user to `ms-DS-ConsistencyGuid`. See https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts for the background. You can fall back to `objectguid` or even `samaccountname` but you will run into trouble when user names change. You have been warned.

https://github.com/cs3org/reva/pull/996
