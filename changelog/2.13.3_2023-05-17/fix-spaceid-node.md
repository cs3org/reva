Bugfix: Fix spaceID in the decomposedFS

We returned the wrong spaceID within ``storageSpaceFromNode``. This was fixed and the storageprovider ID handling refactored.

https://github.com/cs3org/reva/pull/3836
