Bugfix: remove a data race when `revad` starts multiple configs

When `revad` started several configs, each instance registered itself in a  package level slice from its own goroutine.
Append is not thread-safe, so the slice could become corrupted.
Nothing ever read the slice, so it was removed.

https://github.com/cs3org/reva/pull/5946
