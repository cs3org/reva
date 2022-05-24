Bugfix: Update quota calculation

We now render the `free` and `definition` quota properties, taking into account the remaining bytes reported from the storage space and calculating `relative` only when possible.

https://github.com/cs3org/reva/pull/2870