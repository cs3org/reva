Enhancement: support additional gids in cephmount fs driver

When accessing the underlying Ceph mount, the linux thread only contained the uid/gid of the user.
Now it also contaisn the additional gids of the user, which allows to access files that are only accessible by group permissions.

https://github.com/cs3org/reva/pull/5519
