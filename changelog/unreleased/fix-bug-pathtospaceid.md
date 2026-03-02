Bugfix: Fix inverted logic in pathToSpaceID

Fixed a bug where the condition for falling back to the mount_path
when no proper space depth was set was inverted.

https://github.com/cs3org/reva/pull/5524
