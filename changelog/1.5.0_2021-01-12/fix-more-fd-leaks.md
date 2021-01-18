Bugfix: Fix fd leaks

There were some left over open file descriptors on simple.go.

https://github.com/cs3org/reva/pull/1338