Enhancement: Reduce memory usage when uploading with S3ng storage

The memory usage could be high when uploading files using the S3ng storage.
By providing the actual file size when triggering `PutObject`, 
the overall memory usage is reduced.

https://github.com/cs3org/reva/pull/1940
