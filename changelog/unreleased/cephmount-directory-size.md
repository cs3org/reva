Bugfix: report the recursive size of directories in cephmount

Directories were reported with a size of 0, so a folder's size did not match
the contents seen inside it. The size of the subtree is now taken from the
CephFS `ceph.dir.rbytes` extended attribute.

https://github.com/cs3org/reva/pull/5761