Bugfix: harden uploads

Uploads now check response headers for a file id and omit a subsequent stat request which might land on a storage provider that does not yet see the new file due to latency, eg. when NFS caches direntries.

https://github.com/cs3org/reva/pull/3899