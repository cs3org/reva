Enhancement: Separate blobs from metadata in the ocis storage driver

We changed the ocis storage driver to keep the file content separate from the
metadata by storing the blobs in a separate directory.

We also streamlined the ocis and the s3ng drivers so that most of the code is
shared between them.

https://github.com/cs3org/reva/pull/1452