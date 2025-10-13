Bugfix: fix nilpointers seen in prod

* Fix log lines going to stderr, which polluted the logs
* Fix possible nilpointer in getLinkUpdates
* Fix possible nilpointer in eoshttp's `GETFile`

https://github.com/cs3org/reva/pull/5348
