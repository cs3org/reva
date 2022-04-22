Change: Replace template in GroupFilter for UserProvider with a simple string

Previously the "groupfilter" configuration for the UserProvider expected a
go-template value (based of of an `userpb.UserId` as it's input). And it
assumed we could run a single LDAP query to get all groups a user is member of
by specifying the userid. However most LDAP Servers store the GroupMembership
by either username (e.g. in memberUID Attribute) or by the user's DN (e.g. in
member/uniqueMember).

This change removes the userpb.UserId template processing from the groupfilter
and replaces it with a single string (the username) to cleanup the config a
bit. Existing configs need to be update to replace the go template references
in `groupfilter` (e.g. `{{.}}` or `{{.OpaqueId}}`) with `{{query}}`.

https://github.com/cs3org/reva/pull/2436
