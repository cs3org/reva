Bugfix: Fix remaining space calculation for S3 blobstore

The calculation of the remaining space in the S3 blobstore was incorrectly using
the remaining space of the local disk instead.

https://github.com/cs3org/reva/pull/4867
