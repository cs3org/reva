Enhancement: Per service TracerProvider

To improve tracing we create separate TracerProviders per service now.
This is especially helpful when running multiple reva services in a single
process (like e.g. oCIS does).

https://github.com/cs3org/reva/pull/2962
https://github.com/cs3org/reva/pull/2978
