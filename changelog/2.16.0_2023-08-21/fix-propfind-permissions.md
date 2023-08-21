Bugfix: Fix propfind permissions

Propfinds permissions field would always contain the permissions of the requested resource, even for its children
This is fixed.

https://github.com/cs3org/reva/pull/4082
