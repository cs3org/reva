Enhancement: return fileid and etag in MKCOL response

A successful MKCOL now returns the OC-FileId and OC-ETag headers of the
newly created collection, so clients do not need a follow-up PROPFIND.

https://github.com/cs3org/reva/issues/4769
