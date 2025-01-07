Bugfix: broken PROPFIND perms on gRPC

When using the EOS gRPC stack, the permissions returned by PROPFIND
on a folder in a project were erroneous because ACL permissions were
being ignored. This stems from a bug in grpcMDResponseToFileInfo, 
where the SysACL attribute of the FileInfo struct was not being populated.

See: https://github.com/cs3org/reva/pull/4901