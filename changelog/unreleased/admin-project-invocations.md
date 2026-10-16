Enhancement: operate the projects catalogue through the Admin API

The spaces registry exposes admin invocations to list every project of the
catalogue with filters on status, owner and creation date, read one, change
its status or owner, record a new project, delete a record and flush the
cached project list of an instance. The SQL projects manager gained the
corresponding methods and invalidates its cache on every mutation; its cache
TTL was configured in nanoseconds instead of seconds and is now what the
configuration says.

https://github.com/cs3org/reva/pull/5901
