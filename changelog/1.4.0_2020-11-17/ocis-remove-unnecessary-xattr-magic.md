Bugfix: stop setting propagation xattr on new files

We no longer set the propagation flag on a file because it is only evaluated for folders anyway.

https://github.com/cs3org/reva/pull/1265
