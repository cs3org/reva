Bugfix: Normalize remote user ids in OCM to avoid malformed shareWith

This fixes cross-vendor OCM shares failing when Reva is the sender and the
peer is a non-conformant OCM server. The OCM spec requires the invite
`userID` to be the bare identifier of the user at their OCM Server, with the
host carried separately in `recipientProvider`. Some peers instead send a
fully-qualified `userID` such as `id@host` (oCIS) or `id@https://host`
(OpenCloud).

Reva stored that qualified string verbatim as the federated user's opaque id
and later re-appended the provider domain when building `shareWith`, producing
malformed recipients like `id@host@host` or `id@https://host@host`. Receivers
could not resolve those to a local user, so the share silently never
materialized even though the HTTP request returned 200 OK.

Reva now normalizes remote user ids on ingress (invite acceptance and invite
forwarding) by stripping a redundant, self-referential provider suffix that
matches the known provider domain, and defensively formats outbound OCM
Addresses so it never emits the `id@host@host` form. Identifiers that
legitimately contain `@` for a different host are left untouched, and
spec-conformant peers are unaffected.

https://github.com/cs3org/reva/pull/5695
