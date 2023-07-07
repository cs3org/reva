Bugfix: fix messagepack propagation

We cannot read from the lockfile. The data is in the metadata file.

https://github.com/cs3org/reva/pull/4048