Enhancement: operate shares and public links through the Admin API

The user share provider and the public share provider expose admin
invocations: an operator can list the shares and links of any user (orphaned
and expired ones included, with their current paths), create them as a given
initiator, remove them, set or clear their expiration, remove the storage grant
of an expired share, transfer them to another project administrator, move them
to another path and mark them orphaned. Mutations act on the storage as the
share's initiator through the gateway, so the grants end up exactly as for a
share made from the web interface, and every mutation is audited.

To act as a user the services sign tokens with the deployment's JWT secret
(`jwt_secret`, falling back to `[shared]`). The SQL share manager gained an
owner filter, `SetExpiration` and `ListPublicLinksInSpace` for this.

https://github.com/cs3org/reva/pull/5900
