Bugfix: Normalize user ids on incoming OCM shares

This fixes cross-vendor OCM shares failing on the receiving side when Reva is
the receiver and the peer is a non-conformant OCM server. It is the
receiver-side counterpart to the sender-side normalization in #5695.

The OCM spec expects an address as `<bare-id>@<provider>`, with the host kept
separate from the user id. Some peers glue the host onto the id: oCIS sends
`id@host` and OpenCloud sends `id@https://host`, and by the time it reaches the
`POST /ocm/shares` handler the id is doubled up as `id@host@host` (with or
without a scheme).

`GetUserIdFromOCMAddress` only strips the final `@host`, so the parsed opaque id
kept a leftover host suffix that broke user resolution, and the two vendors
failed at two different points. For `shareWith` (the local recipient) the
leftover host made the local user lookup miss and the request returned 404 user
not found. For `sender` and `owner` (the remote sharer) it made the accepted-user
lookup miss, because the accepted remote user is stored with a bare id during the
invitation flow, and the request returned 401 unauthenticated.

The incoming-share handler now parses these three addresses through a helper that
also strips a redundant, self-referential provider suffix that matches the parsed
provider domain. Identifiers that legitimately contain `@` for a different host
are left untouched, and spec-conformant peers such as CERNBox-to-CERNBox and
Nextcloud are unaffected.

https://github.com/cs3org/reva/pull/5698
