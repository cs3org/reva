Bugfix: normal users were incorrectly resolved as ocm users 

When listing OCM shares the grantee was incorrectly looked 
up as an ocm user but in fact it will always be a local user.

https://github.com/cs3org/reva/pull/5882