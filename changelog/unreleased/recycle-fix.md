Bugfix: eos: fixed error reporting for too large recycle bin listing

EOS returns E2BIG, which internally gets converted to PermissionDenied
and has to be properly handled in this case.

https://github.com/cs3org/reva/pull/4591
