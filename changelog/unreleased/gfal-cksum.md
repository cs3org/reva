Enhancement: Use standard header for checksums

On HEAD requests, we currently expose checksums (when available) using the
ownCloud-specific header, which is typically consumed by the sync clients.

This patch adds the standard Digest header using the standard format
detailed at https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Digest.
This is e.g. used by GFAL/Rucio clients in the context of managed transfers of datasets.

https://github.com/cs3org/reva/pull/2921
