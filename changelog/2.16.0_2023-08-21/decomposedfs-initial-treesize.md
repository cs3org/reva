Bugfix: Set treesize when creating a storage space

We now explicitly set the treesize metadata to zero when creating a new
storage space. This prevents empty treesize values for spaces with out
any data.

https://github.com/cs3org/reva/pull/4051
