Enhancement: Add s3ng storage driver, storing blobs in a s3-compatible blobstore

We added a new storage driver (s3ng) which stores the file metadata on a local
filesystem (reusing the decomposed filesystem of the ocis driver) and the
actual content as blobs in any s3-compatible blobstore.

https://github.com/cs3org/reva/pull/1429