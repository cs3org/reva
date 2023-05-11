Bugfix: Fix unshare for EOS storage driver

In the EOS storage driver, the remove acl operation was a no-op.
After removing a share, the recipient of the share was still able
to operate on the shared resource.
Now this has been fixed, removing correctly the ACL from the shared
resource.

https://github.com/cs3org/reva/pull/3794
