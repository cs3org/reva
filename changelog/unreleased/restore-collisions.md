Bugfix: Do not overwrite items on a trash bin restore

A restore onto an existing path deleted the item that was there. A
restore now keeps both, or reports the collisions so that the user can
choose.

https://github.com/cs3org/reva/pull/6052