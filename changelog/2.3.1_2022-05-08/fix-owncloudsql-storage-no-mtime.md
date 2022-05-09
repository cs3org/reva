Bugfix: Fix uploads to owncloudsql storage when no mtime is provided

We've fixed uploads to owncloudsql storage when no mtime is provided.
We now just use the current timestamp. Previously the upload did fail.

https://github.com/cs3org/reva/pull/2831
