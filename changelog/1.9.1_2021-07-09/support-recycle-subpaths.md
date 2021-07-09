Enhancement: Support trashbin sub paths in the recycle API

The recycle API could only act on the root items of the trashbin. Meaning if you delete a deep tree, you couldn't restore just one file from that tree but you had to restore the whole tree. Now listing, restoring and purging work also for sub paths in the trashbin.

https://github.com/cs3org/reva/pull/1827

