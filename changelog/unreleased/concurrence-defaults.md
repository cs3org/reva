Enhancement: Modify the concurrency default

We have changed the default MaxConcurrency value from 100 to 5 to prevent too frequent gc runs on low memory systems.

https://github.com/cs3org/reva/pull/4485
https://github.com/owncloud/ocis/issues/8257