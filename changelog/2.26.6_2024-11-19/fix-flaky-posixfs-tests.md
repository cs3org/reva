Bugfix: Fix flaky posixfs integration tests

We fixed a problem with the posixfs integration tests where the in-memory id cache sometimes hadn't caught up with the cleanup between test runs leading to flaky failures.

https://github.com/cs3org/reva/pull/4929

