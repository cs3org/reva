Bugfix: fall back to the sharer as OCM share owner

When the resource owner can't be resolved to a user, as for
files in lightweight homes, the OCM share was sent with an
empty owner address and rejected by the remote server.
The sharer is now used as the owner instead.

https://github.com/cs3org/reva/pull/5897