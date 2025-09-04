Bugfix: fix nilpointers in spaces shares

* Set `SkipFetchingGroupMembers` and `SkipFetchingUserGroups` to `true` in libregraph API; we don't need those there and fetching group members of `cern-all-users` crashes the daemon
* Better nil handling
* Move some conversion methods from `shares.go` to `conversions.go`

https://github.com/cs3org/reva/pull/5290