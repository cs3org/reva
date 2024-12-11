Bugfix: drop unneeded session locks

We no longer lock session metadada files, as they are already written atomically.

https://github.com/cs3org/reva/pull/4985
