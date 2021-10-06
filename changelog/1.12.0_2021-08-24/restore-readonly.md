Bugfix: Logic to restore files to readonly nodes

This impacts solely the DecomposedFS. Prior to these changes there was no validation when a user tried to restore a file from the trashbin to a share location (i.e any folder under `/Shares`).

With this patch if the user restoring the resource has write permissions on the share, restore is possible.

https://github.com/cs3org/reva/pull/1913
