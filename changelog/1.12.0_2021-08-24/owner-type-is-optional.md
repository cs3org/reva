Bugfix: owner type is optional

When reading the user from the extended attributes the user type might not be set, in this case we now return a user with an invalid type, which correctly reflects the state on disk.

https://github.com/cs3org/reva/pull/1978