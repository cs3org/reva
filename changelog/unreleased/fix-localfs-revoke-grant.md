Bugfix: make localfs grants revocable and listable

The localfs driver wrote, revoked and read back an ACL grantee with three
different key formats. AddGrant stored a user key that carries the user type and
the idp, and a group key with an empty user type; RemoveGrant looked for keys
without either, so the revoke matched no row and silently kept the grantee's
access after a share was unshared or rejected. ListGrants split the key at every
colon, which truncated a URL idp, and it reported the user type as the opaque id.

The key is now built by one helper used by both AddGrant and RemoveGrant, and it
is parsed with bounded splits. Revokes match the identity rather than the whole
key, because the share manager stores only the grantee name and sends no idp,
while the grant was written with one. Adding a grant clears any other row of the
same identity, so the two shapes cannot coexist.

https://github.com/cs3org/reva/pull/6065
