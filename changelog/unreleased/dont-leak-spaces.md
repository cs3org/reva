Bugfix: Don't leak space information on update drive

There were some problems with the `UpdateDrive` func in decomposedfs when it is called without permission
-   When calling with empty request it would leak the complete drive info
-   When calling with non-empty request it would leak the drive name

https://github.com/cs3org/reva/pull/3447
