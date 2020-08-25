Bugfix: owncloud driver - propagate mtime on RemoveGrant

When removing a grant the mtime change also needs to be propagated. Only affectsn the owncluod storage driver.

https://github.com/cs3org/reva/pull/1115