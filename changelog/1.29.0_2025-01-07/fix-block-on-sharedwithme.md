Bugfix: blocking reva on listSharedWithMe

`listSharesWithMe` blocked a reva thread in the case that one of the shares was not resolvable. This has now been fixed

https://github.com/cs3org/reva/pull/5006
