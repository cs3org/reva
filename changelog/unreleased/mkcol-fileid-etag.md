Enhancement: return fileid and etag in MKCOL response

A successful MKCOL now returns the OC-FileId and OC-ETag headers of the
newly created collection, so clients do not need a follow-up PROPFIND.

The storage.FS CreateDir method was changed to return the created
directory's ResourceInfo, which the storage provider passes along in the
CreateContainerResponse opaque. This way no extra stat call through the
gateway is needed to obtain the fileid and etag.

https://github.com/cs3org/reva/issues/4769
