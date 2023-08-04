Bugfix: S3ng include md5 checksum on put

We've fixed the S3 put operation of the S3ng storage to include a md5 checksum.

This md5 checksum is needed when a bucket has a retention period configured (see https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html).

https://github.com/cs3org/reva/pull/4100
