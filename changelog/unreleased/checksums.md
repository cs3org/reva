Enhancement: Checksum support

We now support checksums on file uploads and PROPFIND results. On uploads, the ocdav service now forwards the `OC-Checksum` (and the similar TUS `Upload-Checksum`)  header to the storage provider. We added an internal http status code that allows storage drivers to return checksum errors. On PROPFINDs, ocdav now renders the `<oc:checksum>` header in a bug compatible way for oc10 backward compatibility with existing clients. Finally, GET and HEAD requests now return the `OC-Checksum` header.

https://github.com/cs3org/reva/pull/1400
https://github.com/owncloud/core/pull/38304
https://github.com/owncloud/ocis/issues/1291
https://github.com/owncloud/ocis/issues/1316
