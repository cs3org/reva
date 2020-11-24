Bugfix: Fix file descriptor leak on ocdav put handler 

File descriptors on the ocdav service, especially on the put handler was leaking http connections. This PR addresses this.

https://github.com/cs3org/reva/pull/1260