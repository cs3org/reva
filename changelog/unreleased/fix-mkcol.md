Bugfix: Return correct error during MKCOL

We need to return a "PreconditionFailed" error if one of the parent folders during a MKCOL does not exist.

https://github.com/cs3org/reva/pull/3834
