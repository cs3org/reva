Bugfix: Refactors logger to have ctx

This fixes the native library loggers which are not associated with the context and thus are not handled properly in the reva runtime.

https://github.com/cs3org/reva/pull/2841
