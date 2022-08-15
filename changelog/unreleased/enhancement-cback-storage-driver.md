Enhancement: Implementation of cback storage driver for REVA

Allows for backup browsing. The The cback storage driver implements the storage.FS interface, in particular the following methods:

GetMD (=stat)
ListFolder
Download

All other methods return "operation not permitted error" since it is only a read only FS.

https://github.com/cs3org/reva/pull/3116