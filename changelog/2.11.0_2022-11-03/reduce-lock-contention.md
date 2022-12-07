Enhancement: Reduce lock contention issues

We reduced lock contention during high load by optimistically non-locking when listing the extended attributes of a file.
Only in case of issues the list is read again while holding a lock.

https://github.com/cs3org/reva/pull/3395
