Bugfix: report unimplemented lock operations of OCM storages as not supported

The outcoming OCM storage driver mapped every lock response it did not know to
an internal error. A remote that does not implement locking answers with
CODE_UNIMPLEMENTED, so a client saw a failed request instead of the "locking not
supported" answer the other drivers give, and could not tell the two apart.
SetLock, GetLock, RefreshLock and Unlock now map that code to
errtypes.NotSupported, which surfaces as unimplemented to the caller.

https://github.com/cs3org/reva/pull/6066
