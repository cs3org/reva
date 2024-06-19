Bugfix: clean empty trash node path on delete

We now delete empty directories in the trash when an item is purged or restored. This prevents old empty directories from slowing down the globbing of trash items.

https://github.com/cs3org/reva/pull/4700
