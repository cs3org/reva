Bugfix: check for spaces in listshares

Opening files in their app from the "Shared with me" view was broken, because we returned spaces ids even on old clients.

https://github.com/cs3org/reva/pull/5166