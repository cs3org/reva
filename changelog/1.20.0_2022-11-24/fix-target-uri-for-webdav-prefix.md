Bugfix: Fix datatxtarget uri when prefix is used

When a webdav prefix is used it appears in both host and name parameter of the target uri for data transfer. This PR fixes that.

https://github.com/cs3org/reva/pull/2973
