Bugfix: allow downloading shared files

We allow HEAD and GET on resource IDs now,
as this is the call the web client makes to
download a shared file.

https://github.com/cs3org/reva/pull/6030