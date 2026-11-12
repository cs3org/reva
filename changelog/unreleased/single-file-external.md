Bugfix: try upload on file itself first

We should not try to access the parent folder
if the file already exists; this results in
PermissionDenied for single file shares

https://github.com/cs3org/reva/pull/6005