Bugfix: No longer swallow permissions errors in the gateway

The gateway is no longer ignoring permissions errors.
It will now check the status for `rpc.Code_CODE_PERMISSION_DENIED` codes
and report them properly using `status.NewPermissionDenied` or `status.NewInternal` instead of reusing the original response status.

https://github.com/cs3org/reva/pull/1210