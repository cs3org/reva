Bugfix: Resolve single-file OCM shares correctly over WebDAV

This fixes cross-vendor OCM shares failing to appear on the receiver when Reva
is the sender and the shared resource is a single file. Remote receivers
(oCIS, OpenCloud) mount an OCM share as a directory and address the shared file
as `<share>/<name>`, so the incoming WebDAV request carries the file's own name
as a relative path.

The `ocmoutcoming` storage driver resolved the share token to the shared
resource (the file itself) and then unconditionally joined the relative path
onto it, producing a doubled path such as
`/home/einstein/report.txt/report.txt`. That path does not exist, so the stat
failed and the WebDAV `PROPFIND` returned HTTP 500. Receivers treat a failed
stat as a missing share and silently drop it from their "shared with me"
listing, so the file never became visible even though the share was created and
accepted.

The driver now resolves paths based on the shared resource type. Folder shares
still nest the child beneath the container. For a single-file share, which has
exactly one resource, both the share root and a single-segment child resolve to
the file itself, so `<file>/<file>` is never built and receivers whose appended
name differs from the storage path base are tolerated. Nested paths under a
file share are rejected as malformed.

https://github.com/cs3org/reva/pull/5696
