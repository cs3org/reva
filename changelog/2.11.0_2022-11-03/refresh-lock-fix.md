Bugfix: Refresh lock in decomposedFS needs to overwrite

We fixed a bug in the refresh lock operation in the DecomposedFS. The new lock was appended but needs to overwrite the existing one.

https://github.com/cs3org/reva/pull/3307
