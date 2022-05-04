Change: Implement workaround for chi.RegisterMethod

Implemented a workaround for `chi.RegisterMethod` because of a concurrent map read write issue.
This needs to be fixed upstream in go-chi.

https://github.com/cs3org/reva/pull/2785
