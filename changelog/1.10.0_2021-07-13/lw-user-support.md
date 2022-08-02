Enhancement: Add support for lightweight user types

This PR adds support for assigning and consuming user type when setting/reading
users. On top of that, support for lightweight users is added. These users have
to be restricted to accessing only shares received by them, which is
accomplished by expanding the existing RBAC scope.

https://github.com/cs3org/reva/pull/1744
https://github.com/cs3org/cs3apis/pull/120
