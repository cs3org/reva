Bugfix: Improved validation for the CreatePublicShare method

CreatePublicShare trusted the client-supplied resource info instead of the
one it had just verified via Stat, and never checked Stat's own status
code. Combined with a missing nil-check on write, this let a public share
be persisted with a nil resource_id, which crashes ListPublicShares with a
nil-pointer panic for the whole tenant on every subsequent read.

CreatePublicShare now propagates a non-OK Stat status instead of falling
through with a nil resource info, persists the resource id verified by
Stat instead of the client-supplied one, and the json public-share
manager rejects a nil/empty resource id before persisting.

https://github.com/owncloud/reva/pull/736
