Enhancement: Improve the ini file metadata backend

We improved the ini backend for file metadata:
  - Improve performance
  - Optionally use a reva cache for storing the metadata, which helps
    tremendously when using distributed file systems, for example
  - Allow for using different metadata backends for different storages

We also switched the s3ng integration tests to the ini backend so we 
cover both the xattrs and the ini backend at the same time.

https://github.com/cs3org/reva/pull/3697
