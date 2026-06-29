Bugfix: ceph: fix path resolution

When performing a id-to-path lookup the path may not always be empty,
as in the case for wopi, this case was not handled.

https://github.com/cs3org/reva/pull/5649
