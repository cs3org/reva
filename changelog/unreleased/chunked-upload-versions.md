Bugfix: Fix chunked uploads for new versions

Chunked uploads didn't create a new version, when the file to upload already existed.

https://github.com/cs3org/reva/pull/1899
