Enhancement: userprovider invocations to read users

The userprovider has three read-only invocations for operators: `get` shows a
user by username or by claim, `groups` lists the groups of a user as the driver
resolves them, and `find` finds the users of all types that match a query. One
instance answers each call.

https://github.com/cs3org/reva/pull/XXXX
