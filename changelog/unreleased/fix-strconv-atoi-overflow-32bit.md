Bugfix: fix strconv.Atoi overflow on 32-bit platforms

`strconv.Atoi` uses the platform `int` size, which is 32-bit on armv7. The
version and trash suffixes are 13-digit millisecond timestamps, so parsing
them overflowed and the entry was silently dropped from `ListRevisions` and
the recycle bin. Parse them as `int64` instead.

https://github.com/cs3org/reva/pull/5751
