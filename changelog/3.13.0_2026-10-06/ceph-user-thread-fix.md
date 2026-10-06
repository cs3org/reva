Bugfix: set ceph user thread groups per thread

syscall.Setgroups sets the groups for ALL threads of the process, which caused
race conditions when multiple users were using the same process. Similarly, when
a user thread finished, the original groups were restored on ALL threads.

This is fixed by using unix.Setgroups, which only applies to the calling thread.

https://github.com/cs3org/reva/pull/5858
