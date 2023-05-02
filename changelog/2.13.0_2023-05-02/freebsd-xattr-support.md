Bugfix: FreeBSD xattr support

We now properly handle FreeBSD xattr namespaces by leaving out the `user.` prefix. FreeBSD adds that automatically.

https://github.com/cs3org/reva/pull/3650