Bugfix: Fix FileUploaded event being emitted too early

We fixed a problem where the FileUploaded event was emitted before the upload had actually finished.

https://github.com/cs3org/reva/pull/2882
