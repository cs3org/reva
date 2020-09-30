Bugfix: No longer swallow permissions errors

The storageprovider is no longer ignoring permissions errors.
It will now report them properly using `status.NewPermissionDenied(...)` instead of `status.NewInternal(...)`

https://github.com/cs3org/reva/pull/1206