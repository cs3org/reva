Bugfix: Handle nil quota in decomposedfs 

Do not nil pointer derefenrence when sending nil quota to decomposedfs

https://github.com/cs3org/reva/issues/2167
