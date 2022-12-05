Bugfix: validate s3ng downloads

The s3ng download func now returns an error in cases where the requested node blob is unknown
or the blob size does not match the node meta blob size.

https://github.com/cs3org/reva/pull/3341
